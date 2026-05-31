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
	"encoding/json"
	"os"
	"testing"

	osddaemon "github.com/rook/rook/pkg/daemon/ceph/osd"
	osdcfg "github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/stretchr/testify/assert"
)

func TestParseDesiredDevices(t *testing.T) {
	configuredDevices := []osdcfg.ConfiguredDevice{
		{
			ID: "sda",
			StoreConfig: osdcfg.StoreConfig{
				OSDsPerDevice: 1,
			},
		},
		{
			ID: "sdb",
			StoreConfig: osdcfg.StoreConfig{
				OSDsPerDevice: 1,
			},
		},
		{
			ID: "nvme01",
			StoreConfig: osdcfg.StoreConfig{
				OSDsPerDevice: 5,
			},
		},
	}
	marshalledDevices, err := json.Marshal(configuredDevices)
	assert.NoError(t, err)
	devices := string(marshalledDevices)

	result, err := parseDevices(devices)
	assert.NoError(t, err)
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
	configuredDevices = []osdcfg.ConfiguredDevice{
		{
			ID: "nvme01",
			StoreConfig: osdcfg.StoreConfig{
				OSDsPerDevice: -5,
			},
		},
	}
	marshalledDevices, err = json.Marshal(configuredDevices)
	assert.NoError(t, err)
	devices = string(marshalledDevices)

	result, err = parseDevices(devices)
	assert.Nil(t, result)
	assert.Error(t, err)

	// 0 osd count is not allowed
	configuredDevices = []osdcfg.ConfiguredDevice{
		{
			ID: "nvme01",
			StoreConfig: osdcfg.StoreConfig{
				OSDsPerDevice: 0,
			},
		},
	}
	marshalledDevices, err = json.Marshal(configuredDevices)
	assert.NoError(t, err)
	devices = string(marshalledDevices)

	result, err = parseDevices(devices)
	assert.Nil(t, result)
	assert.Error(t, err)

	// OSDsPerDevice, metadataDevice, databaseSizeMB and deviceClass
	configuredDevices = []osdcfg.ConfiguredDevice{
		{
			ID: "sdd",
			StoreConfig: osdcfg.StoreConfig{
				OSDsPerDevice:  1,
				DatabaseSizeMB: 2048,
				MetadataDevice: "sdb",
			},
		},
		{
			ID: "sde",
			StoreConfig: osdcfg.StoreConfig{
				OSDsPerDevice:  1,
				MetadataDevice: "sdb",
			},
		},
		{
			ID: "sdf",
			StoreConfig: osdcfg.StoreConfig{
				OSDsPerDevice:  1,
				MetadataDevice: "sdc",
			},
		},
		{
			ID: "sdg",
			StoreConfig: osdcfg.StoreConfig{
				OSDsPerDevice:  1,
				DeviceClass:    "tst",
				MetadataDevice: "sdc",
			},
		},
	}
	marshalledDevices, err = json.Marshal(configuredDevices)
	assert.NoError(t, err)
	devices = string(marshalledDevices)

	result, err = parseDevices(devices)
	assert.NoError(t, err)
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

	// check empty devices list
	result, err = parseDevices("")
	assert.NoError(t, err)
	assert.Equal(t, []osddaemon.DesiredDevice{}, result)
}

func TestReadSecretFile(t *testing.T) {
	// Fail if the file does not exist
	badPath := "/tmp/badpath"
	err := readCephSecret(badPath)
	assert.Error(t, err)

	// Fall back to the env var if the file is not found
	assert.NoError(t, os.Setenv(fallbackCephSecretEnvVar, "env-secret"))
	err = readCephSecret(badPath)
	assert.NoError(t, err)
	assert.Equal(t, "env-secret", clusterInfo.CephCred.Secret)
	os.Unsetenv(fallbackCephSecretEnvVar)

	// Create a temp file, but leave it empty
	path, err := os.CreateTemp("", "")
	assert.NoError(t, err)
	defer os.Remove(path.Name())

	// Fail if the secret file is empty
	err = readCephSecret(path.Name())
	assert.Error(t, err)
	assert.Equal(t, "", clusterInfo.CephCred.Secret)

	// Write a test keyring
	testSecret := "testkeyring"
	err = os.WriteFile(path.Name(), []byte(testSecret), 0o600)
	assert.NoError(t, err)

	// Read the secret from the file
	err = readCephSecret(path.Name())
	assert.NoError(t, err)
	assert.Equal(t, testSecret, clusterInfo.CephCred.Secret)
}
