/*
Copyright 2022 The Rook Authors. All rights reserved.

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

package osd

import (
	"context"
	"fmt"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	testexec "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

func TestRemovePVCs(t *testing.T) {
	pvcSuffix := 0
	pvcReactor := func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		// PVCs are created with generateName used, and we need to capture the create calls and
		// generate a name for them in order for PVCs to all have unique names.
		createAction, ok := action.(k8stesting.CreateAction)
		if !ok {
			t.Fatal("not a create action")
			return false, nil, nil
		}
		obj := createAction.GetObject()
		pvc, ok := obj.(*corev1.PersistentVolumeClaim)
		if !ok {
			t.Fatal("not a PVC")
			return false, nil, nil
		}
		if pvc.Name == "" {
			pvc.Name = fmt.Sprintf("%s-%d", pvc.GenerateName, pvcSuffix)
			logger.Info("generated name for PVC:", pvc.Name)
			pvcSuffix++
		}
		// setting pvc.Name above modifies the action in-place before future reactors occur
		// we want the default reactor to create the resource, so return false as if we did nothing
		return false, nil, nil
	}
	ns := "testns"
	ctx := context.TODO()
	clusterInfo := client.AdminTestClusterInfo(ns)

	t.Run("remove osd with data pvc", func(t *testing.T) {
		clientset := testexec.New(t, 1)
		clientset.PrependReactor("create", "persistentvolumeclaims", pvcReactor)
		context := &clusterd.Context{
			Clientset: clientset,
		}

		// Create 3 PVCs for two OSDs in the device set
		deviceSet := cephv1.StorageClassDeviceSet{
			Name:                 "mydata",
			Count:                2,
			Portable:             true,
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{testVolumeClaim("data")},
			SchedulerName:        "custom-scheduler",
		}
		err := createTestPVCs(context, clusterInfo, deviceSet)
		assert.NoError(t, err)

		// Verify the PVCs all exist
		pvcs, err := clientset.CoreV1().PersistentVolumeClaims(clusterInfo.Namespace).List(ctx, metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Equal(t, 2, len(pvcs.Items))

		// Verify the PVCs all exist for the given OSD
		selector := fmt.Sprintf("%s=%s", oposd.CephSetIndexLabelKey, "0")
		pvcs, err = clientset.CoreV1().PersistentVolumeClaims(clusterInfo.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(pvcs.Items))
		assert.Equal(t, "mydata-data-0-0", pvcs.Items[0].Name)

		// Remove the PVCs for one of the OSDs
		removePVCs(context, clusterInfo, "mydata-data-0-0", false)

		// Verify the PVCs all exist
		pvcs, err = clientset.CoreV1().PersistentVolumeClaims(clusterInfo.Namespace).List(ctx, metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(pvcs.Items))
		assert.Equal(t, "mydata-data-1-1", pvcs.Items[0].Name)
	})

	t.Run("remove osd with metadata and wal pvcs", func(t *testing.T) {
		clientset := testexec.New(t, 1)
		clientset.PrependReactor("create", "persistentvolumeclaims", pvcReactor)
		context := &clusterd.Context{
			Clientset: clientset,
		}

		// Create 3 PVCs for two OSDs in the device set
		deviceSet := cephv1.StorageClassDeviceSet{
			Name:                 "mydata",
			Count:                2,
			Portable:             true,
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{testVolumeClaim("data"), testVolumeClaim("metadata"), testVolumeClaim("wal")},
			SchedulerName:        "custom-scheduler",
		}
		err := createTestPVCs(context, clusterInfo, deviceSet)
		assert.NoError(t, err)

		// Verify the PVCs all exist
		pvcs, err := clientset.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Equal(t, 6, len(pvcs.Items))

		// Verify the PVCs all exist for the given OSD
		selector := fmt.Sprintf("%s=%s", oposd.CephSetIndexLabelKey, "0")
		pvcs, err = clientset.CoreV1().PersistentVolumeClaims(clusterInfo.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		assert.NoError(t, err)
		assert.Equal(t, 3, len(pvcs.Items))

		// Remove the PVCs for one of the OSDs
		removePVCs(context, clusterInfo, "mydata-data-0-2", false)

		// Verify the PVCs all deleted for the given OSD
		pvcs, err = clientset.CoreV1().PersistentVolumeClaims(clusterInfo.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		assert.NoError(t, err)
		assert.Equal(t, 0, len(pvcs.Items))
		// Verify the PVCs for the other OSD still exist
		pvcs, err = clientset.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Equal(t, 3, len(pvcs.Items))
	})
}

func createTestPVCs(clusterdContext *clusterd.Context, clusterInfo *client.ClusterInfo, deviceSet cephv1.StorageClassDeviceSet) error {
	spec := cephv1.ClusterSpec{
		Storage: cephv1.StorageScopeSpec{StorageClassDeviceSets: []cephv1.StorageClassDeviceSet{deviceSet}},
	}
	cluster := oposd.New(clusterdContext, clusterInfo, spec, "")
	return cluster.PrepareStorageClassDeviceSets()
}

func testVolumeClaim(name string) corev1.PersistentVolumeClaim {
	storageClass := "mysource"
	claim := corev1.PersistentVolumeClaim{Spec: corev1.PersistentVolumeClaimSpec{
		StorageClassName: &storageClass,
	}}
	claim.Name = name
	return claim
}
