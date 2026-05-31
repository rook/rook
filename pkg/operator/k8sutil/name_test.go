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

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertDaemonID(t *testing.T) {
	testConvertDaemonName(t, "a", 0)
	testConvertDaemonName(t, "b", 1)
	testConvertDaemonName(t, "c", 2)
	testConvertDaemonName(t, "z", 25)
	testConvertDaemonName(t, "aa", 26)
	testConvertDaemonName(t, "ab", 27)
	testConvertDaemonName(t, "ac", 28)
	testConvertDaemonName(t, "az", 51)
	testConvertDaemonName(t, "ba", 52)
	testConvertDaemonName(t, "bb", 53)
	testConvertDaemonName(t, "bz", 77)
	testConvertDaemonName(t, "ca", 78)
	testConvertDaemonName(t, "za", 676)
	testConvertDaemonName(t, "aaa", 702)
	testConvertDaemonName(t, "aaz", 727)
	testConvertDaemonName(t, "aba", 728)
}

func testConvertDaemonName(t *testing.T, name string, index int) {
	// test that the conversion is valid from int to string
	assert.Equal(t, name, IndexToName(index))

	// test that the inverse conversion is valid from string to int
	i, err := NameToIndex(name)
	assert.Nil(t, err)
	assert.Equal(t, index, i)
}
