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
	"path"
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/test"
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

func createPVCAvailableDevices() *DeviceOsdMapping {
	devices := &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data": {
				Data:     -1,
				Metadata: nil,
				Config: DesiredDevice{
					Name:               "/mnt/set1-data-0-rpf2k",
					OSDsPerDevice:      1,
					MetadataDevice:     "",
					DatabaseSizeMB:     0,
					DeviceClass:        "",
					IsFilter:           false,
					IsDevicePathFilter: false,
				},
				PersistentDevicePaths: []string{
					"/dev/rook-vg/rook-lv1",
					"/dev/mapper/rook--vg-rook--lv1",
					"/dev/disk/by-id/dm-name-rook--vg-rook--lv1",
					"/dev/disk/by-id/dm-uuid-LVM-4BOeIsrVP5O2J36cVqMSJNLEcwGIrqSF12RyWdpUaiCuAqOa1hAmD6EUYTO6dyD1",
				},
			},
		},
	}

	return devices
}

func TestConfigureCVDevices(t *testing.T) {
	originalLVMConfPath := lvmConfPath
	lvmConfPathTemp, err := ioutil.TempFile("", "lvmconf")
	if err != nil {
		t.Fatal(err)
	}
	lvmConfPath = lvmConfPathTemp.Name()
	defer func() {
		os.Remove(lvmConfPath)
		lvmConfPath = originalLVMConfPath
	}()

	originalCephConfigDir := cephConfigDir
	cephConfigDir, err = ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		os.RemoveAll(cephConfigDir)
		cephConfigDir = originalCephConfigDir
	}()

	nodeName := "set1-data-0-rpf2k"
	mountedDev := "/mnt/" + nodeName
	mapperDev := "/dev/mapper/rook--vg-rook--lv1"
	clusterFSID := "4bfe8b72-5e69-4330-b6c0-4d914db8ab89"
	osdUUID := "c03d7353-96e5-4a41-98de-830dfff97d06"
	lvBlockPath := "/dev/rook-vg/rook-lv1"

	// Test case for creating new raw mode OSD on LV-backed PVC
	{
		t.Log("Test case for creating new raw mode OSD on LV-backed PVC")
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommandWithOutputFile = func(command string, outFileArg string, args ...string) (string, error) {
			return "{\"key\":\"mysecurekey\"}", nil
		}
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			logger.Infof("[MockExecuteCommandWithOutput] %s %v", command, args)
			if command == "lsblk" && args[0] == mountedDev {
				return fmt.Sprintf(`SIZE="17179869184" ROTA="1" RO="0" TYPE="lvm" PKNAME="" NAME="%s" KNAME="/dev/dm-1, a ...interface{})`, mapperDev), nil
			}
			if command == "sgdisk" {
				return "Disk identifier (GUID): 18484D7E-5287-4CE9-AC73-D02FB69055CE", nil
			}
			if contains(args, "lvm") && contains(args, "list") {
				return `{}`, nil
			}
			if contains(args, "raw") && contains(args, "list") {
				return fmt.Sprintf(`{
				"0": {
					"ceph_fsid": "%s",
					"device": "%s",
					"osd_id": 0,
					"osd_uuid": "%s",
					"type": "bluestore"
				}
			}
			`, clusterFSID, mountedDev, osdUUID), nil
			}
			return "", errors.Errorf("unknown command %s %s", command, args)
		}
		executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
			logger.Infof("[MockExecuteCommandWithCombinedOutput] %s %v", command, args)
			if args[1] == "ceph-volume" && args[2] == "raw" && args[3] == "prepare" && args[4] == "--bluestore" && args[6] == mapperDev {
				return "", nil
			}
			if contains(args, "lvm") && contains(args, "list") {
				return `{}`, nil
			}
			return "", errors.Errorf("unknown command %s %s", command, args)
		}

		context := &clusterd.Context{Executor: executor, ConfigDir: cephConfigDir}
		clusterInfo := &cephclient.ClusterInfo{
			CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8},
			FSID:        clusterFSID,
		}
		agent := &OsdAgent{clusterInfo: clusterInfo, nodeName: nodeName, pvcBacked: true, storeConfig: config.StoreConfig{DeviceClass: "myds"}}
		devices := createPVCAvailableDevices()
		deviceOSDs, err := agent.configureCVDevices(context, devices)
		assert.Nil(t, err)
		assert.Len(t, deviceOSDs, 1)
		deviceOSD := deviceOSDs[0]
		logger.Infof("deviceOSDs: %+v", deviceOSDs)
		assert.Equal(t, osdUUID, deviceOSD.UUID)
		assert.Equal(t, mountedDev, deviceOSD.BlockPath)
		assert.Equal(t, true, deviceOSD.SkipLVRelease)
		assert.Equal(t, true, deviceOSD.LVBackedPV)
		assert.Equal(t, "raw", deviceOSD.CVMode)
		assert.Equal(t, "bluestore", deviceOSD.Store)
	}

	{
		// Test case for tending to create new lvm mode OSD on LV-backed PVC, but it catches an error
		t.Log("Test case for tending to create new lvm mode OSD on LV-backed PVC, but it catches an error")
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			logger.Infof("[MockExecuteCommandWithOutput] %s %v", command, args)
			if command == "lsblk" && args[0] == mountedDev {
				return fmt.Sprintf(`SIZE="17179869184" ROTA="1" RO="0" TYPE="lvm" PKNAME="" NAME="%s" KNAME="/dev/dm-1, a ...interface{})`, mapperDev), nil
			}
			return "", errors.Errorf("unknown command %s %s", command, args)
		}
		executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
			if command == "nsenter" {
				return "", nil
			}
			logger.Infof("[MockExecuteCommandWithCombinedOutput] %s %v", command, args)
			return "", errors.Errorf("unknown command %s %s", command, args)
		}

		context := &clusterd.Context{Executor: executor, ConfigDir: cephConfigDir}
		clusterInfo := &cephclient.ClusterInfo{
			CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 7}, // It doesn't support raw mode OSD
			FSID:        clusterFSID,
		}
		agent := &OsdAgent{clusterInfo: clusterInfo, nodeName: nodeName, pvcBacked: true}
		devices := createPVCAvailableDevices()

		_, err := agent.configureCVDevices(context, devices)

		assert.EqualError(t, err, "failed to initialize devices on PVC: OSD on LV-backed PVC requires new Ceph to use raw mode")
	}

	{
		// Test case for with no available lvm mode OSD and existing raw mode OSD on LV-backed PVC, it should return info of raw mode OSD
		t.Log("Test case for with no available lvm mode OSD and existing raw mode OSD on LV-backed PVC, it should return info of raw mode OSD")
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			logger.Infof("[MockExecuteCommandWithOutput] %s %v", command, args)
			if command == "lsblk" && args[0] == mountedDev {
				return fmt.Sprintf(`SIZE="17179869184" ROTA="1" RO="0" TYPE="lvm" PKNAME="" NAME="%s" KNAME="/dev/dm-1, a ...interface{})`, mapperDev), nil
			}
			if command == "sgdisk" {
				return "Disk identifier (GUID): 18484D7E-5287-4CE9-AC73-D02FB69055CE", nil
			}
			if args[1] == "ceph-volume" && args[4] == "lvm" && args[5] == "list" && args[6] == mapperDev {
				return `{}`, nil
			}
			if args[1] == "ceph-volume" && args[4] == "raw" && args[5] == "list" && args[6] == mountedDev {
				return fmt.Sprintf(`{
				"0": {
					"ceph_fsid": "%s",
					"device": "%s",
					"osd_id": 0,
					"osd_uuid": "%s",
					"type": "bluestore"
				}
			}
			`, clusterFSID, mountedDev, osdUUID), nil
			}
			return "", errors.Errorf("unknown command %s %s", command, args)
		}
		executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
			return "", errors.Errorf("unknown command %s %s", command, args)
		}

		context := &clusterd.Context{Executor: executor, ConfigDir: cephConfigDir}
		clusterInfo := &cephclient.ClusterInfo{
			CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8}, // It supports raw mode OSD
			FSID:        clusterFSID,
		}
		agent := &OsdAgent{clusterInfo: clusterInfo, nodeName: nodeName, pvcBacked: true}
		devices := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{}}

		deviceOSDs, err := agent.configureCVDevices(context, devices)

		assert.Nil(t, err)
		assert.Len(t, deviceOSDs, 1)
		deviceOSD := deviceOSDs[0]
		logger.Infof("deviceOSDs: %+v", deviceOSDs)
		assert.Equal(t, osdUUID, deviceOSD.UUID)
		assert.Equal(t, mountedDev, deviceOSD.BlockPath)
		assert.Equal(t, true, deviceOSD.SkipLVRelease)
		assert.Equal(t, true, deviceOSD.LVBackedPV)
		assert.Equal(t, "raw", deviceOSD.CVMode)
		assert.Equal(t, "bluestore", deviceOSD.Store)
	}

	{
		// Test case for a lvm mode OSD on LV-backed PVC, it should return info of lvm mode OSD
		t.Log("Test case for a lvm mode OSD on LV-backed PVC, it should return info of lvm mode OSD")
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			logger.Infof("[MockExecuteCommandWithOutput] %s %v", command, args)
			if command == "lsblk" && args[0] == mountedDev {
				return fmt.Sprintf(`SIZE="17179869184" ROTA="1" RO="0" TYPE="lvm" PKNAME="" NAME="%s" KNAME="/dev/dm-1"
	`, mapperDev), nil
			}
			if args[1] == "ceph-volume" && args[4] == "lvm" && args[5] == "list" {
				return fmt.Sprintf(`{
					"0": [
						{
							"devices": [
								"/dev/sdb"
							],
							"lv_name": "lv1",
							"lv_path": "%[1]s",
							"lv_size": "6.00g",
							"lv_tags": "ceph.block_device=%[1]s,ceph.block_uuid=hO8Hua-3H6B-qEt0-0NNN-ykFF-lsos-rSlmt2,ceph.cephx_lockbox_secret=,ceph.cluster_fsid=%[2]s,ceph.cluster_name=ceph,ceph.crush_device_class=None,ceph.encrypted=0,ceph.osd_fsid=%[3]s,ceph.osd_id=0,ceph.osdspec_affinity=,ceph.type=block,ceph.vdo=0",
							"lv_uuid": "hO8Hua-3H6B-qEt0-0NNN-ykFF-lsos-rSlmt2",
							"name": "lv1",
							"path": "%[1]s",
							"tags": {
								"ceph.block_device": "%[1]s",
								"ceph.block_uuid": "hO8Hua-3H6B-qEt0-0NNN-ykFF-lsos-rSlmt2",
								"ceph.cephx_lockbox_secret": "",
								"ceph.cluster_fsid": "%[2]s",
								"ceph.cluster_name": "ceph",
								"ceph.crush_device_class": "None",
								"ceph.encrypted": "0",
								"ceph.osd_fsid": "%[3]s",
								"ceph.osd_id": "0",
								"ceph.osdspec_affinity": "",
								"ceph.type": "block",
								"ceph.vdo": "0"
							},
							"type": "block",
							"vg_name": "test-vg"
						}
					]
				}
				`, lvBlockPath, clusterFSID, osdUUID), nil
			}

			return "", errors.Errorf("unknown command %s %s", command, args)
		}

		context := &clusterd.Context{Executor: executor, ConfigDir: cephConfigDir}
		clusterInfo := &cephclient.ClusterInfo{
			CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8}, // It supports raw mode OSD
			FSID:        clusterFSID,
		}
		agent := &OsdAgent{clusterInfo: clusterInfo, nodeName: nodeName, pvcBacked: true}
		devices := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{}}

		deviceOSDs, err := agent.configureCVDevices(context, devices)

		assert.Nil(t, err)
		assert.Len(t, deviceOSDs, 1)
		deviceOSD := deviceOSDs[0]
		logger.Infof("deviceOSDs: %+v", deviceOSDs)
		assert.Equal(t, osdUUID, deviceOSD.UUID)
		assert.Equal(t, lvBlockPath, deviceOSD.BlockPath)
		assert.Equal(t, true, deviceOSD.SkipLVRelease)
		assert.Equal(t, true, deviceOSD.LVBackedPV)
		assert.Equal(t, "lvm", deviceOSD.CVMode)
		assert.Equal(t, "bluestore", deviceOSD.Store)
	}

	{
		// Test case for a raw mode OSD
		t.Log("Test case for a raw mode OSD")
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			logger.Infof("[MockExecuteCommandWithOutput] %s %v", command, args)
			// get lsblk for disks from cephVolumeRAWTestResult var
			if command == "lsblk" && (args[0] == "/dev/vdb" || args[0] == "/dev/vdc") {
				return fmt.Sprintf(`SIZE="17179869184" ROTA="1" RO="0" TYPE="disk" PKNAME="" NAME="%s" KNAME="%s"`, args[0], args[0]), nil
			}
			if args[1] == "ceph-volume" && args[4] == "raw" && args[5] == "list" {
				return cephVolumeRAWTestResult, nil
			}
			if args[1] == "ceph-volume" && args[4] == "lvm" && args[5] == "list" {
				return `{}`, nil
			}
			if command == "sgdisk" {
				return "Disk identifier (GUID): 18484D7E-5287-4CE9-AC73-D02FB69055CE", nil
			}
			return "", errors.Errorf("unknown command %s %s", command, args)
		}
		deviceClassSet := false
		executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
			logger.Infof("[MockExecuteCommandWithCombinedOutput] %s %v", command, args)
			if args[1] == "ceph-volume" && args[2] == "raw" && args[3] == "prepare" && args[4] == "--bluestore" && args[7] == "--crush-device-class" {
				assert.Equal(t, "myclass", args[8])
				deviceClassSet = true
				return "", nil
			}
			return "", errors.Errorf("unknown command %s %s", command, args)
		}

		context := &clusterd.Context{Executor: executor, ConfigDir: cephConfigDir}
		clusterInfo := &cephclient.ClusterInfo{
			CephVersion: cephver.CephVersion{Major: 16, Minor: 2, Extra: 1}, // It supports raw mode OSD
			FSID:        clusterFSID,
		}
		agent := &OsdAgent{clusterInfo: clusterInfo, nodeName: nodeName, storeConfig: config.StoreConfig{DeviceClass: "myclass"}}
		devices := &DeviceOsdMapping{
			Entries: map[string]*DeviceOsdIDEntry{
				"vdb": {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/dev/vdb"}},
			},
		}
		_, err := agent.configureCVDevices(context, devices)
		assert.Nil(t, err)
		assert.True(t, deviceClassSet)
	}
}

func testBaseArgs(args []string) error {
	if args[1] == "ceph-volume" && args[2] == "--log-path" && args[3] == "/tmp/ceph-log" && args[4] == "lvm" && args[5] == "batch" && args[6] == "--prepare" && args[7] == "--bluestore" && args[8] == "--yes" {
		return nil
	}

	return errors.Errorf("unknown args %s ", args)
}

func TestInitializeBlock(t *testing.T) {
	// Common vars for all the tests
	devices := &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"sda": {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/dev/sda"}},
		},
	}

	// Test default behavior
	{
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommand = func(command string, args ...string) error {
			logger.Infof("%s %v", command, args)

			// Validate base common args
			err := testBaseArgs(args)
			if err != nil {
				return err
			}

			// First command
			if args[9] == "--osds-per-device" && args[10] == "1" && args[11] == "/dev/sda" {
				return nil
			}

			// Second command
			if args[9] == "--osds-per-device" && args[10] == "1" && args[11] == "/dev/sda" && args[12] == "--report" {
				return nil
			}

			return errors.Errorf("unknown command %s %s", command, args)
		}
		a := &OsdAgent{clusterInfo: &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8}}, nodeName: "node1"}
		context := &clusterd.Context{Executor: executor}

		err := a.initializeDevicesLVMMode(context, devices)
		assert.NoError(t, err, "failed default behavior test")
		logger.Info("success, go to next test")
	}

	// Test encryption behavior
	{
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommand = func(command string, args ...string) error {
			logger.Infof("%s %v", command, args)

			// Validate base common args
			err := testBaseArgs(args)
			if err != nil {
				return err
			}

			// First command
			if args[9] == "--dmcrypt" && args[10] == "--osds-per-device" && args[11] == "1" && args[12] == "/dev/sda" {
				return nil
			}

			// Second command
			if args[9] == "--dmcrypt" && args[10] == "--osds-per-device" && args[11] == "1" && args[12] == "/dev/sda" && args[13] == "--report" {
				return nil
			}

			return errors.Errorf("unknown command %s %s", command, args)
		}
		a := &OsdAgent{clusterInfo: &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8}}, nodeName: "node1", storeConfig: config.StoreConfig{EncryptedDevice: true}}
		context := &clusterd.Context{Executor: executor}

		err := a.initializeDevicesLVMMode(context, devices)
		assert.NoError(t, err, "failed encryption test")
		logger.Info("success, go to next test")
	}

	// Test multiple OSD per device
	{
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommand = func(command string, args ...string) error {
			logger.Infof("%s %v", command, args)

			// Validate base common args
			err := testBaseArgs(args)
			if err != nil {
				return err
			}

			// First command
			if args[9] == "--osds-per-device" && args[10] == "3" && args[11] == "/dev/sda" {
				return nil
			}

			// Second command
			if args[9] == "--osds-per-device" && args[10] == "3" && args[11] == "/dev/sda" && args[12] == "--report" {
				return nil
			}

			return errors.Errorf("unknown command %s %s", command, args)
		}
		a := &OsdAgent{clusterInfo: &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8}}, nodeName: "node1", storeConfig: config.StoreConfig{OSDsPerDevice: 3}}
		context := &clusterd.Context{Executor: executor}

		err := a.initializeDevicesLVMMode(context, devices)
		assert.NoError(t, err, "failed multiple osd test")
		logger.Info("success, go to next test")
	}

	// Test crush device class
	{
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommand = func(command string, args ...string) error {
			logger.Infof("%s %v", command, args)

			// Validate base common args
			err := testBaseArgs(args)
			if err != nil {
				return err
			}

			// First command
			if args[9] == "--osds-per-device" && args[10] == "1" && args[11] == "/dev/sda" && args[12] == "--crush-device-class" && args[13] == "hybrid" {
				return nil
			}

			// Second command
			if args[9] == "--osds-per-device" && args[10] == "1" && args[11] == "/dev/sda" && args[12] == "--crush-device-class" && args[13] == "hybrid" && args[14] == "--report" {
				return nil
			}

			return errors.Errorf("unknown command %s %s", command, args)
		}
		a := &OsdAgent{clusterInfo: &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8}}, nodeName: "node1", storeConfig: config.StoreConfig{DeviceClass: "hybrid"}}
		context := &clusterd.Context{Executor: executor}
		err := a.initializeDevicesLVMMode(context, devices)
		assert.NoError(t, err, "failed crush device class test")
		logger.Info("success, go to next test")
	}

	// Test with metadata devices
	{
		devices := &DeviceOsdMapping{
			Entries: map[string]*DeviceOsdIDEntry{
				"sda": {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/dev/sda", MetadataDevice: "sdb"}},
			},
		}

		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommand = func(command string, args ...string) error {
			logger.Infof("%s %v", command, args)

			// Validate base common args
			err := testBaseArgs(args)
			if err != nil {
				return err
			}

			// First command
			if args[9] == "--osds-per-device" && args[10] == "1" && args[11] == "/dev/sda" && args[12] == "--db-devices" && args[13] == "/dev/sdb" {
				return nil
			}

			// Second command
			if args[9] == "--osds-per-device" && args[10] == "1" && args[11] == "/dev/sda" && args[12] == "--db-devices" && args[13] == "/dev/sdb" && args[14] == "--report" {
				return nil
			}

			return errors.Errorf("unknown command %s %s", command, args)
		}

		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			logger.Infof("%s %v", command, args)

			// Validate base common args
			err := testBaseArgs(args)
			if err != nil {
				return "", err
			}

			// First command
			if args[9] == "--osds-per-device" && args[10] == "1" && args[11] == "/dev/sda" && args[12] == "--db-devices" && args[13] == "/dev/sdb" {
				return `{"vg": {"devices": "/dev/sdb"}}`, nil
			}

			return "", errors.Errorf("unknown command %s %s", command, args)
		}
		a := &OsdAgent{clusterInfo: &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8}}, nodeName: "node1"}
		context := &clusterd.Context{Executor: executor}

		err := a.initializeDevicesLVMMode(context, devices)
		assert.NoError(t, err, "failed metadata test")
		logger.Info("success, go to next test")
	}

	// Test with metadata devices with dev by-id
	{
		devices := &DeviceOsdMapping{
			Entries: map[string]*DeviceOsdIDEntry{
				"sda": {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/dev/sda", MetadataDevice: "/dev/disk/by-id/nvme-Samsung_SSD_970_EVO_Plus_1TB_XXX"}},
			},
		}

		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommand = func(command string, args ...string) error {
			logger.Infof("%s %v", command, args)

			// Validate base common args
			err := testBaseArgs(args)
			if err != nil {
				return err
			}

			// First command
			if args[9] == "--osds-per-device" && args[10] == "1" && args[11] == "/dev/sda" && args[12] == "--db-devices" && args[13] == "/dev/disk/by-id/nvme-Samsung_SSD_970_EVO_Plus_1TB_XXX" {
				return nil
			}

			// Second command
			if args[9] == "--osds-per-device" && args[10] == "1" && args[11] == "/dev/sda" && args[12] == "--db-devices" && args[13] == "/dev/disk/by-id/nvme-Samsung_SSD_970_EVO_Plus_1TB_XXX" && args[14] == "--report" {
				return nil
			}

			return errors.Errorf("unknown command %s %s", command, args)
		}

		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			logger.Infof("%s %v", command, args)

			// Validate base common args
			err := testBaseArgs(args)
			if err != nil {
				return "", err
			}

			// First command
			if args[9] == "--osds-per-device" && args[10] == "1" && args[11] == "/dev/sda" && args[12] == "--db-devices" && args[13] == "/dev/disk/by-id/nvme-Samsung_SSD_970_EVO_Plus_1TB_XXX" {
				return `{"vg": {"devices": "/dev/disk/by-id/nvme-Samsung_SSD_970_EVO_Plus_1TB_XXX"}}`, nil
			}

			return "", errors.Errorf("unknown command %s %s", command, args)
		}
		a := &OsdAgent{clusterInfo: &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8}}, nodeName: "node1"}
		context := &clusterd.Context{Executor: executor}

		err := a.initializeDevicesLVMMode(context, devices)
		assert.NoError(t, err, "failed metadata device by-id test")
		logger.Info("success, go to next test")
	}
}

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
	clusterInfo := &cephclient.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8},
	}
	a := &OsdAgent{clusterInfo: clusterInfo, nodeName: "node1"}
	devices := &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data": {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
		},
	}

	blockPath, metadataBlockPath, walBlockPath, err := a.initializeBlockPVC(context, devices, false)
	assert.Nil(t, err)
	assert.Equal(t, "/mnt/set1-data-0-rpf2k", blockPath)
	assert.Equal(t, "", metadataBlockPath)
	assert.Equal(t, "", walBlockPath)

	// Test for failure scenario by giving CephVersion{Major: 14, Minor: 2, Extra: 7}
	// instead of CephVersion{Major: 14, Minor: 2, Extra: 8}.
	clusterInfo = &cephclient.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 7},
	}
	a = &OsdAgent{clusterInfo: clusterInfo, nodeName: "node1"}
	devices = &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data": {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
		},
	}

	_, _, _, err = a.initializeBlockPVC(context, devices, false)
	assert.NotNil(t, err)

	executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		if args[1] == "ceph-volume" && args[2] == "lvm" && args[3] == "prepare" && args[4] == "--bluestore" {
			return initializeBlockPVCTestResult, nil
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	// Test with CephVersion{Major: 14, Minor: 2, Extra: 7} for argument lvm  without flag --crush-device-class.
	clusterInfo = &cephclient.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 7},
	}
	a = &OsdAgent{clusterInfo: clusterInfo, nodeName: "node1"}
	devices = &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data": {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
		},
	}

	blockPath, metadataBlockPath, walBlockPath, err = a.initializeBlockPVC(context, devices, false)
	assert.Nil(t, err)
	assert.Equal(t, "/dev/ceph-bceae560-85b1-4a87-9375-6335fb760c8c/osd-block-2ac8edb0-0d2e-4d8f-a6cc-4c972d56079c", blockPath)
	assert.Equal(t, "", metadataBlockPath)
	assert.Equal(t, "", walBlockPath)

	// Test for failure scenario by giving CephVersion{Major: 14, Minor: 2, Extra: 8}
	// instead of cephver.CephVersion{Major: 14, Minor: 2, Extra: 7}.
	clusterInfo = &cephclient.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8},
	}
	a = &OsdAgent{clusterInfo: clusterInfo, nodeName: "node1"}
	devices = &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data": {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
		},
	}

	_, _, _, err = a.initializeBlockPVC(context, devices, false)
	assert.NotNil(t, err)

	// Test for OSD on LV-backed PVC where Ceph does not support raw mode.
	// Expect no commands to be used.
	executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		return "", errors.Errorf("unknown command %s %s", command, args)
	}
	// Test with CephVersion{Major: 14, Minor: 2, Extra: 7}
	clusterInfo = &cephclient.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 7},
	}
	a = &OsdAgent{clusterInfo: clusterInfo, nodeName: "node1"}
	devices = &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data": {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
		},
	}
	_, _, _, err = a.initializeBlockPVC(context, devices, true)
	assert.NotNil(t, err)
	logger.Infof("error message %v", err)

	executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		if args[1] == "ceph-volume" && args[2] == "raw" && args[3] == "prepare" && args[4] == "--bluestore" && args[7] == "--crush-device-class" {
			assert.Equal(t, "foo", args[8])
			return initializeBlockPVCTestResult, nil
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}
	// Test with CephVersion{Major: 14, Minor: 2, Extra: 8} for argument raw  with flag --crush-device-class.
	os.Setenv(oposd.CrushDeviceClassVarName, "foo")
	defer os.Unsetenv(oposd.CrushDeviceClassVarName)
	clusterInfo = &cephclient.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8},
	}
	a = &OsdAgent{clusterInfo: clusterInfo, nodeName: "node1"}
	devices = &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data": {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
		},
	}

	blockPath, metadataBlockPath, walBlockPath, err = a.initializeBlockPVC(context, devices, false)
	assert.Nil(t, err)
	assert.Equal(t, "/mnt/set1-data-0-rpf2k", blockPath)
	assert.Equal(t, "", metadataBlockPath)
	assert.Equal(t, "", walBlockPath)

	// Test for condition when Data !=-1 with CephVersion{Major: 14, Minor: 2, Extra: 8} for raw  with flag --crush-device-class.
	devices = &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data": {Data: 0, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
		},
	}

	blockPath, metadataBlockPath, walBlockPath, err = a.initializeBlockPVC(context, devices, false)
	assert.Nil(t, err)
	assert.Equal(t, "", blockPath)
	assert.Equal(t, "", metadataBlockPath)
	assert.Equal(t, "", walBlockPath)
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
	clusterInfo := &cephclient.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8},
	}
	a := &OsdAgent{clusterInfo: clusterInfo, nodeName: "node1"}

	devices := &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data":     {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
			"metadata": {Data: 0, Metadata: []int{1}, Config: DesiredDevice{Name: "/srv/set1-metadata-0-8c7kr"}},
			"wal":      {Data: 1, Metadata: []int{2}, Config: DesiredDevice{Name: ""}},
		},
	}

	blockPath, metadataBlockPath, walBlockPath, err := a.initializeBlockPVC(context, devices, false)
	assert.Nil(t, err)
	assert.Equal(t, "/mnt/set1-data-0-rpf2k", blockPath)
	assert.Equal(t, "/srv/set1-metadata-0-8c7kr", metadataBlockPath)
	assert.Equal(t, "", walBlockPath)

	executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		if args[1] == "ceph-volume" && args[2] == "lvm" && args[3] == "prepare" && args[4] == "--bluestore" && args[7] == "--block.db" {
			return initializeBlockPVCTestResult, nil
		}
		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	// Test with CephVersion{Major: 14, Minor: 2, Extra: 7} for argument lvm with flag --block.db and without --crush-device-class  flag.
	clusterInfo = &cephclient.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 7},
	}
	a = &OsdAgent{clusterInfo: clusterInfo, nodeName: "node1"}

	devices = &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data":     {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
			"metadata": {Data: 0, Metadata: []int{1}, Config: DesiredDevice{Name: "/srv/set1-metadata-0-8c7kr"}},
			"wal":      {Data: 1, Metadata: []int{2}, Config: DesiredDevice{Name: ""}},
		},
	}

	blockPath, metadataBlockPath, walBlockPath, err = a.initializeBlockPVC(context, devices, false)
	assert.Nil(t, err)
	assert.Equal(t, "/dev/ceph-bceae560-85b1-4a87-9375-6335fb760c8c/osd-block-2ac8edb0-0d2e-4d8f-a6cc-4c972d56079c", blockPath)
	assert.Equal(t, "/srv/set1-metadata-0-8c7kr", metadataBlockPath)
	assert.Equal(t, "", walBlockPath)

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
	clusterInfo = &cephclient.ClusterInfo{
		CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8},
	}
	a = &OsdAgent{clusterInfo: clusterInfo, nodeName: "node1"}

	devices = &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"data":     {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/mnt/set1-data-0-rpf2k"}},
			"metadata": {Data: 0, Metadata: []int{1}, Config: DesiredDevice{Name: "/srv/set1-metadata-0-8c7kr"}},
			"wal":      {Data: 1, Metadata: []int{2}, Config: DesiredDevice{Name: ""}},
		},
	}

	blockPath, metadataBlockPath, walBlockPath, err = a.initializeBlockPVC(context, devices, false)
	assert.Nil(t, err)
	assert.Equal(t, "/mnt/set1-data-0-rpf2k", blockPath)
	assert.Equal(t, "/srv/set1-metadata-0-8c7kr", metadataBlockPath)
	assert.Equal(t, "", walBlockPath)
}

func TestParseCephVolumeLVMResult(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)

		logger.Infof("%s %v", command, args)
		if command == "stdbuf" {
			if args[4] == "lvm" && args[5] == "list" {
				return cephVolumeLVMTestResult, nil
			}
		}
		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	osds, err := GetCephVolumeLVMOSDs(context, &cephclient.ClusterInfo{Namespace: "name"}, "4bfe8b72-5e69-4330-b6c0-4d914db8ab89", "", false, false)
	assert.Nil(t, err)
	require.NotNil(t, osds)
	assert.Equal(t, 2, len(osds))
}

func TestParseCephVolumeRawResult(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		if command == "stdbuf" {
			if args[4] == "raw" && args[5] == "list" {
				return cephVolumeRAWTestResult, nil
			}
		}

		// get lsblk for disks from cephVolumeRAWTestResult var
		if command == "lsblk" && (args[0] == "/dev/vdb" || args[0] == "/dev/vdc") {
			return fmt.Sprintf(`SIZE="17179869184" ROTA="1" RO="0" TYPE="disk" PKNAME="" NAME="%s" KNAME="%s"`, args[0], args[0]), nil
		}
		if command == "sgdisk" {
			return "Disk identifier (GUID): 18484D7E-5287-4CE9-AC73-D02FB69055CE", nil
		}
		return "", errors.Errorf("unknown command: %s, args: %#v", command, args)
	}
	clusterInfo := &cephclient.ClusterInfo{Namespace: "name"}

	context := &clusterd.Context{Executor: executor, Clientset: test.New(t, 3)}
	osds, err := GetCephVolumeRawOSDs(context, clusterInfo, "4bfe8b72-5e69-4330-b6c0-4d914db8ab89", "", "", "", false, false)
	assert.Nil(t, err)
	require.NotNil(t, osds)
	assert.Equal(t, 2, len(osds))
}

func TestCephVolumeResultMultiClusterSingleOSD(t *testing.T) {
	executor := &exectest.MockExecutor{}
	// set up a mock function to return "rook owned" partitions on the device and it does not have a filesystem
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)

		if command == "stdbuf" {
			if args[4] == "lvm" && args[5] == "list" {
				return cephVolumeTestResultMultiCluster, nil
			}
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	osds, err := GetCephVolumeLVMOSDs(context, &cephclient.ClusterInfo{Namespace: "name"}, "451267e6-883f-4936-8dff-080d781c67d5", "", false, false)

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

		if command == "stdbuf" {
			if args[4] == "lvm" && args[5] == "list" {
				return cephVolumeTestResultMultiClusterMultiOSD, nil
			}
		}

		return "", errors.Errorf("unknown command %s% s", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	osds, err := GetCephVolumeLVMOSDs(context, &cephclient.ClusterInfo{Namespace: "name"}, "451267e6-883f-4936-8dff-080d781c67d5", "", false, false)
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

func TestGetEncryptedBlockPath(t *testing.T) {
	cvOp := `
2020-08-13 13:33:55.181541 D | exec: Running command: stdbuf -oL ceph-volume --log-path /var/log/ceph/set1-data-0-hfdc6 raw prepare --bluestore --data /dev/xvdce --crush-device-class hybriddu13 --dmcrypt --block.db /dev/xvdbb --block.wal /dev/xvdcu
2020-08-13 13:34:34.246638 I | cephosd: Running command: /usr/bin/ceph-authtool --gen-print-key
Running command: /usr/bin/ceph-authtool --gen-print-key
Running command: /usr/bin/ceph --cluster ceph --name client.bootstrap-osd --keyring /var/lib/ceph/bootstrap-osd/ceph.keyring -i - osd new e3c9ca4a-d00f-464b-9ac7-91fb151f6c8d
Running command: /usr/bin/ceph-authtool --gen-print-key
Running command: /usr/sbin/cryptsetup --batch-mode --key-file - luksFormat /dev/xvdce
Running command: /usr/sbin/cryptsetup --key-file - --allow-discards luksOpen /dev/xvdce ceph-e3c9ca4a-d00f-464b-9ac7-91fb151f6c8d-xvdce-block-dmcrypt
Running command: /usr/sbin/cryptsetup --batch-mode --key-file - luksFormat /dev/xvdcu
Running command: /usr/sbin/cryptsetup --key-file - --allow-discards luksOpen /dev/xvdcu ceph-e3c9ca4a-d00f-464b-9ac7-91fb151f6c8d-xvdcu-wal-dmcrypt
Running command: /usr/sbin/cryptsetup --batch-mode --key-file - luksFormat /dev/xvdbb
Running command: /usr/sbin/cryptsetup --key-file - --allow-discards luksOpen /dev/xvdbb ceph-e3c9ca4a-d00f-464b-9ac7-91fb151f6c8d-xvdbb-db-dmcrypt
Running command: /usr/bin/mount -t tmpfs tmpfs /var/lib/ceph/osd/ceph-2
Running command: /usr/bin/chown -R ceph:ceph /dev/mapper/ceph-e3c9ca4a-d00f-464b-9ac7-91fb151f6c8d-xvdce-block-dmcrypt
Running command: /usr/bin/ln -s /dev/mapper/ceph-e3c9ca4a-d00f-464b-9ac7-91fb151f6c8d-xvdce-block-dmcrypt /var/lib/ceph/osd/ceph-2/block
Running command: /usr/bin/ceph --cluster ceph --name client.bootstrap-osd --keyring /var/lib/ceph/bootstrap-osd/ceph.keyring mon getmap -o /var/lib/ceph/osd/ceph-2/activate.monmap`

	type args struct {
		op        string
		blockType string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"not-found", args{"Running command: /usr/bin/mount -t tmpfs tmpfs /var/lib/ceph/osd/ceph-1", "block-dmcrypt"}, ""},
		{"found-block", args{cvOp, "block-dmcrypt"}, "/dev/mapper/ceph-e3c9ca4a-d00f-464b-9ac7-91fb151f6c8d-xvdce-block-dmcrypt"},
		{"found-db", args{cvOp, "db-dmcrypt"}, "/dev/mapper/ceph-e3c9ca4a-d00f-464b-9ac7-91fb151f6c8d-xvdbb-db-dmcrypt"},
		{"found-wal", args{cvOp, "wal-dmcrypt"}, "/dev/mapper/ceph-e3c9ca4a-d00f-464b-9ac7-91fb151f6c8d-xvdcu-wal-dmcrypt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getEncryptedBlockPath(tt.args.op, tt.args.blockType); got != tt.want {
				t.Errorf("getEncryptedBlockPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNewStyledLvmBatch(t *testing.T) {
	newStyleLvmBatchVersion := cephver.CephVersion{Major: 14, Minor: 2, Extra: 15}
	legacyLvmBatchVersion := cephver.CephVersion{Major: 14, Minor: 2, Extra: 8}
	assert.Equal(t, true, isNewStyledLvmBatch(newStyleLvmBatchVersion))
	assert.Equal(t, false, isNewStyledLvmBatch(legacyLvmBatchVersion))
}

func TestInitializeBlockWithMD(t *testing.T) {
	// Common vars for all the tests
	devices := &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{
			"sda": {Data: -1, Metadata: nil, Config: DesiredDevice{Name: "/dev/sda", MetadataDevice: "/dev/sdd"}},
		},
	}

	// Test default behavior
	{
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommand = func(command string, args ...string) error {
			logger.Infof("%s %v", command, args)

			// Validate base common args
			err := testBaseArgs(args)
			if err != nil {
				return err
			}

			// Second command
			if args[9] == "--osds-per-device" && args[10] == "1" && args[11] == "/dev/sda" {
				return nil
			}

			return errors.Errorf("unknown command %s %s", command, args)
		}
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			// First command
			if args[9] == "--osds-per-device" && args[10] == "1" && args[11] == "/dev/sda" && args[12] == "--db-devices" && args[13] == "/dev/sdd" && args[14] == "--report" {
				return `[{"block_db": "/dev/sdd", "encryption": "None", "data": "/dev/sda", "data_size": "100.00 GB", "block_db_size": "100.00 GB"}]`, nil
			}

			return "", errors.Errorf("unknown command %s %s", command, args)
		}
		a := &OsdAgent{clusterInfo: &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 15}}, nodeName: "node1"}
		context := &clusterd.Context{Executor: executor}

		err := a.initializeDevicesLVMMode(context, devices)
		assert.NoError(t, err, "failed default behavior test")
	}
}

func TestUseRawMode(t *testing.T) {
	type fields struct {
		clusterInfo    *cephclient.ClusterInfo
		metadataDevice string
		storeConfig    config.StoreConfig
		pvcBacked      bool
	}
	type args struct {
		context   *clusterd.Context
		pvcBacked bool
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
	}{
		{"on pvc with lvm", fields{&cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 5}}, "", config.StoreConfig{}, true}, args{&clusterd.Context{}, true}, false, false},
		{"on pvc with raw", fields{&cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 8}}, "", config.StoreConfig{}, true}, args{&clusterd.Context{}, true}, true, false},
		{"non-pvc with lvm nautilus", fields{&cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 13}}, "", config.StoreConfig{}, false}, args{&clusterd.Context{}, false}, false, false},
		{"non-pvc with lvm octopus", fields{&cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 15, Minor: 2, Extra: 8}}, "", config.StoreConfig{}, false}, args{&clusterd.Context{}, false}, false, false},
		{"non-pvc with raw nautilus simple scenario supported", fields{&cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 14}}, "", config.StoreConfig{}, false}, args{&clusterd.Context{}, false}, true, false},
		{"non-pvc with raw octopus simple scenario supported", fields{&cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 15, Minor: 2, Extra: 9}}, "", config.StoreConfig{}, false}, args{&clusterd.Context{}, false}, true, false},
		{"non-pvc with raw pacific simple scenario supported", fields{&cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 16, Minor: 2, Extra: 1}}, "", config.StoreConfig{}, false}, args{&clusterd.Context{}, false}, true, false},
		{"non-pvc with lvm nautilus complex scenario not supported: encrypted", fields{&cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 14}}, "", config.StoreConfig{EncryptedDevice: true}, false}, args{&clusterd.Context{}, false}, false, false},
		{"non-pvc with lvm octopus complex scenario not supported: encrypted", fields{&cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 15, Minor: 2, Extra: 9}}, "", config.StoreConfig{EncryptedDevice: true}, false}, args{&clusterd.Context{}, false}, false, false},
		{"non-pvc with lvm nautilus complex scenario not supported: osd per device > 1", fields{&cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 14}}, "", config.StoreConfig{OSDsPerDevice: 2}, false}, args{&clusterd.Context{}, false}, false, false},
		{"non-pvc with lvm octopus complex scenario not supported: osd per device > 1", fields{&cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 15, Minor: 2, Extra: 9}}, "", config.StoreConfig{OSDsPerDevice: 2}, false}, args{&clusterd.Context{}, false}, false, false},
		{"non-pvc with lvm nautilus complex scenario not supported: metadata dev", fields{&cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 14}}, "/dev/sdb", config.StoreConfig{}, false}, args{&clusterd.Context{}, false}, false, false},
		{"non-pvc with lvm octopus complex scenario not supported: metadata dev", fields{&cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 15, Minor: 2, Extra: 9}}, "/dev/sdb", config.StoreConfig{}, false}, args{&clusterd.Context{}, false}, false, false},
		{"non-pvc with lvm pacific complex scenario not supported: metadata dev", fields{&cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 16, Minor: 2, Extra: 1}}, "/dev/sdb", config.StoreConfig{}, false}, args{&clusterd.Context{}, false}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &OsdAgent{
				clusterInfo:    tt.fields.clusterInfo,
				metadataDevice: tt.fields.metadataDevice,
				storeConfig:    tt.fields.storeConfig,
				pvcBacked:      tt.fields.pvcBacked,
			}
			got, err := a.useRawMode(tt.args.context, tt.args.pvcBacked)
			if (err != nil) != tt.wantErr {
				t.Errorf("OsdAgent.useRawMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("OsdAgent.useRawMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func contains(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}

func TestAppendOSDInfo(t *testing.T) {
	// Set 1: duplicate entries
	{
		currentOSDs := []oposd.OSDInfo{
			{ID: 0, Cluster: "ceph", UUID: "275950b5-dcb3-4c3e-b0df-014b16755dc5", DevicePartUUID: "", BlockPath: "/dev/ceph-48b22180-8358-4ab4-aec0-3fb83a20328b/osd-block-275950b5-dcb3-4c3e-b0df-014b16755dc5", MetadataPath: "", WalPath: "", SkipLVRelease: false, Location: "root=default host=minikube rack=rack1 zone=b", LVBackedPV: false, CVMode: "lvm", Store: "bluestore", TopologyAffinity: "topology.rook.io/rack=rack1"},
			{ID: 1, Cluster: "ceph", UUID: "3206c1c0-7ea2-412b-bd42-708cfe5e4acb", DevicePartUUID: "", BlockPath: "/dev/ceph-140a1344-636d-4442-85b3-bb3cd18ca002/osd-block-3206c1c0-7ea2-412b-bd42-708cfe5e4acb", MetadataPath: "", WalPath: "", SkipLVRelease: false, Location: "root=default host=minikube rack=rack1 zone=b", LVBackedPV: false, CVMode: "lvm", Store: "bluestore", TopologyAffinity: "topology.rook.io/rack=rack1"},
			{ID: 2, Cluster: "ceph", UUID: "7ea5e98b-755c-4837-a2a3-9ad61e67cf6f", DevicePartUUID: "", BlockPath: "/dev/ceph-0c466524-57a3-4e5f-b4e3-04538ff0aced/osd-block-7ea5e98b-755c-4837-a2a3-9ad61e67cf6f", MetadataPath: "", WalPath: "", SkipLVRelease: false, Location: "root=default host=minikube rack=rack1 zone=b", LVBackedPV: false, CVMode: "lvm", Store: "bluestore", TopologyAffinity: "topology.rook.io/rack=rack1"},
		}
		newOSDs := []oposd.OSDInfo{
			{ID: 2, Cluster: "ceph", UUID: "7ea5e98b-755c-4837-a2a3-9ad61e67cf6f", DevicePartUUID: "", BlockPath: "/dev/mapper/ceph--0c466524--57a3--4e5f--b4e3--04538ff0aced-osd--block--7ea5e98b--755c--4837--a2a3--9ad61e67cf6f", MetadataPath: "", WalPath: "", SkipLVRelease: true, Location: "root=default host=minikube rack=rack1 zone=b", LVBackedPV: false, CVMode: "raw", Store: "bluestore", TopologyAffinity: "topology.rook.io/rack=rack1"},
			{ID: 0, Cluster: "ceph", UUID: "275950b5-dcb3-4c3e-b0df-014b16755dc5", DevicePartUUID: "", BlockPath: "/dev/mapper/ceph--48b22180--8358--4ab4--aec0--3fb83a20328b-osd--block--275950b5--dcb3--4c3e--b0df--014b16755dc5", MetadataPath: "", WalPath: "", SkipLVRelease: true, Location: "root=default host=minikube rack=rack1 zone=b", LVBackedPV: false, CVMode: "raw", Store: "bluestore", TopologyAffinity: "topology.rook.io/rack=rack1"},
			{ID: 1, Cluster: "ceph", UUID: "3206c1c0-7ea2-412b-bd42-708cfe5e4acb", DevicePartUUID: "", BlockPath: "/dev/mapper/ceph--140a1344--636d--4442--85b3--bb3cd18ca002-osd--block--3206c1c0--7ea2--412b--bd42--708cfe5e4acb", MetadataPath: "", WalPath: "", SkipLVRelease: true, Location: "root=default host=minikube rack=rack1 zone=b", LVBackedPV: false, CVMode: "raw", Store: "bluestore", TopologyAffinity: "topology.rook.io/rack=rack1"},
		}
		trimmedOSDs := appendOSDInfo(currentOSDs, newOSDs)
		assert.Equal(t, 3, len(trimmedOSDs))
		assert.NotContains(t, trimmedOSDs[0].BlockPath, "mapper")
	}

	// Set 2: no duplicate entries, just a mix of RAW and LVM OSDs should not trim anything
	{
		currentOSDs := []oposd.OSDInfo{
			{ID: 0, Cluster: "ceph", UUID: "275950b5-dcb3-4c3e-b0df-014b16755dc5", DevicePartUUID: "", BlockPath: "/dev/ceph-48b22180-8358-4ab4-aec0-3fb83a20328b/osd-block-275950b5-dcb3-4c3e-b0df-014b16755dc5", MetadataPath: "", WalPath: "", SkipLVRelease: false, Location: "root=default host=minikube rack=rack1 zone=b", LVBackedPV: false, CVMode: "lvm", Store: "bluestore", TopologyAffinity: "topology.rook.io/rack=rack1"},
			{ID: 1, Cluster: "ceph", UUID: "3206c1c0-7ea2-412b-bd42-708cfe5e4acb", DevicePartUUID: "", BlockPath: "/dev/ceph-140a1344-636d-4442-85b3-bb3cd18ca002/osd-block-3206c1c0-7ea2-412b-bd42-708cfe5e4acb", MetadataPath: "", WalPath: "", SkipLVRelease: false, Location: "root=default host=minikube rack=rack1 zone=b", LVBackedPV: false, CVMode: "lvm", Store: "bluestore", TopologyAffinity: "topology.rook.io/rack=rack1"},
			{ID: 2, Cluster: "ceph", UUID: "7ea5e98b-755c-4837-a2a3-9ad61e67cf6f", DevicePartUUID: "", BlockPath: "/dev/ceph-0c466524-57a3-4e5f-b4e3-04538ff0aced/osd-block-7ea5e98b-755c-4837-a2a3-9ad61e67cf6f", MetadataPath: "", WalPath: "", SkipLVRelease: false, Location: "root=default host=minikube rack=rack1 zone=b", LVBackedPV: false, CVMode: "lvm", Store: "bluestore", TopologyAffinity: "topology.rook.io/rack=rack1"},
		}
		newOSDs := []oposd.OSDInfo{
			{ID: 3, Cluster: "ceph", UUID: "35e61dbc-4455-45fd-b5c8-39be2a29db02", DevicePartUUID: "", BlockPath: "/dev/sdb", MetadataPath: "", WalPath: "", SkipLVRelease: true, Location: "root=default host=minikube rack=rack1 zone=b", LVBackedPV: false, CVMode: "raw", Store: "bluestore", TopologyAffinity: "topology.rook.io/rack=rack1"},
			{ID: 4, Cluster: "ceph", UUID: "f5c0ce2c-76ee-4cbf-94df-9e480da6c614", DevicePartUUID: "", BlockPath: "/dev/sdd", MetadataPath: "", WalPath: "", SkipLVRelease: true, Location: "root=default host=minikube rack=rack1 zone=b", LVBackedPV: false, CVMode: "raw", Store: "bluestore", TopologyAffinity: "topology.rook.io/rack=rack1"},
			{ID: 5, Cluster: "ceph", UUID: "4aadb152-2b30-477a-963e-44447ded6a66", DevicePartUUID: "", BlockPath: "/dev/sde", MetadataPath: "", WalPath: "", SkipLVRelease: true, Location: "root=default host=minikube rack=rack1 zone=b", LVBackedPV: false, CVMode: "raw", Store: "bluestore", TopologyAffinity: "topology.rook.io/rack=rack1"},
		}
		trimmedOSDs := appendOSDInfo(currentOSDs, newOSDs)
		assert.Equal(t, 6, len(trimmedOSDs))
	}
	// Set 3: no current OSDs (no LVM, just RAW)
	{
		currentOSDs := []oposd.OSDInfo{}
		newOSDs := []oposd.OSDInfo{
			{ID: 3, Cluster: "ceph", UUID: "35e61dbc-4455-45fd-b5c8-39be2a29db02", DevicePartUUID: "", BlockPath: "/dev/sdb", MetadataPath: "", WalPath: "", SkipLVRelease: true, Location: "root=default host=minikube rack=rack1 zone=b", LVBackedPV: false, CVMode: "raw", Store: "bluestore", TopologyAffinity: "topology.rook.io/rack=rack1"},
			{ID: 4, Cluster: "ceph", UUID: "f5c0ce2c-76ee-4cbf-94df-9e480da6c614", DevicePartUUID: "", BlockPath: "/dev/sdd", MetadataPath: "", WalPath: "", SkipLVRelease: true, Location: "root=default host=minikube rack=rack1 zone=b", LVBackedPV: false, CVMode: "raw", Store: "bluestore", TopologyAffinity: "topology.rook.io/rack=rack1"},
			{ID: 5, Cluster: "ceph", UUID: "4aadb152-2b30-477a-963e-44447ded6a66", DevicePartUUID: "", BlockPath: "/dev/sde", MetadataPath: "", WalPath: "", SkipLVRelease: true, Location: "root=default host=minikube rack=rack1 zone=b", LVBackedPV: false, CVMode: "raw", Store: "bluestore", TopologyAffinity: "topology.rook.io/rack=rack1"},
		}
		trimmedOSDs := appendOSDInfo(currentOSDs, newOSDs)
		assert.Equal(t, 3, len(trimmedOSDs))
	}
}
