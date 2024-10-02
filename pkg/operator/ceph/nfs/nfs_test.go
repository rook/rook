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

// Package nfs manages NFS ganesha servers for Ceph
package nfs

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileCephNFS_createConfigMap(t *testing.T) {
	s := scheme.Scheme

	clientset := k8sfake.NewSimpleClientset()

	c := &clusterd.Context{
		Executor:  &exectest.MockExecutor{},
		Clientset: clientset,
	}

	r := &ReconcileCephNFS{
		scheme:  s,
		context: c,
		clusterInfo: &cephclient.ClusterInfo{
			FSID:        "myfsid",
			CephVersion: cephver.Squid,
		},
		cephClusterSpec: &cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{
				Image: "quay.io/ceph/ceph:v15",
			},
		},
	}

	nfs := &cephv1.CephNFS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-nfs",
			Namespace: "rook-ceph-test-ns",
		},
		Spec: cephv1.NFSGaneshaSpec{
			RADOS: cephv1.GaneshaRADOSSpec{
				Pool:      "myfs-data0",
				Namespace: "nfs-test-ns",
			},
			Server: cephv1.GaneshaServerSpec{
				Active: 3,
			},
		},
	}

	t.Run("running multiple times should give the same hash", func(t *testing.T) {
		cmName, hash1, err := r.createConfigMap(nfs, "a")
		assert.NoError(t, err)
		assert.Equal(t, "rook-ceph-nfs-my-nfs-a", cmName)
		_, err = r.context.Clientset.CoreV1().ConfigMaps("rook-ceph-test-ns").Get(context.TODO(), cmName, metav1.GetOptions{})
		assert.NoError(t, err)

		_, hash2, err := r.createConfigMap(nfs, "a")
		assert.NoError(t, err)
		_, err = r.context.Clientset.CoreV1().ConfigMaps("rook-ceph-test-ns").Get(context.TODO(), cmName, metav1.GetOptions{})
		assert.NoError(t, err)

		assert.Equal(t, hash1, hash2)
	})

	t.Run("running with different IDs should give different hashes", func(t *testing.T) {
		cmName, hash1, err := r.createConfigMap(nfs, "a")
		assert.NoError(t, err)
		assert.Equal(t, "rook-ceph-nfs-my-nfs-a", cmName)
		_, err = r.context.Clientset.CoreV1().ConfigMaps("rook-ceph-test-ns").Get(context.TODO(), cmName, metav1.GetOptions{})
		assert.NoError(t, err)

		_, hash2, err := r.createConfigMap(nfs, "b")
		assert.NoError(t, err)
		_, err = r.context.Clientset.CoreV1().ConfigMaps("rook-ceph-test-ns").Get(context.TODO(), cmName, metav1.GetOptions{})
		assert.NoError(t, err)

		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("running with different configs should give different hashes", func(t *testing.T) {
		cmName, hash1, err := r.createConfigMap(nfs, "a")
		assert.NoError(t, err)
		assert.Equal(t, "rook-ceph-nfs-my-nfs-a", cmName)
		_, err = r.context.Clientset.CoreV1().ConfigMaps("rook-ceph-test-ns").Get(context.TODO(), cmName, metav1.GetOptions{})
		assert.NoError(t, err)

		nfs2 := nfs.DeepCopy()
		nfs2.Name = "nfs-two"
		_, hash2, err := r.createConfigMap(nfs2, "a")
		assert.NoError(t, err)
		_, err = r.context.Clientset.CoreV1().ConfigMaps("rook-ceph-test-ns").Get(context.TODO(), cmName, metav1.GetOptions{})
		assert.NoError(t, err)

		assert.NotEqual(t, hash1, hash2)
	})
}

func TestReconcileCephNFS_upCephNFS(t *testing.T) {
	ns := "up-ceph-ns-namespace"

	s := scheme.Scheme

	clientset := k8sfake.NewSimpleClientset()

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("executing command: %s %+v", command, args)
			if args[0] == "auth" {
				if args[1] == "get-or-create-key" {
					return "{\"key\":\"mysecurekey\"}", nil
				}
			}
			panic(errors.Errorf("unhandled command %s %v", command, args))
		},
	}

	client := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects().Build()

	c := &clusterd.Context{
		Executor:  executor,
		Client:    client,
		Clientset: clientset,
	}

	r := &ReconcileCephNFS{
		scheme:  s,
		context: c,
		clusterInfo: &cephclient.ClusterInfo{
			FSID:        "myfsid",
			CephVersion: cephver.Squid,
			Context:     context.TODO(),
			Namespace:   ns,
		},
		cephClusterSpec: &cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{
				Image: "quay.io/ceph/ceph:v15",
			},
		},
	}

	nfs := &cephv1.CephNFS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-nfs",
			Namespace: ns,
		},
		Spec: cephv1.NFSGaneshaSpec{
			RADOS: cephv1.GaneshaRADOSSpec{
				Pool:      "myfs-data0",
				Namespace: "nfs-test-ns",
			},
			Server: cephv1.GaneshaServerSpec{
				Active: 2,
			},
		},
	}

	err := r.upCephNFS(nfs)
	assert.NoError(t, err)

	deps, err := r.context.Clientset.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Len(t, deps.Items, 2)
	names := []string{}
	hashes := []string{}
	for _, dep := range deps.Items {
		names = append(names, dep.Name)
		assert.Contains(t, dep.Spec.Template.Annotations, "config-hash")
		hashes = append(hashes, dep.Spec.Template.Annotations["config-hash"])
	}
	assert.ElementsMatch(t, []string{"rook-ceph-nfs-my-nfs-a", "rook-ceph-nfs-my-nfs-b"}, names)
	assert.NotEqual(t, hashes[0], hashes[1])

	svcs, err := r.context.Clientset.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{})
	assert.NoError(t, err)
	// Each NFS server gets a service.
	assert.Len(t, svcs.Items, 2)
	names = []string{}
	for _, svc := range svcs.Items {
		names = append(names, svc.Name)
	}
	assert.ElementsMatch(t, []string{"rook-ceph-nfs-my-nfs-a", "rook-ceph-nfs-my-nfs-b"}, names)
}
