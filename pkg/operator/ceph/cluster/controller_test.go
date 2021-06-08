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
	"os"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"github.com/tevino/abool"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcile_DeleteCephCluster(t *testing.T) {
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
			// reconcile looks for fake dependencies in the dynamic clientset
			DynamicClientset:           dynamicfake.NewSimpleDynamicClient(scheme, fakePool),
			RequestCancelOrchestration: abool.New(),
		}

		// set up ClusterController
		volumeAttachmentController := &attachment.MockAttachment{
			MockList: func(namespace string) (*rookalpha.VolumeList, error) {
				t.Log("test vol attach list")
				return &rookalpha.VolumeList{Items: []rookalpha.Volume{}}, nil
			},
		}
		operatorConfigCallbacks := []func() error{
			func() error {
				t.Log("test op config callback")
				return nil
			},
		}
		addCallbacks := []func() error{
			func() error {
				t.Log("test success callback")
				return nil
			},
		}

		// create the cluster controller and tell it that the cluster has been deleted
		controller := NewClusterController(clusterdCtx, "", volumeAttachmentController, operatorConfigCallbacks, addCallbacks)
		fakeRecorder := record.NewFakeRecorder(5)
		controller.recorder = k8sutil.NewEventReporter(fakeRecorder)

		// Create a fake client to mock API calls
		// Make sure it has the fake CephCluster that is to be deleted in it
		client := clientfake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(fakeCluster).Build()

		// Create a ReconcileCephClient object with the scheme and fake client.
		reconcileCephCluster := &ReconcileCephCluster{
			client:            client,
			scheme:            scheme,
			context:           clusterdCtx,
			clusterController: controller,
		}

		req := reconcile.Request{NamespacedName: nsName}

		resp, err := reconcileCephCluster.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.NotZero(t, resp.RequeueAfter)
		event := <-fakeRecorder.Events
		assert.Contains(t, event, "CephBlockPools")
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
		gvr := cephv1.SchemeGroupVersion.WithResource("cephblockpools")
		err = clusterdCtx.DynamicClientset.Resource(gvr).Namespace(cephNs).Delete(ctx, "my-block-pool", metav1.DeleteOptions{})
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

func Test_checkIfVolumesExist(t *testing.T) {
	t.Run("flexvolume enabled", func(t *testing.T) {
		nodeName := "node841"
		clusterName := "cluster684"
		pvName := "pvc-540"
		rookSystemNamespace := "rook-system-6413"

		os.Setenv("ROOK_ENABLE_FLEX_DRIVER", "true")
		os.Setenv(k8sutil.PodNamespaceEnvVar, rookSystemNamespace)
		defer os.Unsetenv("ROOK_ENABLE_FLEX_DRIVER")
		defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

		context := &clusterd.Context{
			Clientset: testop.New(t, 3),
		}
		listCount := 0
		volumeAttachmentController := &attachment.MockAttachment{
			MockList: func(namespace string) (*rookalpha.VolumeList, error) {
				listCount++
				if listCount == 1 {
					// first listing returns an existing volume attachment, so the controller should wait
					return &rookalpha.VolumeList{
						Items: []rookalpha.Volume{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name:      pvName,
									Namespace: rookSystemNamespace,
								},
								Attachments: []rookalpha.Attachment{
									{
										Node:        nodeName,
										ClusterName: clusterName,
									},
								},
							},
						},
					}, nil
				}

				// subsequent listings should return no volume attachments, meaning that they have all
				// been cleaned up and the controller can move on.
				return &rookalpha.VolumeList{Items: []rookalpha.Volume{}}, nil

			},
		}
		operatorConfigCallbacks := []func() error{
			func() error {
				logger.Infof("test success callback")
				return nil
			},
		}
		addCallbacks := []func() error{
			func() error {
				logger.Infof("test success callback")
				return nil
			},
		}
		// create the cluster controller and tell it that the cluster has been deleted
		controller := NewClusterController(context, "", volumeAttachmentController, operatorConfigCallbacks, addCallbacks)
		clusterToDelete := &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Namespace: clusterName}}

		// The test returns a volume on the first call
		assert.Error(t, controller.checkIfVolumesExist(clusterToDelete))

		// The test does not return volumes on the second call
		assert.NoError(t, controller.checkIfVolumesExist(clusterToDelete))
	})

	t.Run("flexvolume disabled (CSI)", func(t *testing.T) {
		clusterName := "cluster684"
		rookSystemNamespace := "rook-system-6413"

		os.Setenv("ROOK_ENABLE_FLEX_DRIVER", "false")
		os.Setenv(k8sutil.PodNamespaceEnvVar, rookSystemNamespace)
		defer os.Unsetenv("ROOK_ENABLE_FLEX_DRIVER")
		defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

		context := &clusterd.Context{
			Clientset: testop.New(t, 3),
		}
		listCount := 0
		volumeAttachmentController := &attachment.MockAttachment{
			MockList: func(namespace string) (*rookalpha.VolumeList, error) {
				listCount++
				return &rookalpha.VolumeList{Items: []rookalpha.Volume{}}, nil

			},
		}
		operatorConfigCallbacks := []func() error{
			func() error {
				logger.Infof("test success callback")
				return nil
			},
		}
		addCallbacks := []func() error{
			func() error {
				logger.Infof("test success callback")
				os.Setenv("ROOK_ENABLE_FLEX_DRIVER", "true")
				os.Setenv(k8sutil.PodNamespaceEnvVar, rookSystemNamespace)
				defer os.Unsetenv("ROOK_ENABLE_FLEX_DRIVER")
				return nil
			},
		}
		// create the cluster controller and tell it that the cluster has been deleted
		controller := NewClusterController(context, "", volumeAttachmentController, operatorConfigCallbacks, addCallbacks)
		clusterToDelete := &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Namespace: clusterName}}
		assert.NoError(t, controller.checkIfVolumesExist(clusterToDelete))

		// Ensure that the listing of volume attachments was never called.
		assert.Equal(t, 0, listCount)
	})
}
