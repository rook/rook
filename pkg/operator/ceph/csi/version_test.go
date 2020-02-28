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
	testVersion            = CephCSIVersion{2, 0, 0}
	testVersionUnsupported = CephCSIVersion{2, 1, 0}
)

func TestIsAtLeast(t *testing.T) {
	// Test version which is smaller
	var version = CephCSIVersion{1, 40, 10}
	ret := testVersion.isAtLeast(&version)
	assert.Equal(t, true, ret)

	// Test version which is equal
	ret = testVersion.isAtLeast(&testVersion)
	assert.Equal(t, true, ret)

	// Test version which is greater (minor)
	version = CephCSIVersion{2, 1, 0}
	ret = testVersion.isAtLeast(&version)
	assert.Equal(t, false, ret)

	// Test version which is greater (bugfix)
	version = CephCSIVersion{2, 0, 1}
	ret = testVersion.isAtLeast(&version)
	assert.Equal(t, false, ret)
}

func TestSupported(t *testing.T) {
	ret := testVersion.Supported()
	assert.Equal(t, true, ret)

	ret = testVersionUnsupported.Supported()
	assert.Equal(t, false, ret)
}

func Test_extractCephCSIVersion(t *testing.T) {
	expectedVersion := CephCSIVersion{2, 0, 0}
	csiString := []byte(`Cephcsi Version: v2.0.0
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
