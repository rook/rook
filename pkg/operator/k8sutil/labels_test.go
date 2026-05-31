/*
Copyright 2020 The Rook Authors. All rights reserved.

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

// TestParseStringToLabels
func TestParseStringToLabels(t *testing.T) {
	results := map[string]map[string]string{
		"key1=value1": {"key1": "value1"},
		"key1=":       {"key1": ""},
		"key1":        {"key1": ""},
		"key2=value1,key3=value2": {
			"key2": "value1",
			"key3": "value2",
		},
		"": {},
	}
	for input, expected := range results {
		result := ParseStringToLabels(input)

		assert.Equal(t, expected, result)
	}
}
