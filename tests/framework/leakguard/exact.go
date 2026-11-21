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
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"slices"
	"strings"
	"unicode"
)

// minSecretLen is the shortest value worth searching for; shorter ones match
// unrelated text too often to mean a leak.
const minSecretLen = 8

// minLineFormLen is the shortest line of a multi-line value searched for on
// its own.
const minLineFormLen = 16

type exactSecret struct {
	what  string
	forms [][]byte
}

type exactMatch struct {
	secrets []exactSecret
}

// NewExactMatch returns a detector that reports each line containing one of
// a secret's forms, once per secret. The forms are the value raw, standard
// base64 encoded, query escaped, and as the body of fmt's %v and %#v of its
// bytes, which is how a logged Secret object renders its data; and for a
// value spanning lines, the value without its trailing newline and each of its
// lines, trimmed, that holds no whitespace and is at least minLineFormLen
// bytes, as key material such as a PEM body line is; PEM armor, a keyring's
// bracketed section header and its caps lines, and the lines of a
// certificate, which is public, are not. Values and forms shorter than
// minSecretLen are ignored.
func NewExactMatch(secrets []Secret) Detector {
	d := &exactMatch{}
	for _, s := range secrets {
		if len(s.Value) < minSecretLen {
			continue
		}
		d.secrets = append(d.secrets, exactSecret{what: s.Source + "/" + s.Name, forms: exactForms(s.Value)})
	}
	return d
}

func exactForms(value string) [][]byte {
	decimal := fmt.Sprintf("%d", []byte(value))
	hex := fmt.Sprintf("%#v", []byte(value))
	forms := []string{
		value,
		base64.StdEncoding.EncodeToString([]byte(value)),
		decimal[len("[") : len(decimal)-len("]")],
		hex[len("[]byte{") : len(hex)-len("}")],
		url.QueryEscape(value),
	}
	if strings.Contains(value, "\n") {
		forms = append(forms, strings.TrimRight(value, "\r\n"))
		inCertificate := false
		for line := range strings.Lines(value) {
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "-----BEGIN "):
				inCertificate = strings.Contains(line, "CERTIFICATE")
			case strings.HasPrefix(line, "-----END "):
				inCertificate = false
			case isKeyringHeader(line):
			case !inCertificate && len(line) >= minLineFormLen && !strings.ContainsFunc(line, unicode.IsSpace):
				forms = append(forms, line)
			}
		}
	}

	var unique [][]byte
	for _, form := range forms {
		if len(form) >= minSecretLen && !slices.ContainsFunc(unique, func(u []byte) bool { return string(u) == form }) {
			unique = append(unique, []byte(form))
		}
	}
	return unique
}

// isKeyringHeader reports whether the trimmed line is the bracketed section
// header of a Ceph keyring.
func isKeyringHeader(line string) bool {
	return strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]")
}

func (d *exactMatch) Name() string { return "exact" }

func (d *exactMatch) ScanFile(path, rel string) ([]Finding, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var findings []Finding
	r := bufio.NewReader(f)
	for lineNo := 1; ; lineNo++ {
		// ReadBytes, unlike bufio.Scanner, has no line length cap.
		line, readErr := r.ReadBytes('\n')
		if len(line) > 0 {
			findings = append(findings, d.scanLine(line, Location{File: rel, Line: lineNo})...)
		}
		if errors.Is(readErr, io.EOF) {
			return findings, nil
		}
		if readErr != nil {
			return findings, fmt.Errorf("read %s: %w", rel, readErr)
		}
	}
}

func (d *exactMatch) scanLine(line []byte, loc Location) []Finding {
	var findings []Finding
	for _, s := range d.secrets {
		for _, form := range s.forms {
			if bytes.Contains(line, form) {
				findings = append(findings, Finding{Detector: d.Name(), What: s.what, Location: loc})
				break
			}
		}
	}
	return findings
}
