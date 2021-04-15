/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package csi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	testMinVersion         = CephCSIVersion{2, 0, 0}
	testReleaseV210        = CephCSIVersion{2, 1, 0}
	testReleaseV300        = CephCSIVersion{3, 0, 0}
	testReleaseV320        = CephCSIVersion{3, 2, 0}
	testReleaseV321        = CephCSIVersion{3, 2, 1}
	testReleaseV330        = CephCSIVersion{3, 3, 0}
	testVersionUnsupported = CephCSIVersion{4, 0, 0}
)

func TestIsAtLeast(t *testing.T) {
	// Test version which is smaller
	var version = CephCSIVersion{1, 40, 10}
	ret := testMinVersion.isAtLeast(&version)
	assert.Equal(t, true, ret)

	// Test version which is equal
	ret = testMinVersion.isAtLeast(&testMinVersion)
	assert.Equal(t, true, ret)

	// Test version which is greater (minor)
	version = CephCSIVersion{2, 1, 0}
	ret = testMinVersion.isAtLeast(&version)
	assert.Equal(t, false, ret)

	// Test version which is greater (bugfix)
	version = CephCSIVersion{2, 2, 0}
	ret = testMinVersion.isAtLeast(&version)
	assert.Equal(t, false, ret)

	// Test for v2.1.0
	// Test version which is greater (bugfix)
	version = CephCSIVersion{2, 0, 1}
	ret = testReleaseV210.isAtLeast(&version)
	assert.Equal(t, true, ret)

	// Test version which is equal
	ret = testReleaseV210.isAtLeast(&testReleaseV210)
	assert.Equal(t, true, ret)

	// Test version which is greater (minor)
	version = CephCSIVersion{2, 1, 1}
	ret = testReleaseV210.isAtLeast(&version)
	assert.Equal(t, false, ret)

	// Test version which is greater (bugfix)
	version = CephCSIVersion{2, 2, 0}
	ret = testReleaseV210.isAtLeast(&version)
	assert.Equal(t, false, ret)

	// Test for 3.0.0
	// Test version which is equal
	ret = testReleaseV300.isAtLeast(&testReleaseV300)
	assert.Equal(t, true, ret)

	// Test for 3.3.0
	// Test version which is lesser
	ret = testReleaseV330.isAtLeast(&testReleaseV300)
	assert.Equal(t, true, ret)

	// Test version which is greater (minor)
	version = CephCSIVersion{3, 1, 1}
	ret = testReleaseV300.isAtLeast(&version)
	assert.Equal(t, false, ret)

	// Test version which is greater (bugfix)
	version = CephCSIVersion{3, 2, 0}
	ret = testReleaseV300.isAtLeast(&version)
	assert.Equal(t, false, ret)
}

func TestSupported(t *testing.T) {
	AllowUnsupported = false
	ret := testMinVersion.Supported()
	assert.Equal(t, true, ret)

	ret = testMinVersion.Supported()
	assert.Equal(t, true, ret)

	ret = testVersionUnsupported.Supported()
	assert.Equal(t, false, ret)
}

func TestSupportOMAPController(t *testing.T) {
	AllowUnsupported = true
	ret := testMinVersion.SupportsOMAPController()
	assert.True(t, ret)

	AllowUnsupported = false
	ret = testMinVersion.SupportsOMAPController()
	assert.False(t, ret)

	ret = testReleaseV300.SupportsOMAPController()
	assert.False(t, ret)

	ret = testReleaseV320.SupportsOMAPController()
	assert.True(t, ret)

	ret = testReleaseV321.SupportsOMAPController()
	assert.True(t, ret)

	ret = testReleaseV330.SupportsOMAPController()
	assert.True(t, ret)
}
func Test_extractCephCSIVersion(t *testing.T) {
	expectedVersion := CephCSIVersion{3, 0, 0}
	csiString := []byte(`Cephcsi Version: v3.0.0
		Git Commit: e58d537a07ca0184f67d33db85bf6b4911624b44
		Go Version: go1.12.15
		Compiler: gc
		Platform: linux/amd64
		`)
	version, err := extractCephCSIVersion(string(csiString))

	assert.Equal(t, &expectedVersion, version)
	assert.Nil(t, err)

	csiString = []byte(`Cephcsi Version: rubish
	Git Commit: e58d537a07ca0184f67d33db85bf6b4911624b44
	Go Version: go1.12.15
	Compiler: gc
	Platform: linux/amd64
	`)
	version, err = extractCephCSIVersion(string(csiString))

	assert.Nil(t, version)
	assert.Contains(t, err.Error(), "failed to parse version from")
}
