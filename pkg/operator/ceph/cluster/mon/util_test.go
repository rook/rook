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
package mon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertMonID(t *testing.T) {
	assert.Equal(t, "a", indexToName(0))
	assert.Equal(t, "b", indexToName(1))
	assert.Equal(t, "c", indexToName(2))
	assert.Equal(t, "z", indexToName(25))
	assert.Equal(t, "aa", indexToName(26))
	assert.Equal(t, "ab", indexToName(27))
	assert.Equal(t, "ac", indexToName(28))
	assert.Equal(t, "az", indexToName(51))
	assert.Equal(t, "ba", indexToName(52))
	assert.Equal(t, "bb", indexToName(53))
	assert.Equal(t, "bz", indexToName(77))
	assert.Equal(t, "ca", indexToName(78))
	assert.Equal(t, "za", indexToName(676))
	assert.Equal(t, "aaa", indexToName(702))
	assert.Equal(t, "aaz", indexToName(727))
	assert.Equal(t, "aba", indexToName(728))
}
