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
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func Test_findCSIChange(t *testing.T) {
	t.Run("no match", func(t *testing.T) {
		var str = `  map[string]string{
        "CSI_FORCE_CEPHFS_KERNEL_CLIENT":     "true",
        "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION": "false",
        "ROOK_CSI_ENABLE_GRPC_METRICS":       "true",
        "ROOK_CSI_ENABLE_RBD":                "true",
        "ROOK_ENABLE_DISCOVERY_DAEMON":       "false",
-       "ROOK_LOG_LEVEL":                     "INFO",
+       "ROOK_LOG_LEVEL":                     "DEBUG",
        "ROOK_OBC_WATCH_OPERATOR_NAMESPACE":  "true",
  }`
		b := findCSIChange(str)
		assert.False(t, b)
	})

	t.Run("match on addition", func(t *testing.T) {
		var str = `  map[string]string{
        "CSI_FORCE_CEPHFS_KERNEL_CLIENT":     "true",
        "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION": "false",
+       "ROOK_CSI_ENABLE_CEPHFS":             "false",
        "ROOK_CSI_ENABLE_GRPC_METRICS":       "true",
        "ROOK_CSI_ENABLE_RBD":                "true",
        "ROOK_ENABLE_DISCOVERY_DAEMON":       "false",
-       "ROOK_LOG_LEVEL":                     "INFO",
+       "ROOK_LOG_LEVEL":                     "DEBUG",
        "ROOK_OBC_WATCH_OPERATOR_NAMESPACE":  "true",
  }`
		b := findCSIChange(str)
		assert.True(t, b)
	})

	t.Run("match on deletion", func(t *testing.T) {
		var str = `  map[string]string{
        "CSI_FORCE_CEPHFS_KERNEL_CLIENT":     "true",
        "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION": "false",
-       "ROOK_CSI_ENABLE_CEPHFS":             "true",
        "ROOK_CSI_ENABLE_GRPC_METRICS":       "true",
        "ROOK_CSI_ENABLE_RBD":                "true",
        "ROOK_ENABLE_DISCOVERY_DAEMON":       "false",
-       "ROOK_LOG_LEVEL":                     "INFO",
+       "ROOK_LOG_LEVEL":                     "DEBUG",
        "ROOK_OBC_WATCH_OPERATOR_NAMESPACE":  "true",
  }`
		b := findCSIChange(str)
		assert.True(t, b)
	})

	t.Run("match on addition and deletion", func(t *testing.T) {
		var str = `  map[string]string{
        "CSI_FORCE_CEPHFS_KERNEL_CLIENT":     "true",
        "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION": "false",
-       "ROOK_CSI_ENABLE_CEPHFS":             "true",
+       "ROOK_CSI_ENABLE_CEPHFS":             "false",
        "ROOK_CSI_ENABLE_GRPC_METRICS":       "true",
        "ROOK_CSI_ENABLE_RBD":                "true",
        "ROOK_ENABLE_DISCOVERY_DAEMON":       "false",
-       "ROOK_LOG_LEVEL":                     "INFO",
+       "ROOK_LOG_LEVEL":                     "DEBUG",
        "ROOK_OBC_WATCH_OPERATOR_NAMESPACE":  "true",
  }`
		b := findCSIChange(str)
		assert.True(t, b)
	})

	t.Run("match with CSI_ name", func(t *testing.T) {
		var str = `  map[string]string{
        "CSI_FORCE_CEPHFS_KERNEL_CLIENT":     "true",
        "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION": "false",
-       "CSI_PROVISIONER_TOLERATIONS":             "true",
+       "CSI_PROVISIONER_TOLERATIONS":             "false",
        "ROOK_CSI_ENABLE_GRPC_METRICS":       "true",
        "ROOK_CSI_ENABLE_RBD":                "true",
        "ROOK_ENABLE_DISCOVERY_DAEMON":       "false",
-       "ROOK_LOG_LEVEL":                     "INFO",
+       "ROOK_LOG_LEVEL":                     "DEBUG",
        "ROOK_OBC_WATCH_OPERATOR_NAMESPACE":  "true",
  }`
		b := findCSIChange(str)
		assert.True(t, b)
	})

	t.Run("match with CSI_ and ROOK_CSI_ name", func(t *testing.T) {
		var str = `  map[string]string{
        "CSI_FORCE_CEPHFS_KERNEL_CLIENT":     "true",
        "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION": "false",
-       "CSI_PROVISIONER_TOLERATIONS":             "true",
+       "CSI_PROVISIONER_TOLERATIONS":             "false",
-       "ROOK_CSI_ENABLE_CEPHFS":             "true",
+       "ROOK_CSI_ENABLE_CEPHFS":             "false",
        "ROOK_CSI_ENABLE_GRPC_METRICS":       "true",
        "ROOK_CSI_ENABLE_RBD":                "true",
        "ROOK_ENABLE_DISCOVERY_DAEMON":       "false",
-       "ROOK_LOG_LEVEL":                     "INFO",
+       "ROOK_LOG_LEVEL":                     "DEBUG",
        "ROOK_OBC_WATCH_OPERATOR_NAMESPACE":  "true",
  }`
		b := findCSIChange(str)
		assert.True(t, b)
	})
}

func Test_predicateController(t *testing.T) {
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	var (
		client    client.WithWatch
		namespace v1.Namespace
		p         predicate.Funcs
		c         event.CreateEvent
		d         event.DeleteEvent
		u         event.UpdateEvent
		cm        v1.ConfigMap
		cm2       v1.ConfigMap
		cluster   cephv1.CephCluster
		cluster2  cephv1.CephCluster
	)

	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &v1.ConfigMap{}, &v1.ConfigMapList{}, &cephv1.CephCluster{}, &cephv1.CephClusterList{})
	client = fake.NewClientBuilder().WithScheme(s).Build()

	t.Run("create event is not the one we are looking for", func(t *testing.T) {
		c = event.CreateEvent{Object: &namespace}
		p = predicateController(context.TODO(), client, "rook-ceph")
		assert.False(t, p.Create(c))
	})

	t.Run("create event is a CM but not the operator config", func(t *testing.T) {
		c = event.CreateEvent{Object: &cm}
		p = predicateController(context.TODO(), client, "rook-ceph")
		assert.False(t, p.Create(c))
	})

	t.Run("delete event is a CM and it's not the operator config", func(t *testing.T) {
		d = event.DeleteEvent{Object: &cm}
		p = predicateController(context.TODO(), client, "rook-ceph")
		assert.False(t, p.Delete(d))
	})

	t.Run("create event is a CM and it's the operator config", func(t *testing.T) {
		cm.Name = "rook-ceph-operator-config"
		c = event.CreateEvent{Object: &cm}
		p = predicateController(context.TODO(), client, "rook-ceph")
		assert.True(t, p.Create(c))
	})

	t.Run("delete event is a CM and it's the operator config", func(t *testing.T) {
		d = event.DeleteEvent{Object: &cm}
		p = predicateController(context.TODO(), client, "rook-ceph")
		assert.True(t, p.Delete(d))
	})

	t.Run("update event is a CM and it's the operator config but nothing changed", func(t *testing.T) {
		u = event.UpdateEvent{ObjectOld: &cm, ObjectNew: &cm}
		p = predicateController(context.TODO(), client, "rook-ceph")
		assert.False(t, p.Update(u))
	})

	t.Run("update event is a CM and the content changed but with incorrect setting", func(t *testing.T) {
		cm.Data = map[string]string{"foo": "bar"}
		cm2.Name = "rook-ceph-operator-config"
		cm2.Data = map[string]string{"foo": "ooo"}
		u = event.UpdateEvent{ObjectOld: &cm, ObjectNew: &cm2}
		p = predicateController(context.TODO(), client, "rook-ceph")
		assert.False(t, p.Update(u))
	})

	t.Run("update event is a CM and the content changed with correct setting", func(t *testing.T) {
		cm.Data = map[string]string{"foo": "bar"}
		cm2.Name = "rook-ceph-operator-config"
		cm2.Data = map[string]string{"CSI_PROVISIONER_REPLICAS": "2"}
		u = event.UpdateEvent{ObjectOld: &cm, ObjectNew: &cm2}
		p = predicateController(context.TODO(), client, "rook-ceph")
		assert.True(t, p.Update(u))
	})

	t.Run("create event is a CephCluster and it's the first instance and a cm is present", func(t *testing.T) {
		cm.Namespace = "rook-ceph"
		err := client.Create(context.TODO(), &cm)
		assert.NoError(t, err)
		cluster.Generation = 1
		c = event.CreateEvent{Object: &cluster}
		p = predicateController(context.TODO(), client, "rook-ceph")
		assert.True(t, p.Create(c))
	})

	t.Run("create event is a CephCluster and it's not the first instance", func(t *testing.T) {
		cluster.Generation = 2
		c = event.CreateEvent{Object: &cluster}
		p = predicateController(context.TODO(), client, "rook-ceph")
		assert.False(t, p.Create(c))
	})

	t.Run("create event is a CephCluster and it's not the first instance but no cm so we reconcile", func(t *testing.T) {
		err := client.Delete(context.TODO(), &cm)
		assert.NoError(t, err)
		cluster.Generation = 2
		c = event.CreateEvent{Object: &cluster}
		p = predicateController(context.TODO(), client, "rook-ceph")
		assert.True(t, p.Create(c))
	})

	t.Run("create event is a CephCluster and a cluster already exists - not reconciling", func(t *testing.T) {
		cluster.Name = "ceph1"
		cluster2.Name = "ceph2"
		cluster.Namespace = "rook-ceph"
		cluster2.Namespace = "rook-ceph"
		err := client.Create(context.TODO(), &cluster)
		assert.NoError(t, err)
		err = client.Create(context.TODO(), &cluster2)
		assert.NoError(t, err)
		c = event.CreateEvent{Object: &cluster2}
		assert.Equal(t, "rook-ceph", c.Object.GetNamespace())
		p = predicateController(context.TODO(), client, "rook-ceph")
		assert.False(t, p.Create(c))
	})
}
