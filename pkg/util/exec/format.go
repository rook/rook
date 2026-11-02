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

package exec

import (
	"fmt"
	"strings"
)

// safeArgChars are the non-alphanumeric characters a POSIX shell passes through
// literally, matching the set used by Python's shlex.quote.
const safeArgChars = "@%+=:,./-_"

// redactionMarker stands in for the value of a secret-bearing flag. It reports
// the value's length, the way a Kubernetes Secret's redacted form does, because
// the length is what distinguishes a key that is truncated or of the wrong shape
// from one that is merely wrong - and that distinction is otherwise unavailable
// to an operator reading a log. The length is in bytes, and is the only thing
// the marker takes from the value. An empty value never reaches here; redactArgs
// passes it through unredacted.
//
// Every character it emits is one that both quoteArg and url.QueryEscape leave
// alone - letters, digits, '-' and '_' - so the marker reads the same in a shell
// line as in the query of a sanitized request URL, and quoteArg never has to
// quote the token and bury the flag name inside it. The plural is left
// uninflected so the marker keeps one greppable shape.
func redactionMarker(value string) string {
	return fmt.Sprintf("_REDACTED-%d-bytes_", len(value))
}

// secretValueFlags are the flags whose value authenticates whoever holds it, and
// so must never be reported: a log is readable with pods/log alone, while the
// credential itself is otherwise gated behind Ceph admin exec or Secret read
// access. Rook passes two of these:
//
//   - "--secret" by CreateUser, pkg/operator/ceph/object/user.go
//   - "--secret-key" by GetRealmKeyArgsFromSecret, pkg/operator/ceph/object/objectstore.go
//
// "--password" is listed as cover for a future caller rather than for a leak
// that exists now: the only argv carrying it today is built by a Keystone
// integration test, which has no credential worth protecting.
//
// "--access-key" is deliberately absent. An S3 access key ID names a user rather
// than authenticating one, and rook/rook#18188 settled that it may appear at
// DEBUG.
var secretValueFlags = map[string]struct{}{
	"--password":   {},
	"--secret":     {},
	"--secret-key": {},
}

// FormatCommand renders a command and its arguments as a single line that can be
// pasted into a POSIX shell. Arguments are passed to the kernel as a vector, so
// joining them with plain spaces loses the boundaries between them: an argument
// containing whitespace or a shell metacharacter reads back as several arguments,
// or as several commands.
//
// The line reproduces the command's shape but not its credentials: the value of
// a flag that carries one, such as --secret-key, is replaced by a marker that
// reports nothing about it but its length.
func FormatCommand(command string, arg ...string) string {
	redacted := redactArgs(arg)

	quoted := make([]string, 0, len(redacted)+1)
	quoted = append(quoted, quoteArg(command))
	for _, a := range redacted {
		quoted = append(quoted, quoteArg(a))
	}

	return strings.Join(quoted, " ")
}

// redactArgs returns a copy of arg in which the value of every secret-bearing
// flag is replaced by its redactionMarker, in both the "--secret-key=value" and
// "--secret-key value" spellings. The flag name is left intact so a reader can
// still tell which flags a command was given.
//
// The result is for reporting only. The vector handed to the process has to be
// the caller's own, or the command would be given the marker as its passphrase.
func redactArgs(arg []string) []string {
	redacted := make([]string, len(arg))

	valueFollows := false
	for i, a := range arg {
		if valueFollows {
			valueFollows = false
			// A flag-shaped token here is still the preceding flag's value as far
			// as the command is concerned, and a secret may well begin with a dash,
			// so redact whatever follows a secret flag rather than guessing.
			if a != "" {
				redacted[i] = redactionMarker(a)
				continue
			}
			redacted[i] = a
			continue
		}

		// An empty value discloses nothing, and hiding it would cost a real
		// diagnosis: a Secret whose secret-key is present but empty decodes
		// without error, so the logged command line is the only place an operator
		// can see that the key was empty rather than wrong.
		if flag, value, ok := inlineSecretFlag(a); ok && value != "" {
			redacted[i] = flag + "=" + redactionMarker(value)
			continue
		}

		_, valueFollows = secretValueFlags[a]
		redacted[i] = a
	}

	return redacted
}

// inlineSecretFlag splits a secret-bearing flag that carries its value in the
// same token, as in "--secret-key=value", into its name and its value.
func inlineSecretFlag(arg string) (string, string, bool) {
	flag, value, found := strings.Cut(arg, "=")
	if !found {
		return "", "", false
	}
	if _, ok := secretValueFlags[flag]; !ok {
		return "", "", false
	}

	return flag, value, true
}

// quoteArg single-quotes arg if a shell would not read it back verbatim.
func quoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	if !needsQuoting(arg) {
		return arg
	}

	// A single quote cannot be escaped inside single quotes; the quoted run has to
	// be closed, the quote emitted outside it, and a new run opened.
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

func needsQuoting(arg string) bool {
	for _, r := range arg {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune(safeArgChars, r):
		default:
			return true
		}
	}

	return false
}
