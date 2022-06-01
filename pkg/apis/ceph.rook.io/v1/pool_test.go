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
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidatePoolSpec(t *testing.T) {
	p := &CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ec-pool",
		},
		Spec: NamedBlockPoolSpec{
			PoolSpec: PoolSpec{
				ErasureCoded: ErasureCodedSpec{
					CodingChunks: 1,
					DataChunks:   2,
				},
			},
		},
	}
	err := validatePoolSpec(p.ToNamedPoolSpec())
	assert.NoError(t, err)

	p.Spec.ErasureCoded.DataChunks = 1
	err = validatePoolSpec(p.ToNamedPoolSpec())
	assert.Error(t, err)
}

func TestCephBlockPoolValidateUpdate(t *testing.T) {
	p := &CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ec-pool",
		},
		Spec: NamedBlockPoolSpec{
			PoolSpec: PoolSpec{
				Replicated: ReplicatedSpec{RequireSafeReplicaSize: true, Size: 3},
			},
		},
	}
	up := p.DeepCopy()
	up.Spec.ErasureCoded.DataChunks = 2
	up.Spec.ErasureCoded.CodingChunks = 1
	err := up.ValidateUpdate(p)
	assert.Error(t, err)

	// validate with different name in Spec.Name
	ip := p.DeepCopy()
	ip.Spec.Name = "new-ec-pool"
	err = ip.ValidateUpdate(p)
	assert.Error(t, err)
}

func TestMirroringSpec_SnapshotSchedulesEnabled(t *testing.T) {
	type fields struct {
		Enabled           bool
		Mode              string
		SnapshotSchedules []SnapshotScheduleSpec
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{"disabled", fields{Enabled: true, Mode: "pool", SnapshotSchedules: []SnapshotScheduleSpec{}}, false},
		{"enabled", fields{Enabled: true, Mode: "pool", SnapshotSchedules: []SnapshotScheduleSpec{{Interval: "2d"}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &MirroringSpec{
				Enabled:           tt.fields.Enabled,
				Mode:              tt.fields.Mode,
				SnapshotSchedules: tt.fields.SnapshotSchedules,
			}
			if got := p.SnapshotSchedulesEnabled(); got != tt.want {
				t.Errorf("MirroringSpec.SnapshotSchedulesEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
