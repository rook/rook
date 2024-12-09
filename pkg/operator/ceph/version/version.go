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
	Major    int
	Minor    int
	Extra    int
	Build    int
	CommitID string
}

const (
	unknownVersionString = "<unknown version>"
)

var (
	// Minimum supported version
<<<<<<< HEAD
	Minimum = CephVersion{17, 2, 0, 0, ""}

	// Quincy Ceph version
	Quincy = CephVersion{17, 0, 0, 0, ""}
=======
	Minimum = CephVersion{18, 2, 0, 0, ""}

>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
	// Reef Ceph version
	Reef = CephVersion{18, 0, 0, 0, ""}
	// Squid ceph version
	Squid = CephVersion{19, 0, 0, 0, ""}
	// Tentacle ceph version
	Tentacle = CephVersion{20, 0, 0, 0, ""}

	// supportedVersions are production-ready versions that rook supports
<<<<<<< HEAD
	supportedVersions = []CephVersion{Quincy, Reef, Squid}
=======
	supportedVersions = []CephVersion{Reef, Squid}
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

	// unsupportedVersions are possibly Ceph pin-point release that introduced breaking changes and not recommended
	unsupportedVersions []CephVersion

	// for parsing the output of `ceph --version`
	versionPattern = regexp.MustCompile(`ceph version (\d+)\.(\d+)\.(\d+)`)

	// For a build release the output is "ceph version 14.2.4-64.el8cp"
	// So we need to detect the build version change
	buildVersionPattern = regexp.MustCompile(`ceph version (\d+)\.(\d+)\.(\d+)\-(\d+)`)

	// for parsing the commit hash in the ceph --version output. For example:
	// input = `ceph version 14.2.11-139 (5c0dc966af809fd1d429ec7bac48962a746af243) nautilus (stable)`
	// output = [(5c0dc966af809fd1d429ec7bac48962a746af243) 5c0dc966af809fd1d429ec7bac48962a746af243]
	commitIDPattern = regexp.MustCompile(`\(([^)]+)\)`)

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
<<<<<<< HEAD
	case Quincy.Major:
		return "quincy"
=======
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
	case Reef.Major:
		return "reef"
	case Squid.Major:
		return "squid"
	case Tentacle.Major:
		return "tentacle"
	default:
		return unknownVersionString
	}
}

// ExtractCephVersion extracts the major, minor and extra digit of a Ceph release
func ExtractCephVersion(src string) (*CephVersion, error) {
	var build int
	var commitID string
	versionMatch := versionPattern.FindStringSubmatch(src)
	if versionMatch == nil {
		return nil, errors.Errorf("failed to parse version from: %q", src)
	}

	major, err := strconv.Atoi(versionMatch[1])
	if err != nil {
		return nil, errors.Errorf("failed to parse version major part: %q", versionMatch[1])
	}

	minor, err := strconv.Atoi(versionMatch[2])
	if err != nil {
		return nil, errors.Errorf("failed to parse version minor part: %q", versionMatch[2])
	}

	extra, err := strconv.Atoi(versionMatch[3])
	if err != nil {
		return nil, errors.Errorf("failed to parse version extra part: %q", versionMatch[3])
	}

	// See if we are running on a build release
	buildVersionMatch := buildVersionPattern.FindStringSubmatch(src)
	// We don't need to handle any error here, so let's jump in only when "mm" has content
	if buildVersionMatch != nil {
		build, err = strconv.Atoi(buildVersionMatch[4])
		if err != nil {
			logger.Warningf("failed to convert version build number part %q to an integer, ignoring", buildVersionMatch[4])
		}
	}

	commitIDMatch := commitIDPattern.FindStringSubmatch(src)
	if commitIDMatch != nil {
		commitID = commitIDMatch[1]
	}

	return &CephVersion{major, minor, extra, build, commitID}, nil
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

// Unsupported checks if a given release is supported
func (v *CephVersion) Unsupported() bool {
	for _, sv := range unsupportedVersions {
		if v.isExactly(sv) {
			return true
		}
	}
	return false
}

func (v *CephVersion) isRelease(other CephVersion) bool {
	return v.Major == other.Major
}

func (v *CephVersion) isExactly(other CephVersion) bool {
	return v.Major == other.Major && v.Minor == other.Minor && v.Extra == other.Extra
}

<<<<<<< HEAD
// IsQuincy checks if the Ceph version is Quincy
func (v *CephVersion) IsQuincy() bool {
	return v.isRelease(Quincy)
}

=======
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
// IsReef checks if the Ceph version is Reef
func (v *CephVersion) IsReef() bool {
	return v.isRelease(Reef)
}

// IsSquid checks if the Ceph version is Squid
func (v *CephVersion) IsSquid() bool {
	return v.isRelease(Squid)
}

// IsTentacle checks if the Ceph version is Tentacle
func (v *CephVersion) IsTentacle() bool {
	return v.isRelease(Tentacle)
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

// IsAtLeastTentacle checks that the Ceph version is at least Tentacle
func (v *CephVersion) IsAtLeastTentacle() bool {
	return v.IsAtLeast(Tentacle)
}

// IsAtLeastSquid checks that the Ceph version is at least Squid
func (v *CephVersion) IsAtLeastSquid() bool {
	return v.IsAtLeast(Squid)
}

// IsAtLeastReef checks that the Ceph version is at least Reef
func (v *CephVersion) IsAtLeastReef() bool {
	return v.IsAtLeast(Reef)
}

<<<<<<< HEAD
// IsAtLeastQuincy checks that the Ceph version is at least Quincy
func (v *CephVersion) IsAtLeastQuincy() bool {
	return v.IsAtLeast(Quincy)
}

=======
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
// IsIdentical checks if Ceph versions are identical
func IsIdentical(a, b CephVersion) bool {
	if a.Major == b.Major {
		if a.Minor == b.Minor {
			if a.Extra == b.Extra {
				if a.Build == b.Build {
					if a.CommitID == b.CommitID {
						return true
					}
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
				if a.CommitID != b.CommitID {
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

	// We only support Octopus or newer
	if !externalVersion.IsAtLeast(CephVersion{Major: 15}) {
		return errors.Errorf("unsupported external ceph version %q, need at least octopus, delete your cluster CR and create a new one with a correct ceph version", externalVersion.String())
	}

	// Identical version, regardless if other CRs are running, it's ok!
	if IsIdentical(localVersion, externalVersion) {
		return nil
	}

	// Local version must never be higher than the external one
	if IsSuperior(localVersion, externalVersion) {
		return errors.Errorf("local cluster ceph version %q is higher than the external cluster version %q, this must never happen", localVersion.String(), externalVersion.String())
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
