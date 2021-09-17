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

package v1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestPriorityClassNamesSpec(t *testing.T) {
	specYaml := []byte(`
all: all-class
mgr: mgr-class
mon: mon-class
osd: osd-class
`)

	// convert the raw spec yaml into JSON
	rawJSON, err := yaml.ToJSON(specYaml)
	assert.Nil(t, err)

	// unmarshal the JSON into a strongly typed annotations spec object
	var priorityClassNames PriorityClassNamesSpec
	err = json.Unmarshal(rawJSON, &priorityClassNames)
	assert.Nil(t, err)

	// the unmarshalled priority class names spec should equal the expected spec below
	expected := PriorityClassNamesSpec{
		"all": "all-class",
		"mgr": "mgr-class",
		"mon": "mon-class",
		"osd": "osd-class",
	}
	assert.Equal(t, expected, priorityClassNames)
}

func TestPriorityClassNamesDefaultToAll(t *testing.T) {
	priorityClassNames := PriorityClassNamesSpec{
		"all": "all-class",
		"mon": "mon-class",
	}

	assert.Equal(t, "all-class", priorityClassNames.All())
}
