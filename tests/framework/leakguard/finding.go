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

// Package leakguard detects known secrets leaking into the logs and other
// artifacts an integration test captures.
package leakguard

import (
	"fmt"
	"io"
)

// Secret is a value that must not appear in captured output. Source and Name
// identify it in findings so that reports never need to repeat Value.
// Formatting a Secret with any fmt verb renders redactedValue in place of
// Value. Printers that bypass its methods, such as testify's failure diffs,
// print Value, so tests compare a Secret's fields rather than the Secret.
type Secret struct {
	Source string
	Name   string
	Value  string
}

const redactedValue = "<redacted>"

func (s Secret) String() string {
	return fmt.Sprintf("{%s %s %s}", s.Source, s.Name, redactedValue)
}

func (s Secret) GoString() string {
	return fmt.Sprintf("leakguard.Secret{Source:%q, Name:%q, Value:%q}", s.Source, s.Name, redactedValue)
}

// Format applies to every verb, so no verb can reach the fields directly.
func (s Secret) Format(st fmt.State, verb rune) {
	switch {
	case verb == 'v' && st.Flag('#'):
		_, _ = io.WriteString(st, s.GoString())
	case verb == 'v' && st.Flag('+'):
		_, _ = fmt.Fprintf(st, "{Source:%s Name:%s Value:%s}", s.Source, s.Name, redactedValue)
	case verb == 'q':
		_, _ = fmt.Fprintf(st, "%q", s.String())
	default:
		_, _ = io.WriteString(st, s.String())
	}
}

// Location is a line in a scanned file. File is relative to the guard's
// directory.
type Location struct {
	File string
	Line int
}

// Finding records where a detector found a leak. What names the leak (a
// secret's Source/Name, or a rule ID) and never holds the leaked value.
type Finding struct {
	Detector string
	What     string
	Location Location
}

// Detector scans one file. path is where to read it; rel is the name reported
// in each finding's Location.
type Detector interface {
	Name() string
	ScanFile(path, rel string) ([]Finding, error)
}
