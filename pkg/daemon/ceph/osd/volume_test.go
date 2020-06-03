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
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var initializeBlockPVCTestResult = `
 Volume group "ceph-bceae560-85b1-4a87-9375-6335fb760c8c" successfully created
 Logical volume "osd-block-2ac8edb0-0d2e-4d8f-a6cc-4c972d56079c" created.
`

var cephVolumeLVMTestResult = `{
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

var cephVolumeRAWTestResult = `{
    "0": {
        "ceph_fsid": "4bfe8b72-5e69-4330-b6c0-4d914db8ab89",
        "device": "/dev/vdb",
        "osd_id": 0,
        "osd_uuid": "c03d7353-96e5-4a41-98de-830dfff97d06",
        "type": "bluestore"
    },
    "1": {
        "ceph_fsid": "4bfe8b72-5e69-4330-b6c0-4d914db8ab89",
        "device": "/dev/vdc",
        "osd_id": 1,
        "osd_uuid": "62132914-e779-48cf-8f55-fbc9692c8ce5",
        "type": "bluestore"
    }
}
`

func TestInitializeBlockPVC(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		if args[1] == "ceph-volume" && args[2] == "raw" && args[3] == "prepare" && args[4] == "--bluestore" {
			return initializeBlockPVCTestResult, nil
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	// Test with CephVersion{Major: 14, Minor: 2, Extra: 8} for argument raw  without flag --crush-device-class.
	context := &clusterd.Context{Executor: executor}
	clusterInfo := &cephconfig.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8},
	}
	a := &OsdAgent{cluster: clusterInfo, nodeName: "node1"}
	devices := &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data": {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
		},
	}

	blockPath, metadataBlockPath, err := a.initializeBlockPVC(context, devices, false)
	assert.Nil(t, err)
	assert.Equal(t, "/mnt/set1-data-0-rpf2k", blockPath)
	assert.Equal(t, "", metadataBlockPath)

	// Test for failure scenario by giving CephVersion{Major: 14, Minor: 2, Extra: 7}
	// instead of CephVersion{Major: 14, Minor: 2, Extra: 8}.
	clusterInfo = &cephconfig.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 7},
	}
	a = &OsdAgent{cluster: clusterInfo, nodeName: "node1"}
	devices = &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data": {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
		},
	}

	blockPath, metadataBlockPath, err = a.initializeBlockPVC(context, devices, false)
	assert.NotNil(t, err)

	executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		if args[1] == "ceph-volume" && args[2] == "lvm" && args[3] == "prepare" && args[4] == "--bluestore" {
			return initializeBlockPVCTestResult, nil
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	// Test with CephVersion{Major: 14, Minor: 2, Extra: 7} for argument lvm  without flag --crush-device-class.
	clusterInfo = &cephconfig.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 7},
	}
	a = &OsdAgent{cluster: clusterInfo, nodeName: "node1"}
	devices = &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data": {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
		},
	}

	blockPath, metadataBlockPath, err = a.initializeBlockPVC(context, devices, false)
	assert.Nil(t, err)
	assert.Equal(t, "/dev/ceph-bceae560-85b1-4a87-9375-6335fb760c8c/osd-block-2ac8edb0-0d2e-4d8f-a6cc-4c972d56079c", blockPath)
	assert.Equal(t, "", metadataBlockPath)

	// Test for failure scenario by giving CephVersion{Major: 14, Minor: 2, Extra: 8}
	// instead of cephver.CephVersion{Major: 14, Minor: 2, Extra: 7}.
	clusterInfo = &cephconfig.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8},
	}
	a = &OsdAgent{cluster: clusterInfo, nodeName: "node1"}
	devices = &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data": {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
		},
	}

	blockPath, metadataBlockPath, err = a.initializeBlockPVC(context, devices, false)
	assert.NotNil(t, err)

	executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		if args[1] == "ceph-volume" && args[2] == "raw" && args[3] == "prepare" && args[4] == "--bluestore" && args[7] == "--crush-device-class" {
			return initializeBlockPVCTestResult, nil
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}
	// Test with CephVersion{Major: 14, Minor: 2, Extra: 8} for argument raw  with flag --crush-device-class.
	os.Setenv(oposd.CrushDeviceClassVarName, "foo")
	defer os.Unsetenv(oposd.CrushDeviceClassVarName)
	clusterInfo = &cephconfig.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8},
	}
	a = &OsdAgent{cluster: clusterInfo, nodeName: "node1"}
	devices = &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data": {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
		},
	}

	blockPath, metadataBlockPath, err = a.initializeBlockPVC(context, devices, false)
	assert.Nil(t, err)
	assert.Equal(t, "/mnt/set1-data-0-rpf2k", blockPath)
	assert.Equal(t, "", metadataBlockPath)

	// Test for condition when Data !=-1 with CephVersion{Major: 14, Minor: 2, Extra: 8} for raw  with flag --crush-device-class.
	devices = &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data": {Data: 0, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
		},
	}

	blockPath, metadataBlockPath, err = a.initializeBlockPVC(context, devices, false)
	assert.Nil(t, err)
	assert.Equal(t, "", blockPath)
	assert.Equal(t, "", metadataBlockPath)
}

func TestInitializeBlockPVCWithMetadata(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		if args[1] == "ceph-volume" && args[2] == "raw" && args[3] == "prepare" && args[4] == "--bluestore" && args[7] == "--block.db" {
			return initializeBlockPVCTestResult, nil
		}
		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	// Test with CephVersion{Major: 14, Minor: 2, Extra: 8} for argument raw with flag --block.db and without --crush-device-class flag.
	context := &clusterd.Context{Executor: executor}
	clusterInfo := &cephconfig.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8},
	}
	a := &OsdAgent{cluster: clusterInfo, nodeName: "node1"}

	devices := &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data":     {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
			"metadata": {Data: 0, Metadata: []int{1}, Config: DesiredDevice{Name: "/srv/set1-metadata-0-8c7kr"}},
		},
	}

	blockPath, metadataBlockPath, err := a.initializeBlockPVC(context, devices, false)
	assert.Nil(t, err)
	assert.Equal(t, "/mnt/set1-data-0-rpf2k", blockPath)
	assert.Equal(t, "/srv/set1-metadata-0-8c7kr", metadataBlockPath)

	executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		if args[1] == "ceph-volume" && args[2] == "lvm" && args[3] == "prepare" && args[4] == "--bluestore" && args[7] == "--block.db" {
			return initializeBlockPVCTestResult, nil
		}
		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	// Test with CephVersion{Major: 14, Minor: 2, Extra: 7} for argument lvm with flag --block.db and without --crush-device-class  flag.
	clusterInfo = &cephconfig.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 7},
	}
	a = &OsdAgent{cluster: clusterInfo, nodeName: "node1"}

	devices = &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data":     {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
			"metadata": {Data: 0, Metadata: []int{1}, Config: DesiredDevice{Name: "/srv/set1-metadata-0-8c7kr"}},
		},
	}

	blockPath, metadataBlockPath, err = a.initializeBlockPVC(context, devices, false)
	assert.Nil(t, err)
	assert.Equal(t, "/dev/ceph-bceae560-85b1-4a87-9375-6335fb760c8c/osd-block-2ac8edb0-0d2e-4d8f-a6cc-4c972d56079c", blockPath)
	assert.Equal(t, "/srv/set1-metadata-0-8c7kr", metadataBlockPath)

	executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		if args[1] == "ceph-volume" && args[2] == "raw" && args[3] == "prepare" && args[4] == "--bluestore" && args[7] == "--crush-device-class" && args[9] == "--block.db" {
			return initializeBlockPVCTestResult, nil
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	// Test with CephVersion{Major: 14, Minor: 2, Extra: 8} for argument raw with flag --block.db and --crush-device-class  flag.
	os.Setenv(oposd.CrushDeviceClassVarName, "foo")
	defer os.Unsetenv(oposd.CrushDeviceClassVarName)
	context = &clusterd.Context{Executor: executor}
	clusterInfo = &cephconfig.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8},
	}
	a = &OsdAgent{cluster: clusterInfo, nodeName: "node1"}

	devices = &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data":     {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
			"metadata": {Data: 0, Metadata: []int{1}, Config: DesiredDevice{Name: "/srv/set1-metadata-0-8c7kr"}},
		},
	}

	blockPath, metadataBlockPath, err = a.initializeBlockPVC(context, devices, false)
	assert.Nil(t, err)
	assert.Equal(t, "/mnt/set1-data-0-rpf2k", blockPath)
	assert.Equal(t, "/srv/set1-metadata-0-8c7kr", metadataBlockPath)
}

func TestParseCephVolumeLVMResult(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)

		if command == "ceph-volume" {
			return cephVolumeLVMTestResult, nil
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	osds, err := GetCephVolumeLVMOSDs(context, "rook", "4bfe8b72-5e69-4330-b6c0-4d914db8ab89", "", false, false)
	assert.Nil(t, err)
	require.NotNil(t, osds)
	assert.Equal(t, 2, len(osds))
}

func TestParseCephVolumeRawResult(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)

		if command == "ceph-volume" {
			return cephVolumeRAWTestResult, nil
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	osds, err := GetCephVolumeRawOSDs(context, "rook", "4bfe8b72-5e69-4330-b6c0-4d914db8ab89", "", "", false)
	assert.Nil(t, err)
	require.NotNil(t, osds)
	assert.Equal(t, 2, len(osds))
}

func TestCephVolumeResultMultiClusterSingleOSD(t *testing.T) {
	executor := &exectest.MockExecutor{}
	// set up a mock function to return "rook owned" partitions on the device and it does not have a filesystem
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)

		if command == "ceph-volume" {
			return cephVolumeTestResultMultiCluster, nil
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	osds, err := GetCephVolumeLVMOSDs(context, "rook", "451267e6-883f-4936-8dff-080d781c67d5", "", false, false)
	assert.Nil(t, err)
	require.NotNil(t, osds)
	assert.Equal(t, 1, len(osds))
	assert.Equal(t, osds[0].UUID, "dbe407e0-c1cb-495e-b30a-02e01de6c8ae")
}

func TestCephVolumeResultMultiClusterMultiOSD(t *testing.T) {
	executor := &exectest.MockExecutor{}
	// set up a mock function to return "rook owned" partitions on the device and it does not have a filesystem
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)

		if command == "ceph-volume" {
			return cephVolumeTestResultMultiClusterMultiOSD, nil
		}

		return "", errors.Errorf("unknown command %s% s", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	osds, err := GetCephVolumeLVMOSDs(context, "rook", "451267e6-883f-4936-8dff-080d781c67d5", "", false, false)
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

func TestPrintCVLogContent(t *testing.T) {
	tmp, err := ioutil.TempFile("", "cv-log")
	assert.Nil(t, err)

	defer os.Remove(tmp.Name())

	nodeName := "set1-2-data-jmxdx"
	cvLogDir = path.Join(tmp.Name(), nodeName)
	assert.Equal(t, path.Join(tmp.Name(), nodeName), cvLogDir)

	cvLogFilePath := path.Join(cvLogDir, "ceph-volume.log")
	assert.Equal(t, path.Join(cvLogDir, "ceph-volume.log"), cvLogFilePath)

	// Print c-v log, it is empty so this is similating a failure (e,g: the file does not exist)
	cvLog := readCVLogContent(tmp.Name())
	assert.Empty(t, cvLog, cvLog)

	// Write content in the file
	cvDummyLog := []byte(`dummy log`)
	_, err = tmp.Write(cvDummyLog)
	assert.NoError(t, err)
	// Print again, now there is content
	cvLog = readCVLogContent(tmp.Name())
	assert.NotEmpty(t, cvLog, cvLog)
}
