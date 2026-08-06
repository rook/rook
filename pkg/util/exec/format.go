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

import "strings"

// safeArgChars are the non-alphanumeric characters a POSIX shell passes through
// literally, matching the set used by Python's shlex.quote.
const safeArgChars = "@%+=:,./-_"

// FormatCommand renders a command and its arguments as a single line that can be
// pasted into a POSIX shell. Arguments are passed to the kernel as a vector, so
// joining them with plain spaces loses the boundaries between them: an argument
// containing whitespace or a shell metacharacter reads back as several arguments,
// or as several commands.
func FormatCommand(command string, arg ...string) string {
	quoted := make([]string, 0, len(arg)+1)
	quoted = append(quoted, quoteArg(command))
	for _, a := range arg {
		quoted = append(quoted, quoteArg(a))
	}

	return strings.Join(quoted, " ")
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
