/*
Copyright 2026 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package leakguard

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
)

// KnownLeak is a leak that cannot be fixed yet, whose exact-match findings
// are reported but never fail. A finding is the leak's when its What matches
// one of What, its file matches File, and its line holds every marker of one
// of Markers. What and File are path.Match patterns.
type KnownLeak struct {
	Name string
	// Reason says what leaks and where it comes from, never the value.
	Reason string
	What   []string
	File   string
	// Markers are alternative sets of constant text, never a secret.
	Markers [][]string
}

// Validate reports a leak missing a field, with a malformed pattern, or with
// an empty marker set or marker, which would mark every line.
func (k KnownLeak) Validate() error {
	var errs []error
	if k.Name == "" {
		errs = append(errs, errors.New("no name"))
	}
	if k.Reason == "" {
		errs = append(errs, errors.New("no reason"))
	}
	if len(k.What) == 0 {
		errs = append(errs, errors.New("no what pattern"))
	}
	for _, p := range k.What {
		if _, err := path.Match(p, ""); err != nil {
			errs = append(errs, fmt.Errorf("what pattern %q: %w", p, err))
		}
	}
	if k.File == "" {
		errs = append(errs, errors.New("no file pattern"))
	} else if _, err := path.Match(k.File, ""); err != nil {
		errs = append(errs, fmt.Errorf("file pattern %q: %w", k.File, err))
	}
	if len(k.Markers) == 0 {
		errs = append(errs, errors.New("no marker set"))
	}
	for i, set := range k.Markers {
		if len(set) == 0 || slices.Contains(set, "") {
			errs = append(errs, fmt.Errorf("marker set %d is empty or holds an empty marker", i))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("known leak %q: %w", k.Name, err)
	}
	return nil
}

// Matches reports whether f, found on line, is a finding of k.
func (k KnownLeak) Matches(f Finding, line []byte) bool {
	return k.locates(f) && k.marks(line)
}

// locates reports whether f is of one of k's secrets in one of k's files.
func (k KnownLeak) locates(f Finding) bool {
	if ok, _ := path.Match(k.File, f.Location.File); !ok {
		return false
	}
	return slices.ContainsFunc(k.What, func(p string) bool {
		ok, _ := path.Match(p, f.What)
		return ok
	})
}

func (k KnownLeak) marks(line []byte) bool {
	for _, set := range k.Markers {
		if holdsAll(line, set) {
			return true
		}
	}
	return false
}

func holdsAll(line []byte, markers []string) bool {
	for _, m := range markers {
		if !bytes.Contains(line, []byte(m)) {
			return false
		}
	}
	return true
}

// KnownFinding is an exact-match finding of the known leak named Leak.
type KnownFinding struct {
	Finding
	Leak string
}

// matchKnown splits findings into those of no known leak and those of one,
// reading the line of each finding a known leak locates from the capture to
// test its markers. A finding whose line was not read is of no known leak.
func (g *Guard) matchKnown(findings []Finding) (exact []Finding, known []KnownFinding, err error) {
	// byFile maps a file to the line numbers of its findings a known leak
	// locates, and each line number to the indexes of those findings.
	byFile := map[string]map[int][]int{}
	for i, f := range findings {
		if !slices.ContainsFunc(g.known, func(k KnownLeak) bool { return k.locates(f) }) {
			continue
		}
		if byFile[f.Location.File] == nil {
			byFile[f.Location.File] = map[int][]int{}
		}
		byFile[f.Location.File][f.Location.Line] = append(byFile[f.Location.File][f.Location.Line], i)
	}

	leakOf := make([]string, len(findings))
	var errs []error
	for _, file := range slices.Sorted(maps.Keys(byFile)) {
		byLine := byFile[file]
		errs = append(errs, eachLine(filepath.Join(g.dir, filepath.FromSlash(file)), file, byLine, func(i int, line []byte) {
			for _, k := range g.known {
				if k.Matches(findings[i], line) {
					leakOf[i] = k.Name
					return
				}
			}
		}))
	}

	for i, f := range findings {
		if leakOf[i] == "" {
			exact = append(exact, f)
			continue
		}
		known = append(known, KnownFinding{Finding: f, Leak: leakOf[i]})
	}
	return exact, known, errors.Join(errs...)
}

// eachLine calls fn with each index byLine holds for a line of the file at
// name, reported as rel, and that line, numbering lines as the exact-match
// detector does.
func eachLine(name, rel string, byLine map[int][]int, fn func(i int, line []byte)) error {
	f, err := os.Open(name)
	if err != nil {
		return fmt.Errorf("re-read %s: %w", rel, err)
	}
	defer f.Close()

	last := slices.Max(slices.Collect(maps.Keys(byLine)))
	r := bufio.NewReader(f)
	for lineNo := 1; lineNo <= last; lineNo++ {
		line, readErr := r.ReadBytes('\n')
		if len(line) > 0 {
			for _, i := range byLine[lineNo] {
				fn(i, line)
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("re-read %s: %w", rel, readErr)
		}
	}
	return nil
}
