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
	testMinVersion         = CephCSIVersion{3, 5, 0}
	testReleaseV340        = CephCSIVersion{3, 4, 0}
	testReleaseV350        = CephCSIVersion{3, 5, 0}
	testReleaseV360        = CephCSIVersion{3, 6, 0}
	testReleaseV361        = CephCSIVersion{3, 6, 1}
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

	// Test for 3.5.0
	// Test version which is lesser
	ret = testReleaseV350.isAtLeast(&testReleaseV340)
	assert.Equal(t, true, ret)

	// Test for 3.6.0
	ret = testReleaseV360.isAtLeast(&testReleaseV360)
	assert.Equal(t, true, ret)

}

func TestSupported(t *testing.T) {
	AllowUnsupported = false
	ret := testMinVersion.Supported()
	assert.Equal(t, true, ret)

	ret = testVersionUnsupported.Supported()
	assert.Equal(t, false, ret)

	ret = testReleaseV340.Supported()
	assert.Equal(t, false, ret)

	ret = testReleaseV350.Supported()
	assert.Equal(t, true, ret)

	ret = testReleaseV360.Supported()
	assert.Equal(t, true, ret)
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

func TestSupportsCustomCephConf(t *testing.T) {
	AllowUnsupported = true
	ret := testMinVersion.SupportsCustomCephConf()
	assert.True(t, ret)

	AllowUnsupported = false
	ret = testMinVersion.SupportsCustomCephConf()
	assert.True(t, ret)

	ret = testReleaseV340.SupportsCustomCephConf()
	assert.False(t, ret)

	ret = testReleaseV350.SupportsCustomCephConf()
	assert.True(t, ret)

	ret = testReleaseV360.SupportsCustomCephConf()
	assert.True(t, ret)
}

func TestSupportsMultus(t *testing.T) {
	t.Run("AllowUnsupported=true regardless of the version", func(t *testing.T) {
		AllowUnsupported = true
		ret := testMinVersion.SupportsMultus()
		assert.True(t, ret)
	})

	t.Run("AllowUnsupported=false and version 3.5 is too old", func(t *testing.T) {
		AllowUnsupported = false
		ret := testMinVersion.SupportsMultus()
		assert.False(t, ret)
	})

	t.Run("AllowUnsupported=false and version 3.6.1 is fine", func(t *testing.T) {
		AllowUnsupported = false
		ret := testReleaseV361.SupportsMultus()
		assert.True(t, ret)
	})
}
