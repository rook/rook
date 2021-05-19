/*
Copyright 2021 The Rook Authors. All rights reserved.

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

func TestAnnotationsApply(t *testing.T) {
	objMeta := &metav1.ObjectMeta{}
	testAnnotations := Annotations{
		"foo":   "bar",
		"hello": "world",
	}
	testAnnotations.ApplyToObjectMeta(objMeta)
	assert.Equal(t, testAnnotations, Annotations(objMeta.Annotations))

	testAnnotations["isthisatest"] = "test"
	testAnnotations.ApplyToObjectMeta(objMeta)
	assert.Equal(t, testAnnotations, Annotations(objMeta.Annotations))
}

func TestAnnotationsMerge(t *testing.T) {
	testAnnotationsPart1 := Annotations{
		"foo":   "bar",
		"hello": "world",
	}
	testAnnotationsPart2 := Annotations{
		"bar":   "foo",
		"hello": "earth",
	}
	expected := map[string]string{
		"foo":   "bar",
		"bar":   "foo",
		"hello": "world",
	}
	assert.Equal(t, expected, map[string]string(testAnnotationsPart1.Merge(testAnnotationsPart2)))

	// Test that nil annotations can still be appended to
	testAnnotationsPart3 := Annotations{
		"hello": "world",
	}
	var empty Annotations
	assert.Equal(t, map[string]string(testAnnotationsPart3), map[string]string(empty.Merge(testAnnotationsPart3)))
}
