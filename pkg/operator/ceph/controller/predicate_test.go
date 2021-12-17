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

package controller

import (
	"context"
	"fmt"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	name             = "my-pool"
	namespace        = "rook-ceph"
	oldReplicas uint = 3
	newReplicas uint = 2
)

func TestObjectChanged(t *testing.T) {

	oldObject := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cephv1.NamedBlockPoolSpec{
			PoolSpec: cephv1.PoolSpec{
				Replicated: cephv1.ReplicatedSpec{
					Size: oldReplicas,
				},
			},
		},
		Status: &cephv1.CephBlockPoolStatus{
			Phase: "",
		},
	}

	newObject := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cephv1.NamedBlockPoolSpec{
			PoolSpec: cephv1.PoolSpec{
				Replicated: cephv1.ReplicatedSpec{
					Size: oldReplicas,
				},
			},
		},
		Status: &cephv1.CephBlockPoolStatus{
			Phase: "",
		},
	}

	// Identical
	changed, err := objectChanged(oldObject, newObject, "foo")
	assert.NoError(t, err)
	assert.False(t, changed)

	// Replica size changed
	oldObject.Spec.Replicated.Size = newReplicas
	changed, err = objectChanged(oldObject, newObject, "foo")
	assert.NoError(t, err)
	assert.True(t, changed)
}

func TestIsUpgrade(t *testing.T) {
	oldLabel := make(map[string]string)
	newLabel := map[string]string{
		"foo": "bar",
	}

	// no value do nothing
	b := isUpgrade(oldLabel, newLabel)
	assert.False(t, b)

	// different value do something
	newLabel["ceph_version"] = "15.2.0-octopus"
	b = isUpgrade(oldLabel, newLabel)
	assert.True(t, b, fmt.Sprintf("%v,%v", oldLabel, newLabel))

	// same value do nothing
	oldLabel["ceph_version"] = "15.2.0-octopus"
	newLabel["ceph_version"] = "15.2.0-octopus"
	b = isUpgrade(oldLabel, newLabel)
	assert.False(t, b, fmt.Sprintf("%v,%v", oldLabel, newLabel))

	// different value do something
	newLabel["ceph_version"] = "15.2.1-octopus"
	b = isUpgrade(oldLabel, newLabel)
	assert.True(t, b, fmt.Sprintf("%v,%v", oldLabel, newLabel))
}

func TestIsValidEvent(t *testing.T) {
	obj := "rook-ceph-mon-a"
	valid := []byte(`{
		"metadata": {},
		"spec": {},
		"status": {
		  "conditions": [
			{
			  "message": "ReplicaSet \"rook-ceph-mon-b-784fc58bf8\" is progressing.",
			  "reason": "ReplicaSetUpdated",
			  "type": "Progressing"
			}
		  ]
		}
	  }`)

	b := isValidEvent(valid, obj, false)
	assert.True(t, b)

	valid = []byte(`{"foo": "bar"}`)
	b = isValidEvent(valid, obj, false)
	assert.True(t, b)

	invalid := []byte(`{
		"metadata": {},
		"status": {},
	  }`)
	b = isValidEvent(invalid, obj, false)
	assert.False(t, b)
}

func TestIsCanary(t *testing.T) {
	blockPool := &cephv1.CephBlockPool{}

	assert.False(t, isCanary(blockPool))

	d := &appsv1.Deployment{}
	assert.False(t, isCanary(d))

	d.Labels = map[string]string{
		"foo": "bar",
	}
	assert.False(t, isCanary(d))

	d.Labels["mon_canary"] = "true"
	assert.True(t, isCanary(d))
}

func TestIsCMToIgnoreOnUpdate(t *testing.T) {
	blockPool := &cephv1.CephBlockPool{}
	assert.False(t, isCMTConfigOverride(blockPool))

	cm := &corev1.ConfigMap{}
	assert.False(t, isCMTConfigOverride(cm))

	cm.Name = "rook-ceph-mon-endpoints"
	assert.False(t, isCMTConfigOverride(cm))

	cm.Name = "rook-config-override"
	assert.True(t, isCMTConfigOverride(cm))
}

func TestIsCMToIgnoreOnDelete(t *testing.T) {
	blockPool := &cephv1.CephBlockPool{}
	assert.False(t, isCMToIgnoreOnDelete(blockPool))

	cm := &corev1.ConfigMap{}
	assert.False(t, isCMToIgnoreOnDelete(cm))

	cm.Name = "rook-ceph-mon-endpoints"
	assert.False(t, isCMToIgnoreOnDelete(cm))

	cm.Name = "rook-ceph-osd-minikube-status"
	assert.True(t, isCMToIgnoreOnDelete(cm))
}

func TestIsSecretToIgnoreOnUpdate(t *testing.T) {
	blockPool := &cephv1.CephBlockPool{}
	assert.False(t, isSecretToIgnoreOnUpdate(blockPool))

	s := &corev1.Secret{}
	assert.False(t, isSecretToIgnoreOnUpdate(s))

	s.Name = "foo"
	assert.False(t, isSecretToIgnoreOnUpdate(s))

	s.Name = config.StoreName
	assert.True(t, isSecretToIgnoreOnUpdate(s))
}

func TestIsDoNotReconcile(t *testing.T) {
	l := map[string]string{
		"foo": "bar",
	}

	// value not present
	b := IsDoNotReconcile(l)
	assert.False(t, b)

	// good value wrong content
	l["do_not_reconcile"] = "false"
	b = IsDoNotReconcile(l)
	assert.False(t, b)

	// good value and good content
	l["do_not_reconcile"] = "true"
	b = IsDoNotReconcile(l)
	assert.True(t, b)
}

func TestDuplicateCephClusters(t *testing.T) {
	ctx := context.TODO()
	namespace := "rook-ceph"
	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-a",
			Namespace: namespace,
		},
	}
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{}, &cephv1.CephClusterList{})

	t.Run("success - only one ceph cluster", func(t *testing.T) {
		object := []runtime.Object{
			cephCluster,
		}
		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
		assert.False(t, DuplicateCephClusters(ctx, cl, cephCluster, false))
	})

	t.Run("success - we have more than one cluster but they are in different namespaces", func(t *testing.T) {
		dup := &cephv1.CephCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-b",
				Namespace: "anotherns",
			},
		}
		object := []runtime.Object{
			cephCluster,
			dup,
		}
		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
		assert.False(t, DuplicateCephClusters(ctx, cl, dup, true))
	})

	t.Run("fail - we have more than one cluster in the same namespace", func(t *testing.T) {
		dup := &cephv1.CephCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-b",
				Namespace: namespace,
			},
		}
		object := []runtime.Object{
			cephCluster,
			dup,
		}
		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
		assert.True(t, DuplicateCephClusters(ctx, cl, dup, true))
	})
}
