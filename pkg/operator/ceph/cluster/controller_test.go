/*
Copyright 2016 The Rook Authors. All rights reserved.

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
	"context"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileDeleteCephCluster(t *testing.T) {
	ctx := context.TODO()
	cephNs := "rook-ceph"
	clusterName := "my-cluster"
	nsName := types.NamespacedName{
		Name:      clusterName,
		Namespace: cephNs,
	}

	fakeCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              clusterName,
			Namespace:         cephNs,
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
		},
	}

	fakePool := &cephv1.CephBlockPool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CephBlockPool",
			APIVersion: "ceph.rook.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-block-pool",
			Namespace: cephNs,
		},
	}

	// create a Rook-Ceph scheme to use for our tests
	scheme := runtime.NewScheme()
	assert.NoError(t, cephv1.AddToScheme(scheme))

	t.Run("deletion blocked while dependencies exist", func(t *testing.T) {
		// set up clusterd.Context
		clusterdCtx := &clusterd.Context{
			Clientset: k8sfake.NewSimpleClientset(),
		}

		// create the cluster controller and tell it that the cluster has been deleted
		controller := NewClusterController(clusterdCtx, "")
		fakeRecorder := record.NewFakeRecorder(5)
		controller.recorder = fakeRecorder

		// Create a fake client to mock API calls
		// Make sure it has the fake CephCluster that is to be deleted in it
		client := clientfake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(fakeCluster, fakePool).Build()

		err := corev1.AddToScheme(scheme)
		assert.NoError(t, err)
		// Create a ReconcileCephClient object with the scheme and fake client.
		reconcileCephCluster := &ReconcileCephCluster{
			client:            client,
			scheme:            scheme,
			context:           clusterdCtx,
			clusterController: controller,
			opManagerContext:  context.TODO(),
		}

		req := reconcile.Request{NamespacedName: nsName}

		resp, err := reconcileCephCluster.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.NotZero(t, resp.RequeueAfter)
		event := <-fakeRecorder.Events
		assert.Contains(t, event, "CephBlockPool")
		assert.Contains(t, event, "my-block-pool")

		blockedCluster := &cephv1.CephCluster{}
		err = client.Get(ctx, nsName, blockedCluster)
		assert.NoError(t, err)
		status := blockedCluster.Status
		assert.Equal(t, cephv1.ConditionDeleting, status.Phase)
		assert.Equal(t, cephv1.ClusterState(cephv1.ConditionDeleting), status.State)
		assert.Equal(t, corev1.ConditionTrue, cephv1.FindStatusCondition(status.Conditions, cephv1.ConditionDeleting).Status)
		assert.Equal(t, corev1.ConditionTrue, cephv1.FindStatusCondition(status.Conditions, cephv1.ConditionDeletionIsBlocked).Status)

		// delete blocking dependency
		err = client.Delete(ctx, fakePool)
		assert.NoError(t, err)

		resp, err = reconcileCephCluster.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, resp.IsZero())
		event = <-fakeRecorder.Events
		assert.Contains(t, event, "Deleting")

		unblockedCluster := &cephv1.CephCluster{}
		err = client.Get(ctx, nsName, unblockedCluster)
		assert.Error(t, err)
		assert.True(t, kerrors.IsNotFound(err))
	})
}

func TestRemoveFinalizers(t *testing.T) {
	reconcileCephCluster := &ReconcileCephCluster{
		opManagerContext: context.TODO(),
	}
	s := scheme.Scheme
	fakeObject1 := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "rook-ceph",
			Finalizers: []string{
				"cephcluster.ceph.rook.io",
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephCluster",
		},
	}
	schema1 := schema.GroupVersion{Group: "ceph.rook.io", Version: "v1"}
	fakeObject2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "rook-ceph",
			Finalizers: []string{
				"ceph.rook.io/disaster-protection",
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "Secret",
		},
	}
	schema2 := schema.GroupVersion{Group: "", Version: "v1"}

	tests := []struct {
		name      string
		finalizer string
		object    client.Object
		schema    schema.GroupVersion
	}{
		{"CephCluster", "cephcluster.ceph.rook.io", fakeObject1, schema1},
		{"mon secret", "ceph.rook.io/disaster-protection", fakeObject2, schema2},
	}

	for _, tt := range tests {
		t.Run("delete finalizer for "+tt.name, func(t *testing.T) {
			fakeObject, err := meta.Accessor(tt.object)
			assert.NoError(t, err)
			object := []runtime.Object{
				tt.object,
			}
			s.AddKnownTypes(tt.schema, tt.object)
			cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

			assert.NotEmpty(t, fakeObject.GetFinalizers())
			name := types.NamespacedName{Name: fakeObject.GetName(), Namespace: fakeObject.GetNamespace()}
			err = reconcileCephCluster.removeFinalizer(cl, name, tt.object, tt.finalizer)
			assert.NoError(t, err)
			assert.Empty(t, fakeObject.GetFinalizers())
		})
	}
}
