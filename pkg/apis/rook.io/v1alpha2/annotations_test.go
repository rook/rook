/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package v1alpha2

import (
	"encoding/json"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnnotationsSpec(t *testing.T) {
	specYaml := []byte(`
mgr:
  foo: bar
  hello: world
mon:
`)

	// convert the raw spec yaml into JSON
	rawJSON, err := yaml.YAMLToJSON(specYaml)
	assert.Nil(t, err)

	// unmarshal the JSON into a strongly typed annotations spec object
	var annotations AnnotationsSpec
	err = json.Unmarshal(rawJSON, &annotations)
	assert.Nil(t, err)

	// the unmarshalled annotations spec should equal the expected spec below
	expected := AnnotationsSpec{
		"mgr": map[string]string{
			"foo":   "bar",
			"hello": "world",
		},
		"mon": nil,
	}
	assert.Equal(t, expected, annotations)
}

func TestAnnotations_ApplyToPodSpec(t *testing.T) {
	objMeta := &metav1.ObjectMeta{}
	testAnnotations := Annotations{
		"foo":   "bar",
		"hello": "world",
	}
	testAnnotations.ApplyToObjectMeta(objMeta)
	assert.Equal(t, testAnnotations.GetMapStringString(), objMeta.Annotations)

	testAnnotations["isthisatest"] = "test"
	testAnnotations.ApplyToObjectMeta(objMeta)
	assert.Equal(t, testAnnotations.GetMapStringString(), objMeta.Annotations)
}

func TestAnnotations_Merge(t *testing.T) {
	testAnnotationsPart1 := Annotations{
		"foo":   "bar",
		"hello": "world",
	}
	testAnnotationsPart2 := Annotations{
		"bar":   "foo",
		"hello": "earth",
	}
	assert.Equal(t, map[string]string{
		"foo":   "bar",
		"bar":   "foo",
		"hello": "world",
	}, testAnnotationsPart1.Merge(testAnnotationsPart2).GetMapStringString())
}
