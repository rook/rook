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
	assert.False(t, result[0].IsDevicePathFilter)
	assert.False(t, result[1].IsDevicePathFilter)
	assert.False(t, result[2].IsDevicePathFilter)

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

	// OSDsPerDevice, metadataDevice, databaseSizeMB and deviceClass
	devices = "sdd:1:2048::sdb,sde:1:::sdb,sdf:1:::sdc,sdg:1::tst:sdc"
	result, err = parseDevices(devices)
	assert.Equal(t, "sdd", result[0].Name)
	assert.Equal(t, "sde", result[1].Name)
	assert.Equal(t, "sdf", result[2].Name)
	assert.Equal(t, "sdg", result[3].Name)
	assert.Equal(t, 1, result[0].OSDsPerDevice)
	assert.Equal(t, 1, result[1].OSDsPerDevice)
	assert.Equal(t, 1, result[2].OSDsPerDevice)
	assert.Equal(t, 1, result[3].OSDsPerDevice)
	assert.Equal(t, 2048, result[0].DatabaseSizeMB)
	assert.Equal(t, 0, result[1].DatabaseSizeMB)
	assert.Equal(t, 0, result[2].DatabaseSizeMB)
	assert.Equal(t, 0, result[3].DatabaseSizeMB)
	assert.Equal(t, "", result[0].DeviceClass)
	assert.Equal(t, "", result[1].DeviceClass)
	assert.Equal(t, "", result[2].DeviceClass)
	assert.Equal(t, "tst", result[3].DeviceClass)
	assert.Equal(t, "sdb", result[0].MetadataDevice)
	assert.Equal(t, "sdb", result[1].MetadataDevice)
	assert.Equal(t, "sdc", result[2].MetadataDevice)
	assert.Equal(t, "sdc", result[3].MetadataDevice)
	assert.False(t, result[0].IsFilter)
	assert.False(t, result[1].IsFilter)
	assert.False(t, result[2].IsFilter)
	assert.False(t, result[3].IsFilter)
	assert.False(t, result[0].IsDevicePathFilter)
	assert.False(t, result[1].IsDevicePathFilter)
	assert.False(t, result[2].IsDevicePathFilter)
	assert.False(t, result[3].IsDevicePathFilter)

}

func TestDetectCrushLocation(t *testing.T) {
	location := []string{"host=foo"}
	nodeLabels := map[string]string{}

	// no change to the location if there are no labels
	updateLocationWithNodeLabels(&location, nodeLabels)
	assert.Equal(t, 1, len(location))
	assert.Equal(t, "host=foo", location[0])

	// no change to the location if an invalid label or invalid topology
	nodeLabels = map[string]string{
		"topology.rook.io/foo":          "bar",
		"invalid.topology.rook.io/rack": "r1",
		"topology.rook.io/zone":         "z1",
	}
	updateLocationWithNodeLabels(&location, nodeLabels)
	assert.Equal(t, 1, len(location))
	assert.Equal(t, "host=foo", location[0])

	// update the location with valid topology labels
	nodeLabels = map[string]string{
		"failure-domain.beta.kubernetes.io/region": "region1",
		"failure-domain.beta.kubernetes.io/zone":   "zone1",
		"topology.rook.io/rack":                    "rack1",
		"topology.rook.io/row":                     "row1",
	}

	expected := []string{
		"host=foo",
		"rack=rack1",
		"region=region1",
		"row=row1",
		"zone=zone1",
	}
	updateLocationWithNodeLabels(&location, nodeLabels)

	assert.Equal(t, 5, len(location))
	for i, locString := range location {
		assert.Equal(t, locString, expected[i])
	}
}
