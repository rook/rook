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
	"os"
	"testing"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCephCSIController(t *testing.T) {
	ctx := context.TODO()
	var (
		name      = "rook-ceph"
		namespace = "rook-ceph"
	)
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")
	os.Setenv(k8sutil.PodNameEnvVar, "rook-ceph-operator")
	os.Setenv(k8sutil.PodNamespaceEnvVar, namespace)

	os.Setenv("ROOK_CSI_ALLOW_UNSUPPORTED_VERSION", "true")
	CSIParam = Param{
		CSIPluginImage:   "image",
		RegistrarImage:   "image",
		ProvisionerImage: "image",
		AttacherImage:    "image",
		SnapshotterImage: "image",
	}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	t.Run("failure because no CephCluster", func(t *testing.T) {
		fakeClientSet := test.New(t, 1)
		test.SetFakeKubernetesVersion(fakeClientSet, "v1.21.0")
		c := &clusterd.Context{
			Clientset:     fakeClientSet,
			RookClientset: rookclient.NewSimpleClientset(),
		}

		_, err := c.Clientset.CoreV1().Pods(namespace).Create(ctx, test.FakeOperatorPod(namespace), metav1.CreateOptions{})
		assert.NoError(t, err)
		_, err = c.Clientset.AppsV1().ReplicaSets(namespace).Create(context.TODO(), test.FakeReplicaSet(namespace), metav1.CreateOptions{})
		assert.NoError(t, err)
		// Register operator types with the runtime scheme.
		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &v1.ConfigMap{}, &v1.ConfigMapList{}, &cephv1.CephClusterList{})

		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).Build()
		c.Client = cl

		// Create a ReconcileCSI object with the scheme and fake client.
		r := &ReconcileCSI{
			client:  cl,
			context: c,
			opConfig: controller.OperatorConfig{
				OperatorNamespace: namespace,
				Image:             "rook",
				ServiceAccount:    "foo",
			},
		}

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
	})

	t.Run("success ceph csi deployment", func(t *testing.T) {
		fakeClientSet := test.New(t, 1)
		test.SetFakeKubernetesVersion(fakeClientSet, "v1.21.0")
		c := &clusterd.Context{
			Clientset:     fakeClientSet,
			RookClientset: rookclient.NewSimpleClientset(),
		}
		_, err := c.Clientset.CoreV1().Pods(namespace).Create(ctx, test.FakeOperatorPod(namespace), metav1.CreateOptions{})
		assert.NoError(t, err)
		_, err = c.Clientset.AppsV1().ReplicaSets(namespace).Create(context.TODO(), test.FakeReplicaSet(namespace), metav1.CreateOptions{})
		assert.NoError(t, err)
		cephCluster := &cephv1.CephCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespace,
				Namespace: namespace,
			},
			Status: cephv1.ClusterStatus{
				Phase: "",
				CephVersion: &cephv1.ClusterVersion{
					Version: "14.2.9-0",
				},
				CephStatus: &cephv1.CephStatus{
					Health: "",
				},
			},
		}
		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{}, &cephv1.CephClusterList{})

		object := []runtime.Object{
			cephCluster,
		}
		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
		c.Client = cl
		// // Create a ReconcileCSI object with the scheme and fake client.
		r := &ReconcileCSI{
			client:  cl,
			context: c,
			opConfig: controller.OperatorConfig{
				OperatorNamespace: namespace,
				Image:             "rook",
				ServiceAccount:    "foo",
			},
		}

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)

		ds, err := c.Clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Equal(t, 2, len(ds.Items), ds)
	})
}
