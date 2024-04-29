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
package osd

import (
	"context"
	"fmt"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	testexec "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	k8stesting "k8s.io/client-go/testing"
)

func TestPrepareDeviceSets(t *testing.T) {
	testPrepareDeviceSets(t, true)
	testPrepareDeviceSets(t, false)
}

func testPrepareDeviceSets(t *testing.T, setTemplateName bool) {
	ctx := context.TODO()
	clientset := testexec.New(t, 1)
	context := &clusterd.Context{
		Clientset: clientset,
	}
	claim := testVolumeClaim("")
	if setTemplateName {
		claim.Name = "randomname"
	}
	deviceSet := cephv1.StorageClassDeviceSet{
		Name:                 "mydata",
		Count:                1,
		Portable:             true,
		VolumeClaimTemplates: []cephv1.VolumeClaimTemplate{claim},
		SchedulerName:        "custom-scheduler",
	}
	spec := cephv1.ClusterSpec{
		Storage: cephv1.StorageScopeSpec{StorageClassDeviceSets: []cephv1.StorageClassDeviceSet{deviceSet}},
	}
	cluster := &Cluster{
		context:     context,
		clusterInfo: client.AdminTestClusterInfo("testns"),
		spec:        spec,
	}

	errs := newProvisionErrors()
	cluster.prepareStorageClassDeviceSets(errs)
	assert.Equal(t, 1, len(cluster.deviceSets))
	assert.Equal(t, 0, errs.len())
	assert.Equal(t, "mydata", cluster.deviceSets[0].Name)
	assert.True(t, cluster.deviceSets[0].Portable)
	_, dataOK := cluster.deviceSets[0].PVCSources["data"]
	assert.True(t, dataOK)
	assert.Equal(t, "custom-scheduler", cluster.deviceSets[0].SchedulerName)

	// Verify that the PVC has the expected generated name with the default of "data" in the name
	pvcs, err := clientset.CoreV1().PersistentVolumeClaims(cluster.clusterInfo.Namespace).List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(pvcs.Items))
	expectedName := claim.Name
	if !setTemplateName {
		expectedName = "data"
	}
	assert.Equal(t, fmt.Sprintf("mydata-%s-0", expectedName), pvcs.Items[0].GenerateName)
	assert.Equal(t, cluster.clusterInfo.Namespace, pvcs.Items[0].Namespace)

	//Verify that the PVC has correct Image Version Label
	cephImageVersion := createValidImageVersionLabel(cluster.spec.CephVersion.Image)
	for _, item := range pvcs.Items {
		val, exist := item.Labels[CephImageLabelKey]
		assert.Equal(t, true, exist)
		assert.Equal(t, cephImageVersion, val)
	}
}

func TestPrepareDeviceSetWithHolesInPVCs(t *testing.T) {
	ctx := context.TODO()
	clientset := testexec.New(t, 1)
	context := &clusterd.Context{
		Clientset: clientset,
	}

	deviceSet := cephv1.StorageClassDeviceSet{
		Name:                 "mydata",
		Count:                1,
		Portable:             true,
		VolumeClaimTemplates: []cephv1.VolumeClaimTemplate{testVolumeClaim("data"), testVolumeClaim("metadata"), testVolumeClaim("wal")},
		SchedulerName:        "custom-scheduler",
	}
	spec := cephv1.ClusterSpec{
		Storage: cephv1.StorageScopeSpec{StorageClassDeviceSets: []cephv1.StorageClassDeviceSet{deviceSet}},
	}
	ns := "testns"
	cluster := &Cluster{
		context:     context,
		clusterInfo: client.AdminTestClusterInfo(ns),
		spec:        spec,
	}

	pvcSuffix := 0
	var pvcReactor k8stesting.ReactionFunc = func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		// PVCs are created with generateName used, and we need to capture the create calls and
		// generate a name for them in order for PVCs to all have unique names.
		createAction, ok := action.(k8stesting.CreateAction)
		if !ok {
			t.Fatal("err! action is not a create action")
			return false, nil, nil
		}
		obj := createAction.GetObject()
		pvc, ok := obj.(*corev1.PersistentVolumeClaim)
		if !ok {
			t.Fatal("err! action not a PVC")
			return false, nil, nil
		}
		if pvc.Name == "" {
			pvc.Name = fmt.Sprintf("%s-%d", pvc.GenerateName, pvcSuffix)
			logger.Info("generated name for PVC:", pvc.Name)
			pvcSuffix++
		} else {
			logger.Info("PVC already has a name:", pvc.Name)
		}
		// setting pvc.Name above modifies the action in-place before future reactors occur
		// we want the default reactor to create the resource, so return false as if we did nothing
		return false, nil, nil
	}
	clientset.PrependReactor("create", "persistentvolumeclaims", pvcReactor)

	// Create 3 PVCs for two OSDs in the device set
	config := newProvisionErrors()
	cluster.prepareStorageClassDeviceSets(config)
	assert.Equal(t, 1, len(cluster.deviceSets))
	assert.Equal(t, 0, config.len())
	assert.Equal(t, "mydata", cluster.deviceSets[0].Name)
	assert.True(t, cluster.deviceSets[0].Portable)
	_, dataOK := cluster.deviceSets[0].PVCSources["data"]
	assert.True(t, dataOK)

	// Verify the PVCs all exist
	pvcs, err := clientset.CoreV1().PersistentVolumeClaims(cluster.clusterInfo.Namespace).List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 3, len(pvcs.Items))
	assertPVCExists(t, clientset, ns, "mydata-data-0-0")
	assertPVCExists(t, clientset, ns, "mydata-metadata-0-1")
	assertPVCExists(t, clientset, ns, "mydata-wal-0-2")

	// Create 3 more PVCs (6 total) for two OSDs in the device set
	cluster.spec.Storage.StorageClassDeviceSets[0].Count = 2
	cluster.prepareStorageClassDeviceSets(config)
	assert.Equal(t, 2, len(cluster.deviceSets))
	assert.Equal(t, 0, config.len())

	// Verify the PVCs all exist
	pvcs, err = clientset.CoreV1().PersistentVolumeClaims(cluster.clusterInfo.Namespace).List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 6, len(pvcs.Items))
	assertPVCExists(t, clientset, ns, "mydata-data-0-0")
	assertPVCExists(t, clientset, ns, "mydata-metadata-0-1")
	assertPVCExists(t, clientset, ns, "mydata-wal-0-2")
	assertPVCExists(t, clientset, ns, "mydata-data-1-3")
	assertPVCExists(t, clientset, ns, "mydata-metadata-1-4")
	assertPVCExists(t, clientset, ns, "mydata-wal-1-5")

	// Verify the same number of PVCs exist after calling the reconcile again on the PVCs
	cluster.prepareStorageClassDeviceSets(config)
	assert.Equal(t, 2, len(cluster.deviceSets))
	pvcs, err = clientset.CoreV1().PersistentVolumeClaims(cluster.clusterInfo.Namespace).List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 6, len(pvcs.Items))

	// Delete a single PVC and verify it will be re-created
	err = clientset.CoreV1().PersistentVolumeClaims(cluster.clusterInfo.Namespace).Delete(ctx, "mydata-wal-0-2", metav1.DeleteOptions{})
	assert.NoError(t, err)
	cluster.prepareStorageClassDeviceSets(config)
	assert.Equal(t, 2, len(cluster.deviceSets))
	pvcs, err = clientset.CoreV1().PersistentVolumeClaims(cluster.clusterInfo.Namespace).List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 6, len(pvcs.Items))

	// Delete the PVCs for an OSD and verify it will not be re-created if the count is reduced
	cluster.spec.Storage.StorageClassDeviceSets[0].Count = 1
	err = clientset.CoreV1().PersistentVolumeClaims(cluster.clusterInfo.Namespace).Delete(ctx, "mydata-data-0-0", metav1.DeleteOptions{})
	assert.NoError(t, err)
	err = clientset.CoreV1().PersistentVolumeClaims(cluster.clusterInfo.Namespace).Delete(ctx, "mydata-metadata-0-1", metav1.DeleteOptions{})
	assert.NoError(t, err)
	err = clientset.CoreV1().PersistentVolumeClaims(cluster.clusterInfo.Namespace).Delete(ctx, "mydata-wal-0-6", metav1.DeleteOptions{})
	assert.NoError(t, err)
	cluster.prepareStorageClassDeviceSets(config)
	assert.Equal(t, 1, len(cluster.deviceSets))
	pvcs, err = clientset.CoreV1().PersistentVolumeClaims(cluster.clusterInfo.Namespace).List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 3, len(pvcs.Items))

	// Scale back up to a count of two and confirm that a new index is used for the PVCs
	cluster.spec.Storage.StorageClassDeviceSets[0].Count = 2
	cluster.prepareStorageClassDeviceSets(config)
	assert.Equal(t, 2, len(cluster.deviceSets))
	pvcs, err = clientset.CoreV1().PersistentVolumeClaims(cluster.clusterInfo.Namespace).List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 6, len(pvcs.Items))
	assertPVCExists(t, clientset, ns, "mydata-data-1-3")
	assertPVCExists(t, clientset, ns, "mydata-metadata-1-4")
	assertPVCExists(t, clientset, ns, "mydata-wal-1-5")
	assertPVCExists(t, clientset, ns, "mydata-data-2-7")
	assertPVCExists(t, clientset, ns, "mydata-metadata-2-8")
	assertPVCExists(t, clientset, ns, "mydata-wal-2-9")
}

func assertPVCExists(t *testing.T, clientset kubernetes.Interface, namespace, name string) {
	pvc, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, pvc)
}

func testVolumeClaim(name string) cephv1.VolumeClaimTemplate {
	storageClass := "mysource"
	claim := cephv1.VolumeClaimTemplate{Spec: corev1.PersistentVolumeClaimSpec{
		StorageClassName: &storageClass,
	}}
	claim.Name = name
	return claim
}

func TestPrepareDeviceSetsWithCrushParams(t *testing.T) {
	ctx := context.TODO()
	clientset := testexec.New(t, 1)
	context := &clusterd.Context{
		Clientset: clientset,
	}
	deviceSet := cephv1.StorageClassDeviceSet{
		Name:                 "datawithcrushparams1",
		Count:                1,
		VolumeClaimTemplates: []cephv1.VolumeClaimTemplate{testVolumeClaim("testwithcrushparams1")},
		SchedulerName:        "custom-scheduler",
	}
	deviceSet.VolumeClaimTemplates[0].Annotations = map[string]string{
		"crushDeviceClass":     "ssd",
		"crushInitialWeight":   "0.75",
		"crushPrimaryAffinity": "0.666",
	}

	spec := cephv1.ClusterSpec{
		Storage: cephv1.StorageScopeSpec{StorageClassDeviceSets: []cephv1.StorageClassDeviceSet{deviceSet}},
	}
	cluster := &Cluster{
		context:     context,
		clusterInfo: client.AdminTestClusterInfo("testns"),
		spec:        spec,
	}

	config := newProvisionErrors()
	cluster.prepareStorageClassDeviceSets(config)
	assert.Equal(t, 1, len(cluster.deviceSets))
	assert.Equal(t, cluster.deviceSets[0].CrushDeviceClass, "ssd")
	assert.Equal(t, cluster.deviceSets[0].CrushInitialWeight, "0.75")
	assert.Equal(t, cluster.deviceSets[0].CrushPrimaryAffinity, "0.666")

	pvcs, err := clientset.CoreV1().PersistentVolumeClaims(cluster.clusterInfo.Namespace).List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(pvcs.Items))
}

func TestPVCName(t *testing.T) {
	id, err := deviceSetPVCID("mydeviceset-making-the-length-of-pvc-id-greater-than-the-limit-63", "a", 0)
	assert.Error(t, err)
	assert.Equal(t, "", id)

	id, err = deviceSetPVCID("mydeviceset", "a", 10)
	assert.NoError(t, err)
	assert.Equal(t, "mydeviceset-a-10", id)

	id, err = deviceSetPVCID("device-set", "a", 10)
	assert.NoError(t, err)
	assert.Equal(t, "device-set-a-10", id)

	id, err = deviceSetPVCID("device.set.with.dots", "b", 10)
	assert.NoError(t, err)
	assert.Equal(t, "device-set-with-dots-b-10", id)
}

func TestCreateValidImageVersionLabel(t *testing.T) {
	image := "ceph/ceph:v18.2.2"
	assert.Equal(t, "ceph_ceph_v18.2.2", createValidImageVersionLabel(image))
	image = "rook/ceph:master"
	assert.Equal(t, "rook_ceph_master", createValidImageVersionLabel(image))
	image = ".invalid_label"
	assert.Equal(t, "", createValidImageVersionLabel(image))
}
