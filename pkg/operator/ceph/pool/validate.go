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
func ValidatePool(context *clusterd.Context, p *cephv1.CephBlockPool) error {
	if p.Name == "" {
		return errors.New("missing name")
	}
	if p.Namespace == "" {
		return errors.New("missing namespace")
	}
	if err := ValidatePoolSpec(context, p.Namespace, &p.Spec); err != nil {
		return err
	}
	return nil
}

// ValidatePoolSpec validates the Ceph block pool spec CR
func ValidatePoolSpec(context *clusterd.Context, namespace string, p *cephv1.PoolSpec) error {
	if p.IsReplicated() && p.IsErasureCoded() {
		return errors.New("both replication and erasure code settings cannot be specified")
	}

	var crush cephclient.CrushMap
	var err error
	if p.FailureDomain != "" || p.CrushRoot != "" {
		crush, err = cephclient.GetCrushMap(context, namespace)
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

	// validate pool replica size
	if p.Replicated.Size == 1 && p.Replicated.RequireSafeReplicaSize {
		return errors.Errorf("error pool size is %d and requireSafeReplicaSize is %t, must be false", p.Replicated.Size, p.Replicated.RequireSafeReplicaSize)
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

	return nil
}
