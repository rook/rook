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

package v1

import (
	"github.com/pkg/errors"
)

func (p *PoolSpec) IsReplicated() bool {
	return p.Replicated.Size > 0
}

func (p *PoolSpec) IsErasureCoded() bool {
	return p.ErasureCoded.CodingChunks > 0 || p.ErasureCoded.DataChunks > 0
}

func (p *PoolSpec) IsHybridStoragePool() bool {
	return p.Replicated.HybridStorage != nil
}

func (p *PoolSpec) IsCompressionEnabled() bool {
	return p.CompressionMode != ""
}

func (p *ReplicatedSpec) IsTargetRatioEnabled() bool {
	return p.TargetSizeRatio != 0
}

// ValidateCephBlockPool validates specifically a CephBlockPool's spec (not just any NamedPoolSpec)
func ValidateCephBlockPool(p *CephBlockPool) error {
	if p.Spec.Name == ".rgw.root" || p.Spec.Name == ".mgr" || p.Spec.Name == ".nfs" {
		if p.Spec.IsErasureCoded() {
			return errors.Errorf("invalid CephBlockPool spec: ceph built-in pool %q cannot be erasure coded", p.Name)
		}
	}

	return validatePoolSpec(p.ToNamedPoolSpec())
}

// validate any NamedPoolSpec
func validatePoolSpec(ps NamedPoolSpec) error {
	// Checks if either ErasureCoded or Replicated fields are set
	if ps.ErasureCoded.CodingChunks <= 0 && ps.ErasureCoded.DataChunks <= 0 && ps.Replicated.TargetSizeRatio <= 0 && ps.Replicated.Size <= 0 {
		return errors.New("invalid pool spec: either of erasurecoded or replicated fields should be set")
	}
	// Check if any of the ErasureCoded fields are populated. Then check if replicated is populated. Both can't be populated at same time.
	if ps.ErasureCoded.CodingChunks > 0 || ps.ErasureCoded.DataChunks > 0 || ps.ErasureCoded.Algorithm != "" {
		if ps.Replicated.Size > 0 || ps.Replicated.TargetSizeRatio > 0 {
			return errors.New("invalid pool spec: both erasurecoded and replicated fields cannot be set at the same time")
		}
	}

	if ps.Replicated.Size == 0 && ps.Replicated.TargetSizeRatio == 0 {
		// Check if datachunks is set and has value less than 2.
		if ps.ErasureCoded.DataChunks < 2 && ps.ErasureCoded.DataChunks != 0 {
			return errors.New("invalid pool spec: erasurecoded.datachunks needs minimum value of 2")
		}

		// Check if codingchunks is set and has value less than 1.
		if ps.ErasureCoded.CodingChunks < 1 && ps.ErasureCoded.CodingChunks != 0 {
			return errors.New("invalid pool spec: erasurecoded.codingchunks needs minimum value of 1")
		}
	}
	return nil
}

func (p *CephBlockPool) ToNamedPoolSpec() NamedPoolSpec {
	// If the name is not overridden in the pool spec.name, set it to the name of the pool CR
	name := p.Spec.Name
	if name == "" {
		// Set the name of the pool CR since a name override wasn't specified in the spec
		name = p.Name
	}
	return NamedPoolSpec{
		Name:     name,
		PoolSpec: p.Spec.PoolSpec,
	}
}

func (p *CephBlockPool) GetStatusConditions() *[]Condition {
	return &p.Status.Conditions
}

func (p *CephBlockPoolRadosNamespace) GetStatusConditions() *[]Condition {
	return &p.Status.Conditions
}

// SnapshotSchedulesEnabled returns whether snapshot schedules are desired
func (p *MirroringSpec) SnapshotSchedulesEnabled() bool {
	return len(p.SnapshotSchedules) > 0
}
