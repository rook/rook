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
	"fmt"
	"regexp"
	"strconv"

	"github.com/pkg/errors"
)

var (
	//minimum supported version is 3.5.0
	minimum = CephCSIVersion{3, 5, 0}
	//supportedCSIVersions are versions that rook supports
	releasev350 = CephCSIVersion{3, 5, 0}
	releasev360 = CephCSIVersion{3, 6, 0}

	supportedCSIVersions = []CephCSIVersion{
		minimum,
		releasev350,
		releasev360,
	}

	// custom ceph.conf is supported in v3.5.0+
	cephConfSupportedVersion = CephCSIVersion{3, 5, 0}

	// pod networking fix with the extra holder pod is supported with at least v3.6.1
	nsenterSupportedVersion = CephCSIVersion{3, 6, 1}

	// for parsing the output of `cephcsi`
	versionCSIPattern = regexp.MustCompile(`v(\d+)\.(\d+)\.(\d+)`)
)

// CephCSIVersion represents the Ceph CSI version format
type CephCSIVersion struct {
	Major  int
	Minor  int
	Bugfix int
}

func (v *CephCSIVersion) String() string {
	return fmt.Sprintf("v%d.%d.%d",
		v.Major, v.Minor, v.Bugfix)
}

// Supported checks if the detected version is part of the known supported CSI versions
func (v *CephCSIVersion) Supported() bool {
	if !v.isAtLeast(&minimum) {
		return false
	}

	// if AllowUnsupported is set also a csi-image greater than the supported ones are allowed
	if AllowUnsupported {
		return true
	}
	for _, sv := range supportedCSIVersions {
		if v.Major == sv.Major {
			if v.Minor == sv.Minor {
				if v.Bugfix >= sv.Bugfix {
					return true
				}
			}
		}
	}
	return false
}

func (v *CephCSIVersion) isAtLeast(version *CephCSIVersion) bool {
	if v.Major > version.Major {
		return true
	}
	if v.Major == version.Major && v.Minor >= version.Minor {
		if v.Minor > version.Minor {
			return true
		}
		if v.Bugfix >= version.Bugfix {
			return true
		}
	}
	return false
}

// extractCephCSIVersion extracts the major, minor and extra digit of a Ceph CSI release
func extractCephCSIVersion(src string) (*CephCSIVersion, error) {
	m := versionCSIPattern.FindStringSubmatch(src)
	if m == nil || len(m) < 3 {
		return nil, errors.Errorf("failed to parse version from: %q", CSIParam.CSIPluginImage)
	}

	major, err := strconv.Atoi(m[1])
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse version major part: %q", m[0])
	}

	minor, err := strconv.Atoi(m[2])
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse version minor part: %q", m[1])
	}

	bugfix, err := strconv.Atoi(m[3])
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse version bugfix part: %q", m[2])
	}

	return &CephCSIVersion{major, minor, bugfix}, nil
}

// SupportsCustomCephConf checks if the detected version supports custom ceph.conf
func (v *CephCSIVersion) SupportsCustomCephConf() bool {
	// if AllowUnsupported is set also a csi-image greater than the supported ones are allowed
	if AllowUnsupported {
		return true
	}

	return v.isAtLeast(&cephConfSupportedVersion)
}

// SupportsNsenter checks if the csi image has support for calling "nsenter" while executing
// mount/map commands. This is needed for Multus scenarios.
func (v *CephCSIVersion) SupportsNsenter() bool {
	// if AllowUnsupported is set also a csi-image greater than the supported ones are allowed
	if AllowUnsupported {
		return true
	}

	return v.isAtLeast(&nsenterSupportedVersion)
}
