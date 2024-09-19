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

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestCephAnnotationsMerge(t *testing.T) {
	// No annotations defined
	testAnnotations := AnnotationsSpec{}
	a := GetOSDAnnotations(testAnnotations)
	assert.Nil(t, a)

	// Only a specific component annotations without "all"
	testAnnotations = AnnotationsSpec{
		"mgr":       {"mgrkey": "mgrval"},
		"mon":       {"monkey": "monval"},
		"osd":       {"osdkey": "osdval"},
		"rgw":       {"rgwkey": "rgwval"},
		"rbdmirror": {"rbdmirrorkey": "rbdmirrorval"},
	}
	a = GetMgrAnnotations(testAnnotations)
	assert.Equal(t, "mgrval", a["mgrkey"])
	assert.Equal(t, 1, len(a))
	a = GetMonAnnotations(testAnnotations)
	assert.Equal(t, "monval", a["monkey"])
	assert.Equal(t, 1, len(a))
	a = GetOSDAnnotations(testAnnotations)
	assert.Equal(t, "osdval", a["osdkey"])
	assert.Equal(t, 1, len(a))

	// No annotations matching the component
	testAnnotations = AnnotationsSpec{
		"mgr": {"mgrkey": "mgrval"},
	}
	a = GetMonAnnotations(testAnnotations)
	assert.Nil(t, a)

	// Merge with "all"
	testAnnotations = AnnotationsSpec{
		"all":            {"allkey1": "allval1", "allkey2": "allval2"},
		"mgr":            {"mgrkey": "mgrval"},
		"cmdreporter":    {"myversions": "detect"},
		"crashcollector": {"crash": "crashval"},
	}
	a = GetMonAnnotations(testAnnotations)
	assert.Equal(t, "allval1", a["allkey1"])
	assert.Equal(t, "allval2", a["allkey2"])
	assert.Equal(t, 2, len(a))
	a = GetMgrAnnotations(testAnnotations)
	assert.Equal(t, "mgrval", a["mgrkey"])
	assert.Equal(t, "allval1", a["allkey1"])
	assert.Equal(t, "allval2", a["allkey2"])
	assert.Equal(t, 3, len(a))
	b := GetCmdReporterAnnotations(testAnnotations)
	assert.Equal(t, "detect", b["myversions"])
	assert.Equal(t, "allval1", b["allkey1"])
	assert.Equal(t, "allval2", b["allkey2"])
	c := GetCrashCollectorAnnotations(testAnnotations)
	assert.Equal(t, "crashval", c["crash"])
	assert.Equal(t, "allval1", c["allkey1"])
	assert.Equal(t, "allval2", c["allkey2"])
}

func TestAnnotationsSpec(t *testing.T) {
	specYaml := []byte(`
mgr:
  foo: bar
  hello: world
mon:
`)

	// convert the raw spec yaml into JSON
	rawJSON, err := yaml.ToJSON(specYaml)
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
