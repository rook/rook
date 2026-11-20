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
	"cmp"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/betterleaks/betterleaks/v2/config"
	"github.com/betterleaks/betterleaks/v2/report"
	"github.com/betterleaks/betterleaks/v2/scan"
)

//go:embed betterleaks.toml
var betterleaksConfig string

const (
	// betterleaksChunkSize bounds the bytes scanned at once, however long a
	// file or a line is.
	betterleaksChunkSize = 1 << 20
	// betterleaksOverlap is how much of each chunk's end is scanned again at
	// the start of the next, so that every match up to this long, such as a
	// PEM private key, lies wholly inside one chunk.
	betterleaksOverlap  = 16 << 10
	betterleaksReadSize = 16 << 10
)

type betterleaks struct {
	scanner *scan.Scanner
}

// NewBetterleaks returns a detector that reports each line on which a match
// of a betterleaks rule starts, once per rule: the library's default rules
// plus rook-cephx-key. It scans chunks of many lines, so rules whose matches
// span lines, such as private-key, apply. Findings name the rule and never
// carry the match.
func NewBetterleaks() (Detector, error) {
	cfg, err := config.ParseTOMLString(betterleaksConfig, "betterleaks.toml")
	if err != nil {
		return nil, fmt.Errorf("parse betterleaks config: %w", err)
	}
	scanner, err := scan.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("create betterleaks scanner: %w", err)
	}
	return &betterleaks{scanner: scanner}, nil
}

func (d *betterleaks) Name() string { return "betterleaks" }

func (d *betterleaks) ScanFile(path, rel string) ([]Finding, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := chunkScan{detector: d, rel: rel, seen: map[ruleLine]bool{}}
	r := bufio.NewReaderSize(f, betterleaksReadSize)
	chunk := make([]byte, 0, betterleaksChunkSize+betterleaksOverlap+betterleaksReadSize)
	startLine := 1
	for {
		part, readErr := r.ReadSlice('\n')
		chunk = append(chunk, part...)
		eof := errors.Is(readErr, io.EOF)
		if readErr != nil && !eof && !errors.Is(readErr, bufio.ErrBufferFull) {
			return s.sorted(), fmt.Errorf("read %s: %w", rel, readErr)
		}

		// A chunk ends at a line end unless the line is too long to fit.
		lineEnd := len(chunk) > 0 && chunk[len(chunk)-1] == '\n'
		full := len(chunk) >= betterleaksChunkSize && (lineEnd || len(chunk) >= betterleaksChunkSize+betterleaksOverlap)
		if !eof && !full {
			continue
		}
		s.scan(chunk, startLine)
		if eof {
			return s.sorted(), nil
		}

		cut := overlapStart(chunk)
		startLine += bytes.Count(chunk[:cut], []byte{'\n'})
		chunk = chunk[:copy(chunk, chunk[cut:])]
	}
}

// overlapStart returns where the part of chunk to scan again begins: at the
// start of the line holding its last betterleaksOverlap bytes, or inside
// that line if it is too long to carry whole.
func overlapStart(chunk []byte) int {
	cut := len(chunk) - betterleaksOverlap
	lineStart := bytes.LastIndexByte(chunk[:cut], '\n') + 1
	if cut-lineStart <= betterleaksOverlap {
		return lineStart
	}
	return cut
}

type ruleLine struct {
	rule string
	line int
}

type chunkScan struct {
	detector *betterleaks
	rel      string
	seen     map[ruleLine]bool
	findings []Finding
}

// scan records each rule matching chunk, whose first line is startLine,
// once per line it starts on.
func (s *chunkScan) scan(chunk []byte, startLine int) {
	if len(chunk) == 0 {
		return
	}
	results := s.detector.scanner.ScanString(string(chunk))
	slices.SortFunc(results, func(a, b report.Finding) int {
		return cmp.Compare(a.Location.StartLine, b.Location.StartLine)
	})
	line, lineStart := 1, 0
	for _, r := range results {
		for line < r.Location.StartLine {
			i := bytes.IndexByte(chunk[lineStart:], '\n')
			if i < 0 {
				break
			}
			lineStart += i + 1
			line++
		}
		matchLine := r.Location.StartLine
		// A rule whose leading context, such as (?:^|[^A-Za-z0-9_-]), matches
		// the newline ending the previous line is placed on that line.
		start := lineStart + r.Location.StartColumn - 1
		if line == matchLine && start >= 0 && start < len(chunk) && chunk[start] == '\n' {
			matchLine++
		}

		key := ruleLine{rule: r.RuleID, line: startLine - 1 + matchLine}
		if s.seen[key] {
			continue
		}
		s.seen[key] = true
		s.findings = append(s.findings, Finding{
			Detector: s.detector.Name(),
			What:     key.rule,
			Location: Location{File: s.rel, Line: key.line},
		})
	}
}

func (s *chunkScan) sorted() []Finding {
	slices.SortFunc(s.findings, func(a, b Finding) int {
		return cmp.Or(cmp.Compare(a.Location.Line, b.Location.Line), cmp.Compare(a.What, b.What))
	})
	return s.findings
}
