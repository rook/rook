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

package client

import (
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestValidatePinningValues(t *testing.T) {
	// set Distributed correctly
	var testDistributedData cephv1.CephFilesystemSubVolumeGroupSpecPinning
	testDistributedValue := 1
	testDistributedData.Distributed = &testDistributedValue
	err := validatePinningValues(testDistributedData)
	assert.NoError(t, err)

	// set Distributed wrongly
	var testDistributedData1 cephv1.CephFilesystemSubVolumeGroupSpecPinning
	testDistributedValue1 := 5
	testDistributedData1.Distributed = &testDistributedValue1
	err = validatePinningValues(testDistributedData1)
	assert.Error(t, err)

	// set Random correctly
	var testRandomData cephv1.CephFilesystemSubVolumeGroupSpecPinning
	testRandomdValue := 1.0
	testRandomData.Random = &testRandomdValue
	err = validatePinningValues(testRandomData)
	assert.NoError(t, err)

	// set Random wrongly
	var testRandomData1 cephv1.CephFilesystemSubVolumeGroupSpecPinning
	testRandomdValue1 := 5.0
	testRandomData1.Random = &testRandomdValue1
	err = validatePinningValues(testRandomData1)
	assert.Error(t, err)

	// set export correctly
	var testExportData cephv1.CephFilesystemSubVolumeGroupSpecPinning
	testExportValue := 1
	testExportData.Distributed = &testExportValue
	err = validatePinningValues(testExportData)
	assert.NoError(t, err)

	// set export wrongly
	var testExportData1 cephv1.CephFilesystemSubVolumeGroupSpecPinning
	testExportValue1 := 500
	testExportData1.Distributed = &testExportValue1
	err = validatePinningValues(testExportData1)
	assert.Error(t, err)

	// more than one set at a time, error
	var testData cephv1.CephFilesystemSubVolumeGroupSpecPinning
	testValue := 1
	testData.Distributed = &testValue
	testData.Export = &testValue
	err = validatePinningValues(testData)
	assert.Error(t, err)

	// nothing is set, noerror
	var testData1 cephv1.CephFilesystemSubVolumeGroupSpecPinning
	err = validatePinningValues(testData1)
	assert.NoError(t, err)
}

func TestGetCephFSSubVolumeGroupInfo(t *testing.T) {
	tests := []struct {
		name             string
		output           string
		expectedQuota    int64
		expectedDataPool string
	}{
		{
			name:             "finite quota",
			output:           `{"bytes_quota":1099511627776,"bytes_used":0,"data_pool":"myfs-replicated"}`,
			expectedQuota:    1099511627776,
			expectedDataPool: "myfs-replicated",
		},
		{
			// Ceph reports bytes_quota as the string "infinite" when no quota is set.
			name:             "infinite quota",
			output:           `{"bytes_quota":"infinite","bytes_used":0,"data_pool":"myfs-replicated"}`,
			expectedQuota:    0,
			expectedDataPool: "myfs-replicated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &exectest.MockExecutor{}
			executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "fs" && args[1] == "subvolumegroup" && args[2] == "info" {
					return tt.output, nil
				}
				return "", errors.Errorf("unexpected ceph command %q", args)
			}

			svgInfo, err := getCephFSSubVolumeGroupInfo(&clusterd.Context{Executor: executor}, AdminTestClusterInfo("mycluster"), "myfs", "csi")
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedQuota, svgInfo.BytesQuota)
			assert.Equal(t, tt.expectedDataPool, svgInfo.DataPool)
		})
	}
}
