package version

import (
	"fmt"
	"regexp"
	"strconv"
)

// CephVersion represents the Ceph version format
type CephVersion struct {
	Major int
	Minor int
	Extra int
}

const (
	unknownVersionString = "<unknown version>"
)

var (
	// Luminous Ceph version
	Luminous = CephVersion{12, 0, 0}
	// Mimic Ceph version
	Mimic = CephVersion{13, 0, 0}
	// Nautilus Ceph version
	Nautilus = CephVersion{14, 0, 0}
	// Octopus Ceph version
	Octopus = CephVersion{15, 0, 0}

	// supportedVersions are production-ready versions that rook supports
	supportedVersions   = []CephVersion{Luminous, Mimic, Nautilus}
	unsupportedVersions = []CephVersion{Octopus}
	// allVersions includes all supportedVersions as well as unreleased versions that are being tested with rook
	allVersions = append(supportedVersions, unsupportedVersions...)

	// for parsing the output of `ceph --version`
	versionPattern = regexp.MustCompile(`ceph version (\d+)\.(\d+)\.(\d+)`)
)

func (v *CephVersion) String() string {
	return fmt.Sprintf("%d.%d.%d %s",
		v.Major, v.Minor, v.Extra, v.ReleaseName())
}

// CephVersionFormatted returns the Ceph version in a human readable format
func (v *CephVersion) CephVersionFormatted() string {
	return fmt.Sprintf("ceph version %d.%d.%d %s",
		v.Major, v.Minor, v.Extra, v.ReleaseName())
}

// ReleaseName is the name of the Ceph release
func (v *CephVersion) ReleaseName() string {
	switch v.Major {
	case Octopus.Major:
		return "octopus"
	case Nautilus.Major:
		return "nautilus"
	case Mimic.Major:
		return "mimic"
	case Luminous.Major:
		return "luminous"
	default:
		return unknownVersionString
	}
}

// ExtractCephVersion extracts the major, minor and extra digit of a Ceph release
func ExtractCephVersion(src string) (*CephVersion, error) {
	m := versionPattern.FindStringSubmatch(src)
	if m == nil {
		return nil, fmt.Errorf("failed to parse version from: %s", src)
	}

	major, err := strconv.Atoi(m[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse version major part: %s", m[0])
	}

	minor, err := strconv.Atoi(m[2])
	if err != nil {
		return nil, fmt.Errorf("failed to parse version minor part: %s", m[1])
	}

	extra, err := strconv.Atoi(m[3])
	if err != nil {
		return nil, fmt.Errorf("failed to parse version extra part: %s", m[2])
	}

	return &CephVersion{major, minor, extra}, nil
}

// Supported checks if a given release is supported
func (v *CephVersion) Supported() bool {
	for _, sv := range supportedVersions {
		if v.isRelease(sv) {
			return true
		}
	}
	return false
}

func (v *CephVersion) isRelease(other CephVersion) bool {
	return v.Major == other.Major
}

// IsLuminous checks if the Ceph version is Luminous
func (v *CephVersion) IsLuminous() bool {
	return v.isRelease(Luminous)
}

// IsMimic checks if the Ceph version is Mimic
func (v *CephVersion) IsMimic() bool {
	return v.isRelease(Mimic)
}

// IsAtLeast checks a given Ceph version is at least a given one
func (v *CephVersion) IsAtLeast(other CephVersion) bool {
	if v.Major > other.Major {
		return true
	} else if v.Major < other.Major {
		return false
	}
	// If we arrive here then v.Major == other.Major
	if v.Minor > other.Minor {
		return true
	} else if v.Minor < other.Minor {
		return false
	}
	// If we arrive here then v.Minor == other.Minor
	if v.Extra > other.Extra {
		return true
	} else if v.Extra < other.Extra {
		return false
	}
	// If we arrive here then both versions are identical
	return true
}

// IsAtLeastOctopus check that the Ceph version is at least Octopus
func (v *CephVersion) IsAtLeastOctopus() bool {
	return v.IsAtLeast(Octopus)
}

// IsAtLeastNautilus check that the Ceph version is at least Nautilus
func (v *CephVersion) IsAtLeastNautilus() bool {
	return v.IsAtLeast(Nautilus)
}

// IsAtLeastMimic check that the Ceph version is at least Mimic
func (v *CephVersion) IsAtLeastMimic() bool {
	return v.IsAtLeast(Mimic)
}
