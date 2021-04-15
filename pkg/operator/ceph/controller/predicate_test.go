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
	"fmt"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		Spec: cephv1.PoolSpec{
			Replicated: cephv1.ReplicatedSpec{
				Size: oldReplicas,
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
		Spec: cephv1.PoolSpec{
			Replicated: cephv1.ReplicatedSpec{
				Size: oldReplicas,
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
	dum := &cephv1.CephBlockPool{}

	b := isCanary(dum)
	assert.False(t, b)

	d := &appsv1.Deployment{}
	b = isCanary(d)
	assert.False(t, b)

	d.Labels = map[string]string{
		"foo": "bar",
	}
	b = isCanary(d)
	assert.False(t, b)

	d.Labels["mon_canary"] = "true"
	b = isCanary(d)
	assert.True(t, b)
}

func TestIsCMToIgnoreOnUpdate(t *testing.T) {
	dum := &cephv1.CephBlockPool{}

	b := isCMTConfigOverride(dum)
	assert.False(t, b)

	cm := &corev1.ConfigMap{}
	b = isCMTConfigOverride(cm)
	assert.False(t, b)

	cm.Name = "rook-ceph-mon-endpoints"
	b = isCMTConfigOverride(cm)
	assert.False(t, b)

	cm.Name = "rook-config-override"
	b = isCMTConfigOverride(cm)
	assert.True(t, b)
}

func TestIsCMToIgnoreOnDelete(t *testing.T) {
	dum := &cephv1.CephBlockPool{}

	b := isCMToIgnoreOnDelete(dum)
	assert.False(t, b)

	cm := &corev1.ConfigMap{}
	b = isCMToIgnoreOnDelete(cm)
	assert.False(t, b)

	cm.Name = "rook-ceph-mon-endpoints"
	b = isCMToIgnoreOnDelete(cm)
	assert.False(t, b)

	cm.Name = "rook-ceph-osd-minikube-status"
	b = isCMToIgnoreOnDelete(cm)
	assert.True(t, b)
}

func TestIsSecretToIgnoreOnUpdate(t *testing.T) {
	dum := &cephv1.CephBlockPool{}

	b := isSecretToIgnoreOnUpdate(dum)
	assert.False(t, b)

	s := &corev1.Secret{}
	b = isSecretToIgnoreOnUpdate(s)
	assert.False(t, b)

	s.Name = "foo"
	b = isSecretToIgnoreOnUpdate(s)
	assert.False(t, b)

	s.Name = config.StoreName
	b = isSecretToIgnoreOnUpdate(s)
	assert.True(t, b)
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
