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
	"testing"

	rook "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/stretchr/testify/assert"
)

func TestAnnotationMerge(t *testing.T) {
	// No annotations defined
	testAnnotations := rook.AnnotationsSpec{}
	a := GetOSDAnnotations(testAnnotations)
	assert.Nil(t, a)

	// Only a specific component annotation without "all"
	testAnnotations = rook.AnnotationsSpec{
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
	a = GetRGWAnnotations(testAnnotations)
	assert.Equal(t, "rgwval", a["rgwkey"])
	assert.Equal(t, 1, len(a))
	a = GetRBDMirrorAnnotations(testAnnotations)
	assert.Equal(t, "rbdmirrorval", a["rbdmirrorkey"])
	assert.Equal(t, 1, len(a))

	// No annotations matching the component
	testAnnotations = rook.AnnotationsSpec{
		"mgr": {"mgrkey": "mgrval"},
	}
	a = GetMonAnnotations(testAnnotations)
	assert.Nil(t, a)

	// Merge with "all"
	testAnnotations = rook.AnnotationsSpec{
		"all": {"allkey1": "allval1", "allkey2": "allval2"},
		"mgr": {"mgrkey": "mgrval"},
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
}
