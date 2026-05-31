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

package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	testop "github.com/rook/rook/pkg/operator/test"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCleanupJobSpec(t *testing.T) {
	expectedHostPath := "var/lib/rook"
	expectedNamespace := "test-rook-ceph"
	cluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: expectedNamespace,
		},
		Spec: cephv1.ClusterSpec{
			DataDirHostPath: expectedHostPath,
			CleanupPolicy: cephv1.CleanupPolicySpec{
				Confirmation: "yes-really-destroy-data",
			},
		},
	}
	clientset := testop.New(t, 3)
	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookfake.NewSimpleClientset(),
	}
	controller := NewClusterController(context, "")
	podTemplateSpec := controller.cleanUpJobTemplateSpec(cluster, "monSecret", "28b87851-8dc1-46c8-b1ec-90ec51a47c89")
	assert.Equal(t, expectedHostPath, podTemplateSpec.Spec.Containers[0].Env[0].Value)
	assert.Equal(t, expectedNamespace, podTemplateSpec.Spec.Containers[0].Env[1].Value)
}

func TestCleanupPlacement(t *testing.T) {
	// no tolerations end up in an empty list of tolerations
	c := cephv1.ClusterSpec{}
	p := getCleanupPlacement(c)
	assert.Equal(t, cephv1.Placement{}, p)

	// add tolerations for each of the daemons
	c.Placement = cephv1.PlacementSpec{}
	c.Placement[cephv1.KeyAll] = cephv1.Placement{Tolerations: []v1.Toleration{{Key: "allToleration"}}}
	p = getCleanupPlacement(c)
	assert.Equal(t, c.Placement[cephv1.KeyAll], p)

	c.Placement[cephv1.KeyMon] = cephv1.Placement{Tolerations: []v1.Toleration{{Key: "monToleration"}}}
	p = getCleanupPlacement(c)
	assert.Equal(t, 2, len(p.Tolerations))

	c.Placement[cephv1.KeyMgr] = cephv1.Placement{Tolerations: []v1.Toleration{{Key: "mgrToleration"}}}
	p = getCleanupPlacement(c)
	assert.Equal(t, 3, len(p.Tolerations))

	c.Placement[cephv1.KeyMonArbiter] = cephv1.Placement{Tolerations: []v1.Toleration{{Key: "monArbiterToleration"}}}
	p = getCleanupPlacement(c)
	assert.Equal(t, 4, len(p.Tolerations))

	c.Placement[cephv1.KeyOSD] = cephv1.Placement{Tolerations: []v1.Toleration{{Key: "osdToleration"}}}
	p = getCleanupPlacement(c)
	assert.Equal(t, 5, len(p.Tolerations))

	c.Storage.StorageClassDeviceSets = []cephv1.StorageClassDeviceSet{
		{Placement: cephv1.Placement{Tolerations: []v1.Toleration{{Key: "deviceSetToleration"}}}},
	}
	p = getCleanupPlacement(c)
	assert.Equal(t, 6, len(p.Tolerations))
}
