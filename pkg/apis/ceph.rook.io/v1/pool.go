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
	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	webhookName = "rook-ceph-webhook"
	logger      = capnslog.NewPackageLogger("github.com/rook/rook", webhookName)
)

var _ webhook.Validator = &CephBlockPool{}

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

func (p *CephBlockPool) ValidateCreate() error {
	logger.Infof("validate create cephblockpool %v", p)

	err := validatePoolSpec(p.Spec)
	if err != nil {
		return err
	}
	return nil
}

func validatePoolSpec(ps PoolSpec) error {
	// Checks if either ErasureCoded or Replicated fields are set
	if ps.ErasureCoded.CodingChunks <= 0 && ps.ErasureCoded.DataChunks <= 0 && ps.Replicated.TargetSizeRatio <= 0 && ps.Replicated.Size <= 0 {
		return errors.New("invalid create: either of erasurecoded or replicated fields should be set")
	}
	// Check if any of the ErasureCoded fields are populated. Then check if replicated is populated. Both can't be populated at same time.
	if ps.ErasureCoded.CodingChunks > 0 || ps.ErasureCoded.DataChunks > 0 || ps.ErasureCoded.Algorithm != "" {
		if ps.Replicated.Size > 0 || ps.Replicated.TargetSizeRatio > 0 {
			return errors.New("invalid create: both erasurecoded and replicated fields cannot be set at the same time")
		}
	}

	if ps.Replicated.Size == 0 && ps.Replicated.TargetSizeRatio == 0 {
		// Check if datachunks is set and has value less than 2.
		if ps.ErasureCoded.DataChunks < 2 && ps.ErasureCoded.DataChunks != 0 {
			return errors.New("invalid create: erasurecoded.datachunks needs minimum value of 2")
		}

		// Check if codingchunks is set and has value less than 1.
		if ps.ErasureCoded.CodingChunks < 1 && ps.ErasureCoded.CodingChunks != 0 {
			return errors.New("invalid create: erasurecoded.codingchunks needs minimum value of 1")
		}
	}
	return nil
}

func (p *CephBlockPool) ValidateUpdate(old runtime.Object) error {
	logger.Info("validate update cephblockpool")
	ocbp := old.(*CephBlockPool)
	err := validatePoolSpec(p.Spec)
	if err != nil {
		return err
	}
	if p.Spec.ErasureCoded.CodingChunks > 0 || p.Spec.ErasureCoded.DataChunks > 0 || p.Spec.ErasureCoded.Algorithm != "" {
		if ocbp.Spec.Replicated.Size > 0 || ocbp.Spec.Replicated.TargetSizeRatio > 0 {
			return errors.New("invalid update: replicated field is set already in previous object. cannot be changed to use erasurecoded")
		}
	}

	if p.Spec.Replicated.Size > 0 || p.Spec.Replicated.TargetSizeRatio > 0 {
		if ocbp.Spec.ErasureCoded.CodingChunks > 0 || ocbp.Spec.ErasureCoded.DataChunks > 0 || ocbp.Spec.ErasureCoded.Algorithm != "" {
			return errors.New("invalid update: erasurecoded field is set already in previous object. cannot be changed to use replicated")
		}
	}
	return nil
}

func (p *CephBlockPool) ValidateDelete() error {
	return nil
}

// SnapshotSchedulesEnabled returns whether snapshot schedules are desired
func (p *MirroringSpec) SnapshotSchedulesEnabled() bool {
	return len(p.SnapshotSchedules) > 0
}
