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

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
)

func TestValidatePinningValues(t *testing.T) {
	// set Distributed correctly
	var testDistributedData cephv1.CephFilesystemSubVolumeGroupSpecPinning
	var testDistributedValue = 1
	testDistributedData.Distributed = &testDistributedValue
	err := validatePinningValues(testDistributedData)
	assert.NoError(t, err)

	// set Distributed wrongly
	var testDistributedData1 cephv1.CephFilesystemSubVolumeGroupSpecPinning
	var testDistributedValue1 = 5
	testDistributedData1.Distributed = &testDistributedValue1
	err = validatePinningValues(testDistributedData1)
	assert.Error(t, err)

	// set Random correctly
	var testRandomData cephv1.CephFilesystemSubVolumeGroupSpecPinning
	var testRandomdValue = 1.0
	testRandomData.Random = &testRandomdValue
	err = validatePinningValues(testRandomData)
	assert.NoError(t, err)

	// set Random wrongly
	var testRandomData1 cephv1.CephFilesystemSubVolumeGroupSpecPinning
	var testRandomdValue1 = 5.0
	testRandomData1.Random = &testRandomdValue1
	err = validatePinningValues(testRandomData1)
	assert.Error(t, err)

	// set export correctly
	var testExportData cephv1.CephFilesystemSubVolumeGroupSpecPinning
	var testExportValue = 1
	testExportData.Distributed = &testExportValue
	err = validatePinningValues(testExportData)
	assert.NoError(t, err)

	// set export wrongly
	var testExportData1 cephv1.CephFilesystemSubVolumeGroupSpecPinning
	var testExportValue1 = 500
	testExportData1.Distributed = &testExportValue1
	err = validatePinningValues(testExportData1)
	assert.Error(t, err)

	// more than one set at a time, error
	var testData cephv1.CephFilesystemSubVolumeGroupSpecPinning
	var testValue = 1
	testData.Distributed = &testValue
	testData.Export = &testValue
	err = validatePinningValues(testData)
	assert.Error(t, err)

	// nothing is set, noerror
	var testData1 cephv1.CephFilesystemSubVolumeGroupSpecPinning
	err = validatePinningValues(testData1)
	assert.NoError(t, err)
}
