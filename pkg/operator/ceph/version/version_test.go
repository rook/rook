package version

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToString(t *testing.T) {
	assert.Equal(t, "14.0.0 nautilus", fmt.Sprintf("%s", &Nautilus))
	assert.Equal(t, "13.0.0 mimic", fmt.Sprintf("%s", &Mimic))
	assert.Equal(t, "12.0.0 luminous", fmt.Sprintf("%s", &Luminous))

	expected := fmt.Sprintf("-1.0.0 %s", unknownVersionString)
	assert.Equal(t, expected, fmt.Sprintf("%s", &CephVersion{-1, 0, 0}))
}

func TestCephVersionFormatted(t *testing.T) {
	assert.Equal(t, "ceph version 14.0.0 nautilus", Nautilus.CephVersionFormatted())
	assert.Equal(t, "ceph version 13.0.0 mimic", Mimic.CephVersionFormatted())
	assert.Equal(t, "ceph version 12.0.0 luminous", Luminous.CephVersionFormatted())
}

func TestReleaseName(t *testing.T) {
	assert.Equal(t, "nautilus", Nautilus.ReleaseName())
	assert.Equal(t, "mimic", Mimic.ReleaseName())
	assert.Equal(t, "luminous", Luminous.ReleaseName())
	ver := CephVersion{-1, 0, 0}
	assert.Equal(t, unknownVersionString, ver.ReleaseName())
}

func extractVersionHelper(t *testing.T, text string, major, minor, extra int) {
	v, err := ExtractCephVersion(text)
	if assert.NoError(t, err) {
		assert.Equal(t, *v, CephVersion{major, minor, extra})
	}
}

func TestExtractVersion(t *testing.T) {
	// release build
	v0c := "ceph version 12.2.8 (ae699615bac534ea496ee965ac6192cb7e0e07c0) luminous (stable)"
	v0d := `
root@7a97f5a78bc6:/# ceph --version
ceph version 12.2.8 (ae699615bac534ea496ee965ac6192cb7e0e07c0) luminous (stable)
`
	extractVersionHelper(t, v0c, 12, 2, 8)
	extractVersionHelper(t, v0d, 12, 2, 8)

	// development build
	v1c := "ceph version 14.1.33-403-g7ba6bece41"
	v1d := `
bin/ceph --version
*** DEVELOPER MODE: setting PATH, PYTHONPATH and LD_LIBRARY_PATH ***
ceph version 14.1.33-403-g7ba6bece41
(7ba6bece4187eda5d05a9b84211fe6ba8dd287bd) nautilus (rc)
`
	extractVersionHelper(t, v1c, 14, 1, 33)
	extractVersionHelper(t, v1d, 14, 1, 33)

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
	assert.True(t, Luminous.isRelease(Luminous))
	assert.True(t, Mimic.isRelease(Mimic))
	assert.True(t, Nautilus.isRelease(Nautilus))

	assert.False(t, Luminous.isRelease(Mimic))
	assert.False(t, Luminous.isRelease(Nautilus))
	assert.False(t, Mimic.isRelease(Nautilus))

	LuminousUpdate := Luminous
	LuminousUpdate.Minor = 33
	LuminousUpdate.Extra = 4
	assert.True(t, LuminousUpdate.isRelease(Luminous))

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
	assert.True(t, Luminous.IsLuminous())
	assert.False(t, Mimic.IsLuminous())
	assert.False(t, Nautilus.IsLuminous())
	assert.False(t, Octopus.IsLuminous())

	assert.False(t, Luminous.IsMimic())
	assert.True(t, Mimic.IsMimic())
	assert.False(t, Nautilus.IsMimic())
	assert.False(t, Octopus.IsMimic())
}

func TestVersionAtLeast(t *testing.T) {
	assert.True(t, Luminous.IsAtLeast(Luminous))
	assert.False(t, Luminous.IsAtLeast(Mimic))
	assert.False(t, Luminous.IsAtLeast(Nautilus))
	assert.False(t, Luminous.IsAtLeast(Octopus))
	assert.True(t, Mimic.IsAtLeast(Luminous))
	assert.True(t, Mimic.IsAtLeast(Mimic))
	assert.False(t, Mimic.IsAtLeast(Nautilus))
	assert.False(t, Mimic.IsAtLeast(Octopus))
	assert.True(t, Nautilus.IsAtLeast(Luminous))
	assert.True(t, Nautilus.IsAtLeast(Mimic))
	assert.True(t, Nautilus.IsAtLeast(Nautilus))
	assert.False(t, Nautilus.IsAtLeast(Octopus))
	assert.True(t, Octopus.IsAtLeast(Luminous))
	assert.True(t, Octopus.IsAtLeast(Mimic))
	assert.True(t, Octopus.IsAtLeast(Nautilus))
	assert.True(t, Octopus.IsAtLeast(Octopus))

	assert.True(t, (&CephVersion{1, 0, 0}).IsAtLeast(CephVersion{0, 0, 0}))
	assert.False(t, (&CephVersion{0, 0, 0}).IsAtLeast(CephVersion{1, 0, 0}))
	assert.True(t, (&CephVersion{1, 1, 0}).IsAtLeast(CephVersion{1, 0, 0}))
	assert.False(t, (&CephVersion{1, 0, 0}).IsAtLeast(CephVersion{1, 1, 0}))
	assert.True(t, (&CephVersion{1, 1, 1}).IsAtLeast(CephVersion{1, 1, 0}))
	assert.False(t, (&CephVersion{1, 1, 0}).IsAtLeast(CephVersion{1, 1, 1}))
	assert.True(t, (&CephVersion{1, 1, 1}).IsAtLeast(CephVersion{1, 1, 1}))
}

func TestVersionAtLeastX(t *testing.T) {
	assert.True(t, Octopus.IsAtLeastOctopus())
	assert.True(t, Octopus.IsAtLeastNautilus())
	assert.True(t, Octopus.IsAtLeastMimic())
	assert.True(t, Nautilus.IsAtLeastNautilus())
	assert.True(t, Nautilus.IsAtLeastMimic())
	assert.True(t, Mimic.IsAtLeastMimic())
	assert.False(t, Luminous.IsAtLeastMimic())
	assert.False(t, Luminous.IsAtLeastNautilus())
	assert.False(t, Mimic.IsAtLeastNautilus())
	assert.False(t, Nautilus.IsAtLeastOctopus())
}
