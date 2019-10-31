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

// Package config provides methods for generating the Ceph config for a Ceph cluster and for
// producing a "ceph.conf" compatible file from the config as well as Ceph command line-compatible
// flags.
package osd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrderedCRUSHLabels(t *testing.T) {
	assert.Equal(t, "host", CRUSHMapLevelsOrdered[0])
	assert.Equal(t, "chassis", CRUSHMapLevelsOrdered[1])
	assert.Equal(t, "rack", CRUSHMapLevelsOrdered[2])
	assert.Equal(t, "row", CRUSHMapLevelsOrdered[3])
	assert.Equal(t, "pdu", CRUSHMapLevelsOrdered[4])
	assert.Equal(t, "pod", CRUSHMapLevelsOrdered[5])
	assert.Equal(t, "room", CRUSHMapLevelsOrdered[6])
	assert.Equal(t, "datacenter", CRUSHMapLevelsOrdered[7])
	assert.Equal(t, "zone", CRUSHMapLevelsOrdered[8])
	assert.Equal(t, "region", CRUSHMapLevelsOrdered[9])
}
