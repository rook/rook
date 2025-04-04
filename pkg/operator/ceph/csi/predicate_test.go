/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package csi

import (
	"context"
	"testing"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func Test_cmPredicate(t *testing.T) {
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)

	t.Run("create event is a CM but not the operator config", func(t *testing.T) {
		cm := &corev1.ConfigMap{}
		e := event.TypedCreateEvent[*corev1.ConfigMap]{Object: cm}
		p := cmPredicate()
		assert.False(t, p.Create(e))
	})

	t.Run("delete event is a CM and it's not the operator config", func(t *testing.T) {
		cm := &corev1.ConfigMap{}
		e := event.TypedDeleteEvent[*corev1.ConfigMap]{Object: cm}
		p := cmPredicate()
		assert.False(t, p.Delete(e))
	})

	t.Run("create event is a CM and it's the operator config", func(t *testing.T) {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph-operator-config"}}
		e := event.TypedCreateEvent[*corev1.ConfigMap]{Object: cm}
		p := cmPredicate()
		assert.True(t, p.Create(e))
	})

	t.Run("delete event is a CM and it's the operator config", func(t *testing.T) {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph-operator-config"}}
		e := event.TypedDeleteEvent[*corev1.ConfigMap]{Object: cm}
		p := cmPredicate()
		assert.True(t, p.Delete(e))
	})

	t.Run("update event is a CM and it's the operator config but nothing changed", func(t *testing.T) {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph-operator-config"}}
		e := event.TypedUpdateEvent[*corev1.ConfigMap]{ObjectOld: cm, ObjectNew: cm}
		p := cmPredicate()
		assert.False(t, p.Update(e))
	})

	t.Run("update event is a CM and the content changed with correct setting", func(t *testing.T) {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph-operator-config"}}
		cm2 := cm.DeepCopy()
		cm.Data = map[string]string{"foo": "bar"}
		cm2.Data = map[string]string{"CSI_PROVISIONER_REPLICAS": "2"}
		e := event.TypedUpdateEvent[*corev1.ConfigMap]{ObjectOld: cm, ObjectNew: cm2}
		p := cmPredicate()
		assert.True(t, p.Update(e))
	})
}

func Test_cephClusterPredicate(t *testing.T) {
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)

	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &corev1.ConfigMap{}, &corev1.ConfigMapList{}, &cephv1.CephCluster{}, &cephv1.CephClusterList{})

	t.Run("create event is a CephCluster and it's the first instance and a cm is present", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(s).Build()

		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph-operator-config", Namespace: "rook-ceph"}}
		err := client.Create(context.TODO(), cm)
		require.NoError(t, err)

		cluster := &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph", Generation: 1}}
		e := event.TypedCreateEvent[*cephv1.CephCluster]{Object: cluster}
		p := cephClusterPredicate(context.TODO(), client, "rook-ceph")
		assert.True(t, p.Create(e))
	})

	t.Run("create event is a CephCluster and it's not the first instance", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(s).Build()

		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph-operator-config", Namespace: "rook-ceph"}}
		err := client.Create(context.TODO(), cm)
		require.NoError(t, err)

		cluster := &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph", Generation: 2}}
		e := event.TypedCreateEvent[*cephv1.CephCluster]{Object: cluster}
		p := cephClusterPredicate(context.TODO(), client, "rook-ceph")
		assert.False(t, p.Create(e))
	})

	t.Run("create event is a CephCluster and it's not the first instance but no cm so we reconcile", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(s).Build()

		cluster := &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph", Generation: 2}}
		e := event.TypedCreateEvent[*cephv1.CephCluster]{Object: cluster}
		p := cephClusterPredicate(context.TODO(), client, "rook-ceph")
		assert.True(t, p.Create(e))
	})

	t.Run("create event is a CephCluster and a cluster already exists - not reconciling", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(s).Build()

		{
			cluster := &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Name: "ceph1", Namespace: "rook-ceph", Generation: 2}}
			err := client.Create(context.TODO(), cluster)
			require.NoError(t, err)
		}

		cluster := &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Name: "ceph2", Namespace: "rook-ceph"}}
		err := client.Create(context.TODO(), cluster)
		require.NoError(t, err)

		e := event.TypedCreateEvent[*cephv1.CephCluster]{Object: cluster}
		p := cephClusterPredicate(context.TODO(), client, "rook-ceph")
		assert.False(t, p.Create(e))
	})
}
