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

func TestParseCephVolumeResult(t *testing.T) {
	executor := &exectest.MockExecutor{}
	// set up a mock function to return "rook owned" partitions on the device and it does not have a filesystem
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		logger.Infof("%s %+v", command, args)

		if command == "ceph-volume" {
			return cephVolumeTestResult, nil
		}

		return "", fmt.Errorf("unknown command %s %+v", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	osds, err := getCephVolumeOSDs(context, "rook")
	assert.Nil(t, err)
	require.NotNil(t, osds)
	assert.Equal(t, 2, len(osds))
}
