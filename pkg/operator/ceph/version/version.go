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
	"regexp"
	"strconv"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
)

// CephVersion represents the Ceph version format
type CephVersion struct {
	Major int
	Minor int
	Extra int
	Build int
}

const (
	unknownVersionString = "<unknown version>"
)

var (
	// Minimum supported version is 14.2.5
	Minimum = CephVersion{14, 2, 5, 0}
	// Nautilus Ceph version
	Nautilus = CephVersion{14, 0, 0, 0}
	// Octopus Ceph version
	Octopus = CephVersion{15, 0, 0, 0}
	// Pacific Ceph version
	Pacific = CephVersion{16, 0, 0, 0}

	// supportedVersions are production-ready versions that rook supports
	supportedVersions   = []CephVersion{Nautilus, Octopus}
	unsupportedVersions = []CephVersion{Pacific}
	// allVersions includes all supportedVersions as well as unreleased versions that are being tested with rook
	allVersions = append(supportedVersions, unsupportedVersions...)

	// for parsing the output of `ceph --version`
	versionPattern = regexp.MustCompile(`ceph version (\d+)\.(\d+)\.(\d+)`)

	// For a build release the output is "ceph version 14.2.4-64.el8cp"
	// So we need to detect the build version change
	buildVersionPattern = regexp.MustCompile(`ceph version (\d+)\.(\d+)\.(\d+)\-(\d+)`)

	logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephver")
)

func (v *CephVersion) String() string {
	return fmt.Sprintf("%d.%d.%d-%d %s",
		v.Major, v.Minor, v.Extra, v.Build, v.ReleaseName())
}

// CephVersionFormatted returns the Ceph version in a human readable format
func (v *CephVersion) CephVersionFormatted() string {
	return fmt.Sprintf("ceph version %d.%d.%d-%d %s",
		v.Major, v.Minor, v.Extra, v.Build, v.ReleaseName())
}

// ReleaseName is the name of the Ceph release
func (v *CephVersion) ReleaseName() string {
	switch v.Major {
	case Octopus.Major:
		return "octopus"
	case Nautilus.Major:
		return "nautilus"
	default:
		return unknownVersionString
	}
}

// ExtractCephVersion extracts the major, minor and extra digit of a Ceph release
func ExtractCephVersion(src string) (*CephVersion, error) {
	var build int
	m := versionPattern.FindStringSubmatch(src)
	if m == nil {
		return nil, errors.Errorf("failed to parse version from: %q", src)
	}

	major, err := strconv.Atoi(m[1])
	if err != nil {
		return nil, errors.Errorf("failed to parse version major part: %q", m[1])
	}

	minor, err := strconv.Atoi(m[2])
	if err != nil {
		return nil, errors.Errorf("failed to parse version minor part: %q", m[2])
	}

	extra, err := strconv.Atoi(m[3])
	if err != nil {
		return nil, errors.Errorf("failed to parse version extra part: %q", m[3])
	}

	// See if we are running on a build release
	mm := buildVersionPattern.FindStringSubmatch(src)
	// We don't need to handle any error here, so let's jump in only when "mm" has content
	if mm != nil {
		build, err = strconv.Atoi(mm[4])
		if err != nil {
			logger.Warningf("failed to convert version build number part %q to an integer, ignoring", mm[4])
		}
	}

	return &CephVersion{major, minor, extra, build}, nil
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

// IsNautilus checks if the Ceph version is Nautilus
func (v *CephVersion) IsNautilus() bool {
	return v.isRelease(Nautilus)
}

// IsOctopus checks if the Ceph version is Octopus
func (v *CephVersion) IsOctopus() bool {
	return v.isRelease(Octopus)
}

// IsPacific checks if the Ceph version is Pacific
func (v *CephVersion) IsPacific() bool {
	return v.isRelease(Pacific)
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

// IsAtLeastPacific check that the Ceph version is at least Pacific
func (v *CephVersion) IsAtLeastPacific() bool {
	return v.IsAtLeast(Pacific)
}

// IsAtLeastOctopus check that the Ceph version is at least Octopus
func (v *CephVersion) IsAtLeastOctopus() bool {
	return v.IsAtLeast(Octopus)
}

// IsAtLeastNautilus check that the Ceph version is at least Nautilus
func (v *CephVersion) IsAtLeastNautilus() bool {
	return v.IsAtLeast(Nautilus)
}

// IsIdentical checks if Ceph versions are identical
func IsIdentical(a, b CephVersion) bool {
	if a.Major == b.Major {
		if a.Minor == b.Minor {
			if a.Extra == b.Extra {
				if a.Build == b.Build {
					return true
				}
			}
		}
	}

	return false
}

// IsSuperior checks if a given version if superior to another one
func IsSuperior(a, b CephVersion) bool {
	if a.Major > b.Major {
		return true
	}
	if a.Major == b.Major {
		if a.Minor > b.Minor {
			return true
		}
	}
	if a.Major == b.Major {
		if a.Minor == b.Minor {
			if a.Extra > b.Extra {
				return true
			}
		}
	}
	if a.Major == b.Major {
		if a.Minor == b.Minor {
			if a.Extra == b.Extra {
				if a.Build > b.Build {
					return true
				}
			}
		}
	}

	return false
}

// IsInferior checks if a given version if inferior to another one
func IsInferior(a, b CephVersion) bool {
	if a.Major < b.Major {
		return true
	}
	if a.Major == b.Major {
		if a.Minor < b.Minor {
			return true
		}
	}
	if a.Major == b.Major {
		if a.Minor == b.Minor {
			if a.Extra < b.Extra {
				return true
			}
		}
	}
	if a.Major == b.Major {
		if a.Minor == b.Minor {
			if a.Extra == b.Extra {
				if a.Build < b.Build {
					return true
				}
			}
		}
	}

	return false
}

// ValidateCephVersionsBetweenLocalAndExternalClusters makes sure an external cluster can be connected
// by checking the external ceph versions available and comparing it with the local image provided
func ValidateCephVersionsBetweenLocalAndExternalClusters(localVersion, externalVersion CephVersion) error {
	logger.Debugf("local version is %q, external version is %q", localVersion.String(), externalVersion.String())

	// We only support Nautilus or newer
	if !externalVersion.IsAtLeastNautilus() {
		return errors.Errorf("unsupported ceph version %q, need at least nautilus, delete your cluster CR and create a new one with a correct ceph version", externalVersion.String())
	}

	// Identical version, regardless if other CRs are running, it's ok!
	if IsIdentical(localVersion, externalVersion) {
		return nil
	}

	// Local version must never be higher than the external one
	if IsSuperior(localVersion, externalVersion) {
		return errors.Errorf("local cluster ceph version is higher %q than the external cluster %q, this must never happen", externalVersion.String(), localVersion.String())
	}

	// External cluster was updated to a minor version higher, consider updating too!
	if localVersion.Major == externalVersion.Major {
		if IsSuperior(externalVersion, localVersion) {
			logger.Warningf("external cluster ceph version is a minor version higher %q than the local cluster %q, consider upgrading", externalVersion.String(), localVersion.String())
			return nil
		}
	}

	// The external cluster was upgraded, consider upgrading too!
	if localVersion.Major < externalVersion.Major {
		logger.Errorf("external cluster ceph version is a major version higher %q than the local cluster %q, consider upgrading", externalVersion.String(), localVersion.String())
		return nil
	}

	return nil
}
