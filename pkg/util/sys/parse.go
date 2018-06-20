/*
Copyright 2016 The Rook Authors. All rights reserved.

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
package sys

import (
	"regexp"
	"strings"
)

// grep finds the *first* line that matches, rather than multiple lines
func Grep(input, searchFor string) string {
	logger.Debugf("grep. search=%s, input=%s", searchFor, input)
	if input == "" || searchFor == "" {
		return ""
	}
	for _, line := range strings.Split(input, "\n") {
		if matched, _ := regexp.MatchString(searchFor, line); matched {
			logger.Debugf("grep found line: %s", line)
			return line
		}
	}
	return ""
}

// awk finds the space-delimited token at the given position.
// 0 returns the whole line, while 1 is the first token.
func Awk(input string, position int, delimit string) string {
	logger.Debugf("awk %d from %s", position, input)
	if position == 0 {
		return input
	}

	words := strings.Split(strings.TrimSpace(input), delimit)
	position--
	if position < 0 || position >= len(words) {
		// out of range
		return ""
	}
	result := strings.TrimSpace(words[position])
	logger.Debugf("found awk string %s", result)
	return result
}
