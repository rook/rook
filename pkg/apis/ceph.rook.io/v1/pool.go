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
	"reflect"

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

func (p *PoolSpec) IsEmpty() bool {
	return reflect.DeepEqual(*p, PoolSpec{})
}

func (p *PoolSpec) IsReplicated() bool {
	return p.Replicated != nil && p.Replicated.Size > 0
}

func (p *PoolSpec) IsErasureCoded() bool {
	return p.ErasureCoded != nil && (p.ErasureCoded.CodingChunks > 0 || p.ErasureCoded.DataChunks > 0)
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
	if !ps.IsErasureCoded() && !ps.IsReplicated() {
		return errors.New("invalid create: either of erasureCoded or replicated fields should be set")
	}

	if ps.IsErasureCoded() && ps.IsReplicated() {
		return errors.New("invalid create: both erasureCoded and replicated fields cannot be set at the same time")
	}

	if ps.IsErasureCoded() {
		if ps.ErasureCoded.DataChunks < 2 {
			return errors.New("invalid create: erasurecoded.datachunks needs minimum value of 2")
		}
		if ps.ErasureCoded.CodingChunks < 1 {
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

	if ocbp.Spec.IsErasureCoded() && !p.Spec.IsErasureCoded() {
		return errors.New("invalid update: erasureCoded field is set already in previous object. erasureCoded cannot be disabled in an update")
	}

	if ocbp.Spec.IsReplicated() && !p.Spec.IsReplicated() {
		return errors.New("invalid update: replicated field is set already in previous object. replicated cannot be disabled in an update")
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
