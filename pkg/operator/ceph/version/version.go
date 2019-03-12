package version

import (
	"fmt"
	"regexp"
	"strconv"
)

type CephVersion struct {
	Major int
	Minor int
	Extra int
}

const (
	unknownVersionString = "<unknown version>"
)

var (
	Luminous = CephVersion{12, 0, 0}
	Mimic    = CephVersion{13, 0, 0}
	Nautilus = CephVersion{14, 0, 0}

	// supportedVersions are production-ready versions that rook supports
	supportedVersions   = []CephVersion{Luminous, Mimic}
	unsupportedVersions = []CephVersion{Nautilus}
	// allVersions includes all supportedVersions as well as unreleased versions that are being tested with rook
	allVersions = append(supportedVersions, unsupportedVersions...)

	// for parsing the output of `ceph --version`
	versionPattern = regexp.MustCompile(`ceph version (\d+)\.(\d+)\.(\d+)`)
)

func (v *CephVersion) String() string {
	return fmt.Sprintf("%d.%d.%d %s",
		v.Major, v.Minor, v.Extra, v.ReleaseName())
}

func (v *CephVersion) ReleaseName() string {
	switch v.Major {
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

func (v *CephVersion) IsLuminous() bool {
	return v.isRelease(Luminous)
}

func (v *CephVersion) AtLeast(other CephVersion) bool {
	if v.Major > other.Major {
		return true
	} else if v.Major < other.Major {
		return false
	}
	if v.Minor > other.Minor {
		return true
	} else if v.Minor < other.Minor {
		return false
	}
	if v.Extra > other.Extra {
		return true
	} else if v.Extra < other.Extra {
		return false
	}
	return true
}

func (v *CephVersion) AtLeastNautilus() bool {
	return v.AtLeast(Nautilus)
}

func (v *CephVersion) AtLeastMimic() bool {
	return v.AtLeast(Mimic)
}
