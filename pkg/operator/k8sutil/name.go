/*
Copyright 2018 The Rook Authors. All rights reserved.
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

package k8sutil

import "fmt"

const (
	maxPerChar = 26
)

// IndexToName converts an index to a daemon name based on as few letters of the alphabet as possible.
// For example:
//
//	0 -> a
//	1 -> b
//	25 -> z
//	26 -> aa
func IndexToName(index int) string {
	var result string
	for {
		i := index % maxPerChar
		c := 'z' - maxPerChar + i + 1
		result = fmt.Sprintf("%c%s", c, result)
		if index < maxPerChar {
			break
		}
		// subtract 1 since the character conversion is zero-based
		index = (index / maxPerChar) - 1
	}
	return result
}

// NameToIndex converts a daemon name to an index, which is the inverse of IndexToName
// For example:
//
//	a -> 0
//	b -> 1
func NameToIndex(name string) (int, error) {
	factor := 1
	for i := 1; i < len(name); i++ {
		factor *= maxPerChar
	}
	var result int
	for _, c := range name {
		charVal := int(maxPerChar - ('z' - c))
		if charVal < 1 || charVal > maxPerChar {
			return -1, fmt.Errorf("invalid char '%c' (%d) in %s", c, charVal, name)
		}
		if factor == 1 {
			// The least significant letter needs to be 0-based so we subtract 1
			result += charVal - 1
		} else {
			result += charVal * factor
		}
		factor /= maxPerChar
	}
	return result, nil

}
