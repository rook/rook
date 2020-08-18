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

package v1

import (
	"encoding/json"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLabelsSpec(t *testing.T) {
	specYaml := []byte(`
mgr:
  foo: bar
  hello: world
mon:
`)

	// convert the raw spec yaml into JSON
	rawJSON, err := yaml.YAMLToJSON(specYaml)
	assert.Nil(t, err)

	// unmarshal the JSON into a strongly typed Labels spec object
	var Labels LabelsSpec
	err = json.Unmarshal(rawJSON, &Labels)
	assert.Nil(t, err)

	// the unmarshalled Labels spec should equal the expected spec below
	expected := LabelsSpec{
		"mgr": map[string]string{
			"foo":   "bar",
			"hello": "world",
		},
		"mon": nil,
	}
	assert.Equal(t, expected, Labels)
}

func TestLabelsApply(t *testing.T) {
	objMeta := &metav1.ObjectMeta{}
	testLabels := Labels{
		"foo":   "bar",
		"hello": "world",
	}
	testLabels.ApplyToObjectMeta(objMeta)
	assert.Equal(t, testLabels.getMapStringString(), objMeta.Labels)

	testLabels["isthisatest"] = "test"
	testLabels.ApplyToObjectMeta(objMeta)
	assert.Equal(t, testLabels.getMapStringString(), objMeta.Labels)
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
	assert.Equal(t, expected, testLabelsPart1.Merge(testLabelsPart2).getMapStringString())

	// Test that nil Labels can still be appended to
	testLabelsPart3 := Labels{
		"hello": "world",
	}
	var empty Labels
	assert.Equal(t, map[string]string(testLabelsPart3), empty.Merge(testLabelsPart3).getMapStringString())
}

// getMapStringString return the Labels as a
func (a Labels) getMapStringString() map[string]string {
	res := map[string]string{}
	for k, v := range a {
		res[k] = v
	}
	return res
}
