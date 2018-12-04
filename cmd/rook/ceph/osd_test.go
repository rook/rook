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

package ceph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDesiredDevices(t *testing.T) {
	devices := "sda,sdb,nvme01:5"
	result, err := parseDevices(devices)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(result))
	assert.Equal(t, "sda", result[0].Name)
	assert.Equal(t, "sdb", result[1].Name)
	assert.Equal(t, "nvme01", result[2].Name)
	assert.Equal(t, 1, result[0].OSDsPerDevice)
	assert.Equal(t, 1, result[1].OSDsPerDevice)
	assert.Equal(t, 5, result[2].OSDsPerDevice)
	assert.False(t, result[0].IsFilter)
	assert.False(t, result[1].IsFilter)
	assert.False(t, result[2].IsFilter)

	// negative osd count is not allowed
	devices = "nvme01:-5"
	result, err = parseDevices(devices)
	assert.Nil(t, result)
	assert.NotNil(t, err)

	// 0 osd count is not allowed
	devices = "nvme01:0"
	result, err = parseDevices(devices)
	assert.Nil(t, result)
	assert.NotNil(t, err)
}
