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

package rook

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLabelsApply(t *testing.T) {
	tcs := []struct {
		name     string
		target   *metav1.ObjectMeta
		input    Labels
		expected Labels
	}{
		{
			name:   "it should be able to update meta with no label",
			target: &metav1.ObjectMeta{},
			input: Labels{
				"foo": "bar",
			},
			expected: Labels{
				"foo": "bar",
			},
		},
		{
			name: "it should keep the original labels when new labels are set",
			target: &metav1.ObjectMeta{
				Labels: Labels{
					"foo": "bar",
				},
			},
			input: Labels{
				"hello": "world",
			},
			expected: Labels{
				"foo":   "bar",
				"hello": "world",
			},
		},
		{
			name: "it should NOT overwrite the existing keys",
			target: &metav1.ObjectMeta{
				Labels: Labels{
					"foo": "bar",
				},
			},
			input: Labels{
				"foo": "baz",
			},
			expected: Labels{
				"foo": "bar",
			},
		},
	}

	for _, tc := range tcs {
		tc.input.ApplyToObjectMeta(tc.target)
		assert.Equal(t, map[string]string(tc.expected), tc.target.Labels)
	}
}

func TestLabelsMerge(t *testing.T) {
	testLabelsPart1 := Labels{
		"foo":   "bar",
		"hello": "world",
	}
	testLabelsPart2 := Labels{
		"bar":   "foo",
		"hello": "earth",
	}
	expected := map[string]string{
		"foo":   "bar",
		"bar":   "foo",
		"hello": "world",
	}
	assert.Equal(t, expected, map[string]string(testLabelsPart1.Merge(testLabelsPart2)))

	// Test that nil Labels can still be appended to
	testLabelsPart3 := Labels{
		"hello": "world",
	}
	var empty Labels
	assert.Equal(t, map[string]string(testLabelsPart3), map[string]string(empty.Merge(testLabelsPart3)))
}
