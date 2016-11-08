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
package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrimMachineID(t *testing.T) {
	testTrimMachineID(t, " 123 		", "123")
	testTrimMachineID(t, " 1234567890", "1234567890")
	testTrimMachineID(t, " 123456789012", "123456789012")
	testTrimMachineID(t, " 1234567890123", "123456789012")
	testTrimMachineID(t, "1234567890123", "123456789012")
	testTrimMachineID(t, "123456789012345678", "123456789012")
}

func testTrimMachineID(t *testing.T, input, expected string) {
	result := trimMachineID(input)
	assert.Equal(t, expected, result)
}
