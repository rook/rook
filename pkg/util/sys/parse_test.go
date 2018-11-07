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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGrep(t *testing.T) {
	// single line
	testGrep(t, "", "", "")
	testGrep(t, "foo", "", "")
	testGrep(t, "food", "foo", "food")
	testGrep(t, "have you ever seen such a sight?", "^have", "have you ever seen such a sight?")
	testGrep(t, "have you ever seen such a sight?", "you", "have you ever seen such a sight?")
	testGrep(t, "have you ever seen such a sight?", "^you", "")

	// multi-line
	input := `This is a test
	of the emergency broadcast system.
	If the test fails,
    you need to fix it! `
	testGrep(t, input, "test", "This is a test")
	testGrep(t, input, "broadcast", "	of the emergency broadcast system.")
	testGrep(t, input, "fix", "    you need to fix it! ")
}

func testGrep(t *testing.T, input, searchFor, expected string) {
	output := Grep(input, searchFor)
	assert.Equal(t, expected, output)
}
