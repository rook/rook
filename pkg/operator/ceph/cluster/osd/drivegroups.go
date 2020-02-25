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

package osd

import (
	"encoding/json"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
)

// MarshalAsDriveGroupBlobs converts a Ceph CRD <Drive Group Name> => <Drive Group Spec> mapping
// into a JSON-marshalled <Drive Group Name> => <Drive Group JSON blob> mapping.
func MarshalAsDriveGroupBlobs(dgs cephv1.DriveGroupsSpec) (string, error) {
	var blobs config.DriveGroupBlobs = map[string]string{}

	for _, dg := range dgs {
		b, err := json.Marshal(dg.Spec)
		if err != nil {
			return "", errors.Wrapf(err, "failed to marshal Drive Group %q into a JSON blob", dg.Name)
		}
		blobs[dg.Name] = string(b)
	}

	b, err := json.Marshal(blobs)
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal Drive Group JSON blobs after marshalling each spec into a blob: %+v", blobs)
	}

	return string(b), nil
}

// SanitizeDriveGroups processes the drive groups to remove or correct invalid specs.
func SanitizeDriveGroups(dgs cephv1.DriveGroupsSpec) cephv1.DriveGroupsSpec {
	out := cephv1.DriveGroupsSpec{}
	for _, dg := range dgs {
		group := *dg.DeepCopy()
		if len(group.Spec) == 0 {
			logger.Warningf("drive group %q spec is empty. skipping this drive group: %+v", group.Name, group.Spec)
			continue
		}

		// We want to do placement only based on Rook's definition of placement, so remove any
		// placement the Drive Group specifies by removing any existing placements from the spec.
		if p, ok := group.Spec["placement"]; ok {
			logger.Warningf("Rook will ignore 'placement' spec (%+v) within Drive Group %q by removing it from the Drive Group spec; Rook will control placement instead", p, group.Name)
			delete(group.Spec, "placement")
		}
		if h, ok := group.Spec["host_pattern"]; ok {
			logger.Warningf("Rook will ignore deprecated 'host_pattern' spec (\"%+v\") within Drive Group %q by removing it from the Drive Group spec; Rook will control placement instead", h, group.Name)
			delete(group.Spec, "host_pattern")
		}

		// Ceph requires a placement to be set, so we set a host pattern to match any host
		// use 'interface{}' because DeepCopyJSON only supports maps of map[string]interface{}
		group.Spec["placement"] = map[string]interface{}{
			"host_pattern": "*",
		}
		// Ceph requires the service ID to be set, which is the name of the drive group; in Rook, we
		// force this value to be the same as the name given in the CephCluster CRD
		if s, ok := group.Spec["service_id"]; ok {
			logger.Warningf("Rook will ignore the 'service_id' spec (\"%+v\") within Drive Group %q by overwriting it with the Rook-defined name (%q) instead", s, group.Name, group.Name)
		}
		group.Spec["service_id"] = group.Name

		out = append(out, group)
	}
	return out
}

// DriveGroupPlacementMatchesNode returns true if the Drive Group's placement matches the given node.
// It returns false if the placement does not match the given node.
// It returns an error if placement match cannot be determined.
func DriveGroupPlacementMatchesNode(dg cephv1.DriveGroup, n *v1.Node) (bool, error) {
	valid, err := k8sutil.ValidNode(*n, dg.Placement)
	if err != nil {
		return false, errors.Wrapf(err, "failed to determine if node %q is valid for drive group %q", n.Name, dg.Name)
	}
	return valid, nil
}

// DriveGroupsWithPlacementMatchingNode returns a subset of the Drive Groups Spec with placement
// that matches the given node.
// It returns an error if placement cannot be determined for the node for any Drive Group in the spec.
func DriveGroupsWithPlacementMatchingNode(dgs cephv1.DriveGroupsSpec, n *v1.Node) (cephv1.DriveGroupsSpec, error) {
	groups := cephv1.DriveGroupsSpec{}
	for _, dg := range dgs {
		match, err := DriveGroupPlacementMatchesNode(dg, n)
		if err != nil {
			return cephv1.DriveGroupsSpec{}, errors.Wrapf(err, "failed to find all drive groups matching node %q", n.Name)
		}
		if match {
			groups = append(groups, dg)
		}
	}
	return groups, nil
}
