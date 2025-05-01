/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package keyring

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	v1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/version"
)

var CephAuthRotateSupportedVersion = version.CephVersion{Major: 20, Minor: 2, Extra: 0}

// CephxKeyIdentifierAnnotation is the annotation that should be applied to pod specs to
// ensure that pods restart after keys are rotated (and not restarted when keys are not rotated).
// The keyring secret resourceVersion is suggested but not always available.
//
//nolint:gosec // G101: this is not hardcoded credentials
const CephxKeyIdentifierAnnotation = "cephx-key-identifier"

// ShouldRotateCephxKeys determines whether CephX keys should be rotated based on the CephX key
// rotation config, the version of Ceph present in the image being deployed (desiredCephVersion),
// and the last-reconciled CephX key status.
// runningCephVersion is used to determine if the cluster is capable of rotating CephX keys.
// Intended to use running/desired ceph version from CurrentAndDesiredCephVersion().
func ShouldRotateCephxKeys(cfg v1.CephxConfig, runningCephVersion, desiredCephVersion version.CephVersion, status v1.CephxStatus) (bool, error) {
	if !runningCephVersion.IsAtLeast(CephAuthRotateSupportedVersion) {
		logger.Debugf("should not rotate cephx keys using unsupported ceph version %#v", runningCephVersion)
		return false, nil
	}

	if status.KeyCephVersion == v1.UninitializedCephxKeyCephVersion {
		return false, nil // no need to rotate key when key isn't yet initialized
	}

	switch cfg.KeyRotationPolicy {
	case v1.CephxKeyRotationPolicy(""), v1.DisabledCephxKeyRotationPolicy:
		return false, nil
	case v1.KeyGenerationCephxKeyRotationPolicy:
		return cfg.KeyGeneration > status.KeyGeneration, nil
	case "WithCephVersionUpdate": // TODO: use types.go value when allowed by user input
		// basic functionality for this policy is implemented here, but this is disabled as a user
		// selectable option. code and tests are retained for when we can validate this more deeply

		if version.IsIdentical(desiredCephVersion, version.CephVersion{}) {
			// likely cause of this is developer error. if that makes it to release, it's probably
			// best to not cause unrecoverable error for users
			logger.Info("ShouldRotateCephxKeys(): desiredCephVersion is unspecified")
			return false, nil
		}

		if status.KeyCephVersion == "" {
			return true, nil // when previous version is unknown, assume rotation
		}

		statusVer, err := parseCephVersionFromStatusVersion(status.KeyCephVersion)
		if err != nil {
			return false, errors.Wrapf(err, "failed to determine if cephx keys need to be rotated under %q rotation policy; failed to parse key ceph version from status %q", cfg.KeyRotationPolicy, status.KeyCephVersion)
		}
		// by API spec, commit ID is not part of CephCluster.status.version.version or
		// keyCephVersion, so strip it from version found in ceph image
		desiredCephVersion.CommitID = ""
		if version.IsSuperior(desiredCephVersion, statusVer) {
			return true, nil
		}
		return false, nil
	default:
		return false, errors.Errorf("unknown cephx key rotation policy %q", cfg.KeyRotationPolicy)
	}
}

// CephVersionToCephxStatusVersion renders a CephVersion struct into status.KeyCephVersion format.
// This is expected to be the same format used by CephCluster.status.version.version.
func CephVersionToCephxStatusVersion(v version.CephVersion) string {
	return fmt.Sprintf("%d.%d.%d-%d", // DO NOT CHANGE FORMAT FROM "Maj.Min.Ext-Bld"
		v.Major, v.Minor, v.Extra, v.Build)
}

func parseCephVersionFromStatusVersion(inVer string) (version.CephVersion, error) {
	cephVer := version.CephVersion{}
	var err error

	semvBuild := strings.Split(inVer, "-")
	if len(semvBuild) != 2 {
		return version.CephVersion{}, errors.Errorf("failed to parse %q (expected format \"X.Y.Z-B\") into a ceph version at build split", inVer)
	}
	semv := semvBuild[0]
	build := semvBuild[1]

	cephVer.Build, err = strconv.Atoi(build)
	if err != nil {
		return version.CephVersion{}, errors.Wrapf(err, "failed to parse build version from %q", inVer)
	}

	mme := strings.Split(semv, ".")
	if len(mme) != 3 {
		return version.CephVersion{}, errors.Errorf("failed to parse %q (expected format \"X.Y.Z-B\") into a ceph version at Major-Minor-Extra split", inVer)
	}

	cephVer.Major, err = strconv.Atoi(mme[0])
	if err != nil {
		return version.CephVersion{}, errors.Wrapf(err, "failed to parse major version from %q", inVer)
	}

	cephVer.Minor, err = strconv.Atoi(mme[1])
	if err != nil {
		return version.CephVersion{}, errors.Wrapf(err, "failed to parse minor version from %q", inVer)
	}

	cephVer.Extra, err = strconv.Atoi(mme[2])
	if err != nil {
		return version.CephVersion{}, errors.Wrapf(err, "failed to parse extra version from %q", inVer)
	}

	return cephVer, nil
}

// UninitializedCephxStatus provides the initial status that indicates CephX keys haven't been
// initialized. This should be applied when a resource status is first set to a non-nil status.
// Together with UpdatedCephxStatus() below, this helps ensure that Rook only applies key generation
// and ceph version info to the status when a resource is first being provisioned.
// Resources that were provisioned before CephX key rotation and version tracking were implemented
// will be identified by KeyCephVersion="", the empty string.
func UninitializedCephxStatus() v1.CephxStatus {
	return v1.CephxStatus{
		KeyGeneration:  0,
		KeyCephVersion: v1.UninitializedCephxKeyCephVersion,
	}
}

// UpdatedCephxStatus returns the updated CephxStatus based on rotation config and status from
// before rotation occurred.
func UpdatedCephxStatus(didRotate bool, cfg v1.CephxConfig, runningCephVersion version.CephVersion, status v1.CephxStatus) v1.CephxStatus {
	newStatus := status.DeepCopy()

	// uninitialized key ceph version indicates that the key was newly been created
	// this is true regardless of whether the key was rotated or not
	if status.KeyCephVersion == v1.UninitializedCephxKeyCephVersion {
		newStatus.KeyCephVersion = CephVersionToCephxStatusVersion(runningCephVersion)
		newStatus.KeyGeneration = 1

		// corner case: user sets KeyGeneration policy with KeyGeneration > 1 to the spec of a CR
		// that is being newly provisioned. in this case, rotation isn't relevant, but Rook should
		// make sure status doesn't report that rotation is in progress when it isn't
		if cfg.KeyRotationPolicy == v1.KeyGenerationCephxKeyRotationPolicy && cfg.KeyGeneration > 1 {
			newStatus.KeyGeneration = cfg.KeyGeneration
		}

		return *newStatus
	}

	if !didRotate {
		// preserve previous status when keys not rotated. importantly, retains KeyCephVersion
		// status as an empty string in brownfield resources where the version is unknown
		return status
	}

	newStatus.KeyCephVersion = CephVersionToCephxStatusVersion(runningCephVersion)
	newStatus.KeyGeneration++

	if cfg.KeyRotationPolicy == v1.KeyGenerationCephxKeyRotationPolicy {
		if cfg.KeyGeneration > newStatus.KeyGeneration {
			// do not allow the status key gen to be reduced, no matter what input config says
			newStatus.KeyGeneration = cfg.KeyGeneration
		}
	}

	return *newStatus
}
