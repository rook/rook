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

// Package pool to manage a rook pool.
package pool

import (
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
)

// ValidatePool Validate the pool arguments
func ValidatePool(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, clusterSpec *cephv1.ClusterSpec, p *cephv1.CephBlockPool) error {
	if p.Name == "" {
		return errors.New("missing name")
	}
	if p.Namespace == "" {
		return errors.New("missing namespace")
	}
	if err := ValidatePoolSpec(context, clusterInfo, clusterSpec, &p.Spec); err != nil {
		return err
	}
	return nil
}

// ValidatePoolSpec validates the Ceph block pool spec CR
func ValidatePoolSpec(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, clusterSpec *cephv1.ClusterSpec, p *cephv1.PoolSpec) error {
	if p.IsReplicated() && p.IsErasureCoded() {
		return errors.New("both replication and erasure code settings cannot be specified")
	}

	if p.FailureDomain != "" && p.Replicated.SubFailureDomain != "" {
		if p.FailureDomain == p.Replicated.SubFailureDomain {
			return errors.New("failure and subfailure domain cannot be identical")
		}
	}

	// validate pools for stretch clusters
	if clusterSpec.IsStretchCluster() {
		if p.IsReplicated() {
			if p.Replicated.Size != 4 {
				return errors.New("pools in a stretch cluster must have replication size 4")
			}
		}
		if p.IsErasureCoded() {
			return errors.New("erasure coded pools are not supported in stretch clusters")
		}
	}

	var crush cephclient.CrushMap
	var err error
	if p.FailureDomain != "" || p.CrushRoot != "" {
		crush, err = cephclient.GetCrushMap(context, clusterInfo)
		if err != nil {
			return errors.Wrap(err, "failed to get crush map")
		}
	}

	// validate the failure domain if specified
	if p.FailureDomain != "" {
		found := false
		for _, t := range crush.Types {
			if t.Name == p.FailureDomain {
				found = true
				break
			}
		}
		if !found {
			return errors.Errorf("unrecognized failure domain %s", p.FailureDomain)
		}
	}

	// validate the crush root if specified
	if p.CrushRoot != "" {
		found := false
		for _, t := range crush.Buckets {
			if t.Name == p.CrushRoot {
				found = true
				break
			}
		}
		if !found {
			return errors.Errorf("unrecognized crush root %s", p.CrushRoot)
		}
	}

	// validate the crush subdomain if specified
	if p.Replicated.SubFailureDomain != "" {
		found := false
		for _, t := range crush.Types {
			if t.Name == p.Replicated.SubFailureDomain {
				found = true
				break
			}
		}
		if !found {
			return errors.Errorf("unrecognized crush sub domain %s", p.Replicated.SubFailureDomain)
		}
	}

	// validate pool replica size
	if p.IsReplicated() {
		if p.Replicated.Size == 1 && p.Replicated.RequireSafeReplicaSize {
			return errors.Errorf("error pool size is %d and requireSafeReplicaSize is %t, must be false", p.Replicated.Size, p.Replicated.RequireSafeReplicaSize)
		}

		if p.Replicated.Size <= p.Replicated.ReplicasPerFailureDomain {
			return errors.Errorf("error pool size is %d and replicasPerFailureDomain is %d, size must be greater", p.Replicated.Size, p.Replicated.ReplicasPerFailureDomain)
		}

		if p.Replicated.ReplicasPerFailureDomain != 0 && p.Replicated.Size%p.Replicated.ReplicasPerFailureDomain != 0 {
			return errors.Errorf("error replicasPerFailureDomain is %d must be a factor of the replica count %d", p.Replicated.ReplicasPerFailureDomain, p.Replicated.Size)
		}
	}

	// validate pool compression mode if specified
	if p.CompressionMode != "" {
		switch p.CompressionMode {
		case "none", "passive", "aggressive", "force":
			break
		default:
			return errors.Errorf("unrecognized compression mode %q", p.CompressionMode)
		}
	}

	// Validate mirroring settings
	if p.Mirroring.Enabled {
		switch p.Mirroring.Mode {
		case "image", "pool":
			break
		default:
			return errors.Errorf("unrecognized mirroring mode %q. only 'image and 'pool' are supported", p.Mirroring.Mode)
		}

		if p.Mirroring.SnapshotSchedulesEnabled() {
			for _, snapSchedule := range p.Mirroring.SnapshotSchedules {
				if snapSchedule.Interval == "" && snapSchedule.StartTime != "" {
					return errors.New("schedule interval cannot be empty if start time is specified")
				}
			}
		}
	}

	if !p.Mirroring.Enabled && p.Mirroring.SnapshotSchedulesEnabled() {
		return errors.New("mirroring must be enabled to configure snapshot scheduling")
	}

	return nil
}
