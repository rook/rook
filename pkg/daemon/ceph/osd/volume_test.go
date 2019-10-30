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

package osd

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var cephVolumeTestResult = `{
    "0": [
        {
            "devices": [
                "/dev/sdb"
            ],
            "lv_name": "osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894",
            "lv_path": "/dev/ceph-93550251-f76c-4219-a33f-df8805de7b9e/osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894",
            "lv_size": "<8.00g",
            "lv_tags": "ceph.block_device=/dev/ceph-93550251-f76c-4219-a33f-df8805de7b9e/osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894,ceph.block_uuid=X39Wps-Qewq-d8LV-kj2p-ZqC3-IFQn-C35sV7,ceph.cephx_lockbox_secret=,ceph.cluster_fsid=4bfe8b72-5e69-4330-b6c0-4d914db8ab89,ceph.cluster_name=ceph,ceph.crush_device_class=None,ceph.encrypted=0,ceph.osd_fsid=dbe407e0-c1cb-495e-b30a-02e01de6c8ae,ceph.osd_id=0,ceph.type=block,ceph.vdo=0",
            "lv_uuid": "X39Wps-Qewq-d8LV-kj2p-ZqC3-IFQn-C35sV7",
            "name": "osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894",
            "path": "/dev/ceph-93550251-f76c-4219-a33f-df8805de7b9e/osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894",
            "tags": {
                "ceph.block_device": "/dev/ceph-93550251-f76c-4219-a33f-df8805de7b9e/osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894",
                "ceph.block_uuid": "X39Wps-Qewq-d8LV-kj2p-ZqC3-IFQn-C35sV7",
                "ceph.cephx_lockbox_secret": "",
                "ceph.cluster_fsid": "4bfe8b72-5e69-4330-b6c0-4d914db8ab89",
                "ceph.cluster_name": "ceph",
                "ceph.crush_device_class": "None",
                "ceph.encrypted": "0",
                "ceph.osd_fsid": "dbe407e0-c1cb-495e-b30a-02e01de6c8ae",
                "ceph.osd_id": "0",
                "ceph.type": "block",
                "ceph.vdo": "0"
            },
            "type": "block",
            "vg_name": "ceph-93550251-f76c-4219-a33f-df8805de7b9e"
        }
    ],
    "1": [
        {
            "devices": [
                "/dev/sdc"
            ],
            "lv_name": "osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
            "lv_path": "/dev/ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42/osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
            "lv_size": "<8.00g",
            "lv_tags": "ceph.block_device=/dev/ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42/osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0,ceph.block_uuid=tpdiTi-9Ozq-SrWi-p6od-LohX-s4U0-n2V0vk,ceph.cephx_lockbox_secret=,ceph.cluster_fsid=4bfe8b72-5e69-4330-b6c0-4d914db8ab89,ceph.cluster_name=ceph,ceph.crush_device_class=None,ceph.encrypted=0,ceph.osd_fsid=265d47ca-3e3c-4ef2-ac83-a44b7fb7feee,ceph.osd_id=1,ceph.type=block,ceph.vdo=0",
            "lv_uuid": "tpdiTi-9Ozq-SrWi-p6od-LohX-s4U0-n2V0vk",
            "name": "osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
            "path": "/dev/ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42/osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
            "tags": {
                "ceph.block_device": "/dev/ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42/osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
                "ceph.block_uuid": "tpdiTi-9Ozq-SrWi-p6od-LohX-s4U0-n2V0vk",
                "ceph.cephx_lockbox_secret": "",
                "ceph.cluster_fsid": "4bfe8b72-5e69-4330-b6c0-4d914db8ab89",
                "ceph.cluster_name": "ceph",
                "ceph.crush_device_class": "None",
                "ceph.encrypted": "0",
                "ceph.osd_fsid": "265d47ca-3e3c-4ef2-ac83-a44b7fb7feee",
                "ceph.osd_id": "1",
                "ceph.type": "block",
                "ceph.vdo": "0"
            },
            "type": "block",
            "vg_name": "ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42"
        }
    ]
}
`

var cephVolumeTestResultMultiCluster = `{
    "0": [
        {
            "devices": [
                "/dev/sdb"
            ],
            "lv_name": "osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894",
            "lv_path": "/dev/ceph-93550251-f76c-4219-a33f-df8805de7b9e/osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894",
            "lv_size": "<8.00g",
            "lv_tags": "ceph.block_device=/dev/ceph-93550251-f76c-4219-a33f-df8805de7b9e/osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894,ceph.block_uuid=X39Wps-Qewq-d8LV-kj2p-ZqC3-IFQn-C35sV7,ceph.cephx_lockbox_secret=,ceph.cluster_fsid=4bfe8b72-5e69-4330-b6c0-4d914db8ab89,ceph.cluster_name=ceph,ceph.crush_device_class=None,ceph.encrypted=0,ceph.osd_fsid=dbe407e0-c1cb-495e-b30a-02e01de6c8ae,ceph.osd_id=0,ceph.type=block,ceph.vdo=0",
            "lv_uuid": "X39Wps-Qewq-d8LV-kj2p-ZqC3-IFQn-C35sV7",
            "name": "osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894",
            "path": "/dev/ceph-93550251-f76c-4219-a33f-df8805de7b9e/osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894",
            "tags": {
                "ceph.block_device": "/dev/ceph-93550251-f76c-4219-a33f-df8805de7b9e/osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894",
                "ceph.block_uuid": "X39Wps-Qewq-d8LV-kj2p-ZqC3-IFQn-C35sV7",
                "ceph.cephx_lockbox_secret": "",
                "ceph.cluster_fsid": "451267e6-883f-4936-8dff-080d781c67d5",
                "ceph.cluster_name": "ceph",
                "ceph.crush_device_class": "None",
                "ceph.encrypted": "0",
                "ceph.osd_fsid": "dbe407e0-c1cb-495e-b30a-02e01de6c8ae",
                "ceph.osd_id": "0",
                "ceph.type": "block",
                "ceph.vdo": "0"
            },
            "type": "block",
            "vg_name": "ceph-93550251-f76c-4219-a33f-df8805de7b9e"
        },

        {
            "devices": [
                "/dev/sdc"
            ],
            "lv_name": "osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
            "lv_path": "/dev/ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42/osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
            "lv_size": "<8.00g",
            "lv_tags": "ceph.block_device=/dev/ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42/osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0,ceph.block_uuid=tpdiTi-9Ozq-SrWi-p6od-LohX-s4U0-n2V0vk,ceph.cephx_lockbox_secret=,ceph.cluster_fsid=4bfe8b72-5e69-4330-b6c0-4d914db8ab89,ceph.cluster_name=ceph,ceph.crush_device_class=None,ceph.encrypted=0,ceph.osd_fsid=265d47ca-3e3c-4ef2-ac83-a44b7fb7feee,ceph.osd_id=1,ceph.type=block,ceph.vdo=0",
            "lv_uuid": "tpdiTi-9Ozq-SrWi-p6od-LohX-s4U0-n2V0vk",
            "name": "osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
            "path": "/dev/ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42/osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
            "tags": {
                "ceph.block_device": "/dev/ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42/osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
                "ceph.block_uuid": "tpdiTi-9Ozq-SrWi-p6od-LohX-s4U0-n2V0vk",
                "ceph.cephx_lockbox_secret": "",
                "ceph.cluster_fsid": "4bfe8b72-5e69-4330-b6c0-4d914db8ab89",
                "ceph.cluster_name": "ceph",
                "ceph.crush_device_class": "None",
                "ceph.encrypted": "0",
                "ceph.osd_fsid": "265d47ca-3e3c-4ef2-ac83-a44b7fb7feee",
                "ceph.osd_id": "1",
                "ceph.type": "block",
                "ceph.vdo": "0"
            },
            "type": "block",
            "vg_name": "ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42"
        }
    ]
}
`
var cephVolumeTestResultMultiClusterMultiOSD = `{
    "0": [
        {
            "devices": [
                "/dev/sdb"
            ],
            "lv_name": "osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894",
            "lv_path": "/dev/ceph-93550251-f76c-4219-a33f-df8805de7b9e/osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894",
            "lv_size": "<8.00g",
            "lv_tags": "ceph.block_device=/dev/ceph-93550251-f76c-4219-a33f-df8805de7b9e/osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894,ceph.block_uuid=X39Wps-Qewq-d8LV-kj2p-ZqC3-IFQn-C35sV7,ceph.cephx_lockbox_secret=,ceph.cluster_fsid=4bfe8b72-5e69-4330-b6c0-4d914db8ab89,ceph.cluster_name=ceph,ceph.crush_device_class=None,ceph.encrypted=0,ceph.osd_fsid=dbe407e0-c1cb-495e-b30a-02e01de6c8ae,ceph.osd_id=0,ceph.type=block,ceph.vdo=0",
            "lv_uuid": "X39Wps-Qewq-d8LV-kj2p-ZqC3-IFQn-C35sV7",
            "name": "osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894",
            "path": "/dev/ceph-93550251-f76c-4219-a33f-df8805de7b9e/osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894",
            "tags": {
                "ceph.block_device": "/dev/ceph-93550251-f76c-4219-a33f-df8805de7b9e/osd-data-d1cb42c3-60f6-4347-82eb-3188dc3df894",
                "ceph.block_uuid": "X39Wps-Qewq-d8LV-kj2p-ZqC3-IFQn-C35sV7",
                "ceph.cephx_lockbox_secret": "",
                "ceph.cluster_fsid": "451267e6-883f-4936-8dff-080d781c67d5",
                "ceph.cluster_name": "ceph",
                "ceph.crush_device_class": "None",
                "ceph.encrypted": "0",
                "ceph.osd_fsid": "dbe407e0-c1cb-495e-b30a-02e01de6c8ae",
                "ceph.osd_id": "0",
                "ceph.type": "block",
                "ceph.vdo": "0"
            },
            "type": "block",
            "vg_name": "ceph-93550251-f76c-4219-a33f-df8805de7b9e"
        },

        {
            "devices": [
                "/dev/sdc"
            ],
            "lv_name": "osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
            "lv_path": "/dev/ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42/osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
            "lv_size": "<8.00g",
            "lv_tags": "ceph.block_device=/dev/ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42/osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0,ceph.block_uuid=tpdiTi-9Ozq-SrWi-p6od-LohX-s4U0-n2V0vk,ceph.cephx_lockbox_secret=,ceph.cluster_fsid=4bfe8b72-5e69-4330-b6c0-4d914db8ab89,ceph.cluster_name=ceph,ceph.crush_device_class=None,ceph.encrypted=0,ceph.osd_fsid=265d47ca-3e3c-4ef2-ac83-a44b7fb7feee,ceph.osd_id=1,ceph.type=block,ceph.vdo=0",
            "lv_uuid": "tpdiTi-9Ozq-SrWi-p6od-LohX-s4U0-n2V0vk",
            "name": "osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
            "path": "/dev/ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42/osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
            "tags": {
                "ceph.block_device": "/dev/ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42/osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
                "ceph.block_uuid": "tpdiTi-9Ozq-SrWi-p6od-LohX-s4U0-n2V0vk",
                "ceph.cephx_lockbox_secret": "",
                "ceph.cluster_fsid": "4bfe8b72-5e69-4330-b6c0-4d914db8ab89",
                "ceph.cluster_name": "ceph",
                "ceph.crush_device_class": "None",
                "ceph.encrypted": "0",
                "ceph.osd_fsid": "265d47ca-3e3c-4ef2-ac83-a44b7fb7feee",
                "ceph.osd_id": "1",
                "ceph.type": "block",
                "ceph.vdo": "0"
            },
            "type": "block",
            "vg_name": "ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42"
        }
    ],
    "1": [
        {
            "devices": [
                "/dev/sdc"
            ],
            "lv_name": "osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
            "lv_path": "/dev/ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42/osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
            "lv_size": "<8.00g",
            "lv_tags": "ceph.block_device=/dev/ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42/osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0,ceph.block_uuid=tpdiTi-9Ozq-SrWi-p6od-LohX-s4U0-n2V0vk,ceph.cephx_lockbox_secret=,ceph.cluster_fsid=4bfe8b72-5e69-4330-b6c0-4d914db8ab89,ceph.cluster_name=ceph,ceph.crush_device_class=None,ceph.encrypted=0,ceph.osd_fsid=265d47ca-3e3c-4ef2-ac83-a44b7fb7feee,ceph.osd_id=1,ceph.type=block,ceph.vdo=0",
            "lv_uuid": "tpdiTi-9Ozq-SrWi-p6od-LohX-s4U0-n2V0vk",
            "name": "osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
            "path": "/dev/ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42/osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
            "tags": {
                "ceph.block_device": "/dev/ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42/osd-data-5100eb6b-3a61-4fc1-80ee-86aec275b8b0",
                "ceph.block_uuid": "tpdiTi-9Ozq-SrWi-p6od-LohX-s4U0-n2V0vk",
                "ceph.cephx_lockbox_secret": "",
                "ceph.cluster_fsid": "4bfe8b72-5e69-4330-b6c0-4d914db8ab89",
                "ceph.cluster_name": "ceph",
                "ceph.crush_device_class": "None",
                "ceph.encrypted": "0",
                "ceph.osd_fsid": "265d47ca-3e3c-4ef2-ac83-a44b7fb7feee",
                "ceph.osd_id": "1",
                "ceph.type": "block",
                "ceph.vdo": "0"
            },
            "type": "block",
            "vg_name": "ceph-dfb1ca03-eb4f-4a5f-84b4-f4734aaefd42"
        }
    ]
}
`

func TestParseCephVolumeResult(t *testing.T) {
	executor := &exectest.MockExecutor{}
	// set up a mock function to return "rook owned" partitions on the device and it does not have a filesystem
	executor.MockExecuteCommandWithCombinedOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		logger.Infof("%s %+v", command, args)

		if command == "ceph-volume" {
			return cephVolumeTestResult, nil
		}

		return "", fmt.Errorf("unknown command %s %+v", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	osds, err := getCephVolumeOSDs(context, "rook", "4bfe8b72-5e69-4330-b6c0-4d914db8ab89", "", false)
	assert.Nil(t, err)
	require.NotNil(t, osds)
	assert.Equal(t, 2, len(osds))
}

func TestCephVolumeResultMultiClusterSingleOSD(t *testing.T) {
	executor := &exectest.MockExecutor{}
	// set up a mock function to return "rook owned" partitions on the device and it does not have a filesystem
	executor.MockExecuteCommandWithCombinedOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		logger.Infof("%s %+v", command, args)

		if command == "ceph-volume" {
			return cephVolumeTestResultMultiCluster, nil
		}

		return "", fmt.Errorf("unknown command %s %+v", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	osds, err := getCephVolumeOSDs(context, "rook", "451267e6-883f-4936-8dff-080d781c67d5", "", false)
	assert.Nil(t, err)
	require.NotNil(t, osds)
	assert.Equal(t, 1, len(osds))
	assert.Equal(t, osds[0].UUID, "dbe407e0-c1cb-495e-b30a-02e01de6c8ae")
}

func TestCephVolumeResultMultiClusterMultiOSD(t *testing.T) {
	executor := &exectest.MockExecutor{}
	// set up a mock function to return "rook owned" partitions on the device and it does not have a filesystem
	executor.MockExecuteCommandWithCombinedOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		logger.Infof("%s %+v", command, args)

		if command == "ceph-volume" {
			return cephVolumeTestResultMultiClusterMultiOSD, nil
		}

		return "", fmt.Errorf("unknown command %s %+v", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	osds, err := getCephVolumeOSDs(context, "rook", "451267e6-883f-4936-8dff-080d781c67d5", "", false)
	assert.Nil(t, err)
	require.NotNil(t, osds)
	assert.Equal(t, 1, len(osds))
	assert.Equal(t, osds[0].UUID, "dbe407e0-c1cb-495e-b30a-02e01de6c8ae")
}

func TestSanitizeOSDsPerDevice(t *testing.T) {
	assert.Equal(t, "1", sanitizeOSDsPerDevice(-1))
	assert.Equal(t, "1", sanitizeOSDsPerDevice(0))
	assert.Equal(t, "1", sanitizeOSDsPerDevice(1))
	assert.Equal(t, "2", sanitizeOSDsPerDevice(2))
}

func TestGetDatabaseSize(t *testing.T) {
	assert.Equal(t, 0, getDatabaseSize(0, 0))
	assert.Equal(t, 2048, getDatabaseSize(4096, 2048))
}

func TestHideRestoreconCommand(t *testing.T) {
	os.Setenv("ROOK_HOST_NETWORKING", "false")
	defer os.Setenv("ROOK_HOST_NETWORKING", "")

	// Should not run if ROOK_HOST_NETWORKING is false
	err := replaceRestoreconCommand()
	assert.NoError(t, err)

	// Should not run if /etc/redhat-release does not exist
	os.Setenv("ROOK_HOST_NETWORKING", "true")
	err = replaceRestoreconCommand()
	assert.NoError(t, err)

	// Should run now
	// Fake redhat-release file
	f, err := ioutil.TempFile("", "redhat-release")
	assert.NoError(t, err)
	defer f.Close()
	defer os.Remove(f.Name())
	redHatReleaseFile = f.Name()
	assert.FileExists(t, redHatReleaseFile)

	// Fake restorecon command
	ff, err := ioutil.TempFile("", "restorecon")
	defer ff.Close()
	defer os.Remove(ff.Name())
	assert.NoError(t, err)
	restoreconPath = ff.Name()
	assert.FileExists(t, restoreconPath)

	restoreconPathNewPath = restoreconPath + ".old"
	defer os.Remove(restoreconPathNewPath)
	err = replaceRestoreconCommand()
	assert.NoError(t, err)
	assert.FileExists(t, restoreconPathNewPath, "restoreconPath is %q and restoreconPathNewPath is %q", restoreconPath, restoreconPathNewPath)

	r, err := ioutil.ReadFile(restoreconPath)
	assert.NoError(t, err)

	b := strings.Contains(string(r), "restorecon command was replaced with a no-op")
	assert.True(t, b, restoreconPath)
}
