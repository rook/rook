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
<<<<<<< HEAD
	assert.Equal(t, "17.0.0-0 quincy", Quincy.String())
=======
	assert.Equal(t, "19.0.0-0 squid", Squid.String())
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

	received := CephVersion{-1, 0, 0, 0, ""}
	expected := fmt.Sprintf("-1.0.0-0 %s", unknownVersionString)
	assert.Equal(t, expected, received.String())
}

func TestCephVersionFormatted(t *testing.T) {
<<<<<<< HEAD
	assert.Equal(t, "ceph version 17.0.0-0 quincy", Quincy.CephVersionFormatted())
}

func TestReleaseName(t *testing.T) {
	assert.Equal(t, "quincy", Quincy.ReleaseName())
=======
	assert.Equal(t, "ceph version 19.0.0-0 squid", Squid.CephVersionFormatted())
}

func TestReleaseName(t *testing.T) {
	assert.Equal(t, "squid", Squid.ReleaseName())
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
	ver := CephVersion{-1, 0, 0, 0, ""}
	assert.Equal(t, unknownVersionString, ver.ReleaseName())
}

func extractVersionHelper(t *testing.T, text string, major, minor, extra, build int, commitID string) {
	v, err := ExtractCephVersion(text)
	if assert.NoError(t, err) {
		assert.Equal(t, *v, CephVersion{major, minor, extra, build, commitID})
	}
}

func TestExtractVersion(t *testing.T) {
	// release build
	v0c := "ceph version 18.2.6 (ae699615bac534ea496ee965ac6192cb7e0e07c1) reef (stable)"
	v0d := `
root@7a97f5a78bc6:/# ceph --version
ceph version 18.2.6 (ae699615bac534ea496ee965ac6192cb7e0e07c1) reef (stable)
`
	extractVersionHelper(t, v0c, 18, 2, 6, 0, "ae699615bac534ea496ee965ac6192cb7e0e07c1")
	extractVersionHelper(t, v0d, 18, 2, 6, 0, "ae699615bac534ea496ee965ac6192cb7e0e07c1")

	// development build
	v1c := "ceph version 18.1.33-403-g7ba6bece41 (7ba6bece4187eda5d05a9b84211fe6ba8dd287bd) reef (rc)"
	v1d := `
bin/ceph --version
*** DEVELOPER MODE: setting PATH, PYTHONPATH and LD_LIBRARY_PATH ***
ceph version 18.1.33-403-g7ba6bece41
(7ba6bece4187eda5d05a9b84211fe6ba8dd287bd) reef (rc)
`
	extractVersionHelper(t, v1c, 18, 1, 33, 403, "7ba6bece4187eda5d05a9b84211fe6ba8dd287bd")
	extractVersionHelper(t, v1d, 18, 1, 33, 403, "7ba6bece4187eda5d05a9b84211fe6ba8dd287bd")

	// build without git version info. it is possible to build the ceph tree
	// without a version number, but none of the container builds do this.
	// it is arguable that this should be a requirement since we are
	// explicitly adding fine-grained versioning to avoid issues with
	// release granularity. adding the reverse name-to-version is easy
	// enough if this ever becomes a need.
	v2c := "ceph version Development (no_version) reef (rc)"
	v2d := `
bin/ceph --version
*** DEVELOPER MODE: setting PATH, PYTHONPATH and LD_LIBRARY_PATH ***
<<<<<<< HEAD
ceph version Development (no_version) quincy (rc)
=======
ceph version Development (no_version) reef (rc)
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
`
	v, err := ExtractCephVersion(v2c)
	assert.Error(t, err)
	assert.Nil(t, v)

	v, err = ExtractCephVersion(v2d)
	assert.Error(t, err)
	assert.Nil(t, v)

	// Test the round trip for serializing and deserializing the version
	v3c := "ceph version 18.2.5-1 reef"
	v, err = ExtractCephVersion(v3c)
	assert.NoError(t, err)
	assert.NotNil(t, v)
	assert.Equal(t, "18.2.5-1 reef", v.String())
}

func TestSupported(t *testing.T) {
	for _, v := range supportedVersions {
		assert.True(t, v.Supported())
	}
	assert.False(t, Tentacle.Supported())
}

func TestIsRelease(t *testing.T) {
<<<<<<< HEAD
	assert.True(t, Quincy.isRelease(Quincy))
	assert.True(t, Reef.isRelease(Reef))

	assert.False(t, Reef.isRelease(Quincy))

	QuincyUpdate := Quincy
	QuincyUpdate.Minor = 33
	QuincyUpdate.Extra = 4
	assert.True(t, QuincyUpdate.isRelease(Quincy))
}

func TestIsReleaseX(t *testing.T) {
	assert.False(t, Quincy.IsReef())
}

func TestVersionAtLeast(t *testing.T) {
	assert.True(t, Quincy.IsAtLeast(Quincy))
=======
	assert.True(t, Reef.isRelease(Reef))
	assert.True(t, Squid.isRelease(Squid))

	assert.False(t, Reef.isRelease(Squid))

	ReefUpdate := Reef
	ReefUpdate.Minor = 33
	ReefUpdate.Extra = 4
	assert.True(t, ReefUpdate.isRelease(Reef))
}

func TestIsReleaseX(t *testing.T) {
	assert.False(t, Squid.IsReef())
}

func TestVersionAtLeast(t *testing.T) {
	assert.True(t, Squid.IsAtLeast(Squid))
	assert.True(t, Squid.IsAtLeast(Reef))
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

	assert.True(t, (&CephVersion{1, 0, 0, 0, ""}).IsAtLeast(CephVersion{0, 0, 0, 0, ""}))
	assert.False(t, (&CephVersion{0, 0, 0, 0, ""}).IsAtLeast(CephVersion{1, 0, 0, 0, ""}))
	assert.True(t, (&CephVersion{1, 1, 0, 0, ""}).IsAtLeast(CephVersion{1, 0, 0, 0, ""}))
	assert.False(t, (&CephVersion{1, 0, 0, 0, ""}).IsAtLeast(CephVersion{1, 1, 0, 0, ""}))
	assert.True(t, (&CephVersion{1, 1, 1, 0, ""}).IsAtLeast(CephVersion{1, 1, 0, 0, ""}))
	assert.False(t, (&CephVersion{1, 1, 0, 0, ""}).IsAtLeast(CephVersion{1, 1, 1, 0, ""}))
	assert.True(t, (&CephVersion{1, 1, 1, 0, ""}).IsAtLeast(CephVersion{1, 1, 1, 0, ""}))
}

func TestVersionAtLeastX(t *testing.T) {
<<<<<<< HEAD
	assert.True(t, Quincy.IsAtLeastQuincy())
	assert.False(t, Quincy.IsAtLeastReef())
=======
	assert.True(t, Reef.IsAtLeastReef())
	assert.False(t, Reef.IsAtLeastSquid())
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
}

func TestIsIdentical(t *testing.T) {
	assert.True(t, IsIdentical(CephVersion{15, 2, 2, 0, ""}, CephVersion{15, 2, 2, 0, ""}))
	assert.False(t, IsIdentical(CephVersion{16, 2, 2, 0, ""}, CephVersion{15, 2, 2, 0, ""}))
}

func TestIsSuperior(t *testing.T) {
	assert.False(t, IsSuperior(CephVersion{15, 2, 2, 0, ""}, CephVersion{15, 2, 2, 0, ""}))
	assert.False(t, IsSuperior(CephVersion{15, 2, 2, 0, ""}, CephVersion{16, 2, 2, 0, ""}))
	assert.True(t, IsSuperior(CephVersion{16, 2, 2, 0, ""}, CephVersion{15, 2, 2, 0, ""}))
	assert.True(t, IsSuperior(CephVersion{15, 2, 2, 0, ""}, CephVersion{15, 1, 3, 0, ""}))
	assert.True(t, IsSuperior(CephVersion{15, 2, 2, 0, ""}, CephVersion{15, 2, 1, 0, ""}))
	assert.True(t, IsSuperior(CephVersion{15, 2, 2, 1, ""}, CephVersion{15, 2, 1, 0, ""}))
}

func TestIsInferior(t *testing.T) {
	assert.False(t, IsInferior(CephVersion{15, 2, 2, 0, ""}, CephVersion{15, 2, 2, 0, ""}))
	assert.False(t, IsInferior(CephVersion{16, 2, 2, 0, ""}, CephVersion{15, 2, 2, 0, ""}))
	assert.True(t, IsInferior(CephVersion{15, 2, 2, 0, ""}, CephVersion{16, 2, 2, 0, ""}))
	assert.True(t, IsInferior(CephVersion{15, 1, 3, 0, ""}, CephVersion{15, 2, 2, 0, ""}))
	assert.True(t, IsInferior(CephVersion{15, 2, 1, 0, ""}, CephVersion{15, 2, 2, 0, ""}))
	assert.True(t, IsInferior(CephVersion{15, 2, 1, 0, ""}, CephVersion{15, 2, 2, 1, ""}))
}

func TestValidateCephVersionsBetweenLocalAndExternalClusters(t *testing.T) {
	// TEST 1: versions are identical
	localCephVersion := CephVersion{Major: 17, Minor: 2, Extra: 1}
	externalCephVersion := CephVersion{Major: 17, Minor: 2, Extra: 1}
	err := ValidateCephVersionsBetweenLocalAndExternalClusters(localCephVersion, externalCephVersion)
	assert.NoError(t, err)

	// TEST 2: local cluster version major is lower than external cluster version
	localCephVersion = CephVersion{Major: 17, Minor: 2, Extra: 1}
	externalCephVersion = CephVersion{Major: 18, Minor: 2, Extra: 1}
	err = ValidateCephVersionsBetweenLocalAndExternalClusters(localCephVersion, externalCephVersion)
	assert.NoError(t, err)

	// TEST 3: local cluster version major is higher than external cluster version
	localCephVersion = CephVersion{Major: 17, Minor: 2, Extra: 1}
	externalCephVersion = CephVersion{Major: 16, Minor: 2, Extra: 1}
	err = ValidateCephVersionsBetweenLocalAndExternalClusters(localCephVersion, externalCephVersion)
	assert.Error(t, err)

	// TEST 4: local version is > but from a minor release
	// local version must never be higher
	localCephVersion = CephVersion{Major: 17, Minor: 2, Extra: 2}
	externalCephVersion = CephVersion{Major: 17, Minor: 2, Extra: 1}
	err = ValidateCephVersionsBetweenLocalAndExternalClusters(localCephVersion, externalCephVersion)
	assert.Error(t, err)

	// TEST 5: external version is > but from a minor release
	localCephVersion = CephVersion{Major: 15, Minor: 2, Extra: 1}
	externalCephVersion = CephVersion{Major: 15, Minor: 2, Extra: 2}
	err = ValidateCephVersionsBetweenLocalAndExternalClusters(localCephVersion, externalCephVersion)
	assert.NoError(t, err)
}

func TestCephVersion_Unsupported(t *testing.T) {
	type fields struct {
		Major int
		Minor int
		Extra int
		Build int
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
<<<<<<< HEAD
		{"quincy", fields{Major: 17, Minor: 2, Extra: 0, Build: 0}, false},
=======
		{"squid", fields{Major: 19, Minor: 2, Extra: 0, Build: 0}, false},
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
		{"reef", fields{Major: 18, Minor: 2, Extra: 0, Build: 0}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &CephVersion{
				Major: tt.fields.Major,
				Minor: tt.fields.Minor,
				Extra: tt.fields.Extra,
				Build: tt.fields.Build,
			}
			if got := v.Unsupported(); got != tt.want {
				t.Errorf("CephVersion.Unsupported() = %v, want %v", got, tt.want)
			}
		})
	}
}
