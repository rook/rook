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

package version

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToString(t *testing.T) {
	assert.Equal(t, "14.0.0-0 nautilus", fmt.Sprintf("%s", &Nautilus))
	assert.Equal(t, "13.0.0-0 mimic", fmt.Sprintf("%s", &Mimic))

	expected := fmt.Sprintf("-1.0.0-0 %s", unknownVersionString)
	assert.Equal(t, expected, fmt.Sprintf("%s", &CephVersion{-1, 0, 0, 0}))
}

func TestCephVersionFormatted(t *testing.T) {
	assert.Equal(t, "ceph version 14.0.0-0 nautilus", Nautilus.CephVersionFormatted())
	assert.Equal(t, "ceph version 13.0.0-0 mimic", Mimic.CephVersionFormatted())
}

func TestReleaseName(t *testing.T) {
	assert.Equal(t, "nautilus", Nautilus.ReleaseName())
	assert.Equal(t, "mimic", Mimic.ReleaseName())
	ver := CephVersion{-1, 0, 0, 0}
	assert.Equal(t, unknownVersionString, ver.ReleaseName())
}

func extractVersionHelper(t *testing.T, text string, major, minor, extra, build int) {
	v, err := ExtractCephVersion(text)
	if assert.NoError(t, err) {
		assert.Equal(t, *v, CephVersion{major, minor, extra, build})
	}
}

func TestExtractVersion(t *testing.T) {
	// release build
	v0c := "ceph version 13.2.6 (ae699615bac534ea496ee965ac6192cb7e0e07c1) mimic (stable)"
	v0d := `
root@7a97f5a78bc6:/# ceph --version
ceph version 13.2.6 (ae699615bac534ea496ee965ac6192cb7e0e07c1) mimic (stable)
`
	extractVersionHelper(t, v0c, 13, 2, 6, 0)
	extractVersionHelper(t, v0d, 13, 2, 6, 0)

	// development build
	v1c := "ceph version 14.1.33-403-g7ba6bece41"
	v1d := `
bin/ceph --version
*** DEVELOPER MODE: setting PATH, PYTHONPATH and LD_LIBRARY_PATH ***
ceph version 14.1.33-403-g7ba6bece41
(7ba6bece4187eda5d05a9b84211fe6ba8dd287bd) nautilus (rc)
`
	extractVersionHelper(t, v1c, 14, 1, 33, 403)
	extractVersionHelper(t, v1d, 14, 1, 33, 403)

	// build without git version info. it is possible to build the ceph tree
	// without a version number, but none of the container builds do this.
	// it is arguable that this should be a requirement since we are
	// explicitly adding fine-grained versioning to avoid issues with
	// release granularity. adding the reverse name-to-version is easy
	// enough if this ever becomes a need.
	v2c := "ceph version Development (no_version) nautilus (rc)"
	v2d := `
bin/ceph --version
*** DEVELOPER MODE: setting PATH, PYTHONPATH and LD_LIBRARY_PATH ***
ceph version Development (no_version) nautilus (rc)
`
	v, err := ExtractCephVersion(v2c)
	assert.Error(t, err)
	assert.Nil(t, v)

	v, err = ExtractCephVersion(v2d)
	assert.Error(t, err)
	assert.Nil(t, v)
}

func TestSupported(t *testing.T) {
	for _, v := range supportedVersions {
		assert.True(t, v.Supported())
	}
	for _, v := range unsupportedVersions {
		assert.False(t, v.Supported())
	}
}

func TestIsRelease(t *testing.T) {
	assert.True(t, Mimic.isRelease(Mimic))
	assert.True(t, Nautilus.isRelease(Nautilus))

	assert.False(t, Mimic.isRelease(Nautilus))

	MimicUpdate := Mimic
	MimicUpdate.Minor = 33
	MimicUpdate.Extra = 4
	assert.True(t, MimicUpdate.isRelease(Mimic))

	NautilusUpdate := Nautilus
	NautilusUpdate.Minor = 33
	NautilusUpdate.Extra = 4
	assert.True(t, NautilusUpdate.isRelease(Nautilus))
}

func TestIsReleaseX(t *testing.T) {
	assert.True(t, Mimic.IsMimic())
	assert.False(t, Nautilus.IsMimic())
	assert.False(t, Octopus.IsMimic())
}

func TestVersionAtLeast(t *testing.T) {
	assert.True(t, Mimic.IsAtLeast(Mimic))
	assert.False(t, Mimic.IsAtLeast(Nautilus))
	assert.False(t, Mimic.IsAtLeast(Octopus))
	assert.True(t, Nautilus.IsAtLeast(Mimic))
	assert.True(t, Nautilus.IsAtLeast(Nautilus))
	assert.False(t, Nautilus.IsAtLeast(Octopus))
	assert.True(t, Octopus.IsAtLeast(Mimic))
	assert.True(t, Octopus.IsAtLeast(Nautilus))
	assert.True(t, Octopus.IsAtLeast(Octopus))

	assert.True(t, (&CephVersion{1, 0, 0, 0}).IsAtLeast(CephVersion{0, 0, 0, 0}))
	assert.False(t, (&CephVersion{0, 0, 0, 0}).IsAtLeast(CephVersion{1, 0, 0, 0}))
	assert.True(t, (&CephVersion{1, 1, 0, 0}).IsAtLeast(CephVersion{1, 0, 0, 0}))
	assert.False(t, (&CephVersion{1, 0, 0, 0}).IsAtLeast(CephVersion{1, 1, 0, 0}))
	assert.True(t, (&CephVersion{1, 1, 1, 0}).IsAtLeast(CephVersion{1, 1, 0, 0}))
	assert.False(t, (&CephVersion{1, 1, 0, 0}).IsAtLeast(CephVersion{1, 1, 1, 0}))
	assert.True(t, (&CephVersion{1, 1, 1, 0}).IsAtLeast(CephVersion{1, 1, 1, 0}))
}

func TestVersionAtLeastX(t *testing.T) {
	assert.True(t, Octopus.IsAtLeastOctopus())
	assert.True(t, Octopus.IsAtLeastNautilus())
	assert.True(t, Octopus.IsAtLeastMimic())
	assert.True(t, Nautilus.IsAtLeastNautilus())
	assert.True(t, Nautilus.IsAtLeastMimic())
	assert.True(t, Mimic.IsAtLeastMimic())
	assert.True(t, Pacific.IsAtLeastPacific())
	assert.False(t, Mimic.IsAtLeastNautilus())
	assert.False(t, Nautilus.IsAtLeastOctopus())
	assert.False(t, Nautilus.IsAtLeastPacific())
}

func TestIsIdentical(t *testing.T) {
	assert.True(t, IsIdentical(CephVersion{14, 2, 2, 0}, CephVersion{14, 2, 2, 0}))
	assert.False(t, IsIdentical(CephVersion{14, 2, 2, 0}, CephVersion{15, 2, 2, 0}))
}

func TestIsSuperior(t *testing.T) {
	assert.False(t, IsSuperior(CephVersion{14, 2, 2, 0}, CephVersion{14, 2, 2, 0}))
	assert.False(t, IsSuperior(CephVersion{14, 2, 2, 0}, CephVersion{15, 2, 2, 0}))
	assert.True(t, IsSuperior(CephVersion{15, 2, 2, 0}, CephVersion{14, 2, 2, 0}))
	assert.True(t, IsSuperior(CephVersion{15, 2, 2, 0}, CephVersion{15, 1, 3, 0}))
	assert.True(t, IsSuperior(CephVersion{15, 2, 2, 0}, CephVersion{15, 2, 1, 0}))
	assert.True(t, IsSuperior(CephVersion{15, 2, 2, 1}, CephVersion{15, 2, 1, 0}))
}

func TestIsInferior(t *testing.T) {
	assert.False(t, IsInferior(CephVersion{14, 2, 2, 0}, CephVersion{14, 2, 2, 0}))
	assert.False(t, IsInferior(CephVersion{15, 2, 2, 0}, CephVersion{14, 2, 2, 0}))
	assert.True(t, IsInferior(CephVersion{14, 2, 2, 0}, CephVersion{15, 2, 2, 0}))
	assert.True(t, IsInferior(CephVersion{15, 1, 3, 0}, CephVersion{15, 2, 2, 0}))
	assert.True(t, IsInferior(CephVersion{15, 2, 1, 0}, CephVersion{15, 2, 2, 0}))
	assert.True(t, IsInferior(CephVersion{15, 2, 1, 0}, CephVersion{15, 2, 2, 1}))
}

func TestValidateCephVersionsBetweenLocalAndExternalClusters(t *testing.T) {
	// TEST 1: versions are identical
	localCephVersion := CephVersion{Major: 14, Minor: 2, Extra: 1}
	externalCephVersion := CephVersion{Major: 14, Minor: 2, Extra: 1}
	err := ValidateCephVersionsBetweenLocalAndExternalClusters(localCephVersion, externalCephVersion)
	assert.NoError(t, err)

	// TEST 2: local cluster version major is lower than external cluster version
	localCephVersion = CephVersion{Major: 14, Minor: 2, Extra: 1}
	externalCephVersion = CephVersion{Major: 15, Minor: 2, Extra: 1}
	err = ValidateCephVersionsBetweenLocalAndExternalClusters(localCephVersion, externalCephVersion)
	assert.NoError(t, err)

	// TEST 3: local cluster version major is higher than external cluster version
	localCephVersion = CephVersion{Major: 15, Minor: 2, Extra: 1}
	externalCephVersion = CephVersion{Major: 14, Minor: 2, Extra: 1}
	err = ValidateCephVersionsBetweenLocalAndExternalClusters(localCephVersion, externalCephVersion)
	assert.Error(t, err)

	// TEST 4: local version is > but from a minor release
	// local version must never be higher
	localCephVersion = CephVersion{Major: 14, Minor: 2, Extra: 2}
	externalCephVersion = CephVersion{Major: 14, Minor: 2, Extra: 1}
	err = ValidateCephVersionsBetweenLocalAndExternalClusters(localCephVersion, externalCephVersion)
	assert.Error(t, err)

	// TEST 5: external version is > but from a minor release
	localCephVersion = CephVersion{Major: 14, Minor: 2, Extra: 1}
	externalCephVersion = CephVersion{Major: 14, Minor: 2, Extra: 2}
	err = ValidateCephVersionsBetweenLocalAndExternalClusters(localCephVersion, externalCephVersion)
	assert.NoError(t, err)
}
