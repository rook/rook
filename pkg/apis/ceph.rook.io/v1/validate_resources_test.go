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

func TestCephClusterValidateCreate(t *testing.T) {
	c := &CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph",
		},
		Spec: ClusterSpec{
			DataDirHostPath: "/var/lib/rook",
		},
	}
	err := c.ValidateCreate()
	assert.NoError(t, err)
	c.Spec.External.Enable = true
	c.Spec.Monitoring = MonitoringSpec{
		Enabled:        true,
		RulesNamespace: "rook-ceph",
	}
	err = c.ValidateCreate()
	assert.Error(t, err)
}

func TestValidatePoolSpec(t *testing.T) {
	p := &CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ec-pool",
		},
		Spec: PoolSpec{
			ErasureCoded: ErasureCodedSpec{
				CodingChunks: 1,
				DataChunks:   2,
			},
		},
	}
	err := ValidatePoolSpecs(p.Spec)
	assert.NoError(t, err)

	p.Spec.ErasureCoded.DataChunks = 1
	err = ValidatePoolSpecs(p.Spec)
	assert.Error(t, err)
}

func TestCephBlockPoolValidateUpdate(t *testing.T) {
	p := &CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ec-pool",
		},
		Spec: PoolSpec{
			Replicated: ReplicatedSpec{RequireSafeReplicaSize: true, Size: 3},
		},
	}
	up := p.DeepCopy()
	up.Spec.ErasureCoded.DataChunks = 2
	up.Spec.ErasureCoded.CodingChunks = 1
	err := up.ValidateUpdate(p)
	assert.Error(t, err)
}

func TestCephClusterValidateUpdate(t *testing.T) {
	c := &CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "rook-ceph",
		},
		Spec: ClusterSpec{
			DataDirHostPath: "/var/lib/rook",
		},
	}
	err := c.ValidateCreate()
	assert.NoError(t, err)

	// Updating the CRD specs with invalid values
	uc := c.DeepCopy()
	uc.Spec.DataDirHostPath = "var/rook"
	err = uc.ValidateUpdate(c)
	assert.Error(t, err)
}
