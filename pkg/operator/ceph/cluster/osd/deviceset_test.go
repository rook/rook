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
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	testexec "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	storageClass := "mysource"
	claim := v1.PersistentVolumeClaim{Spec: v1.PersistentVolumeClaimSpec{
		StorageClassName: &storageClass,
	}}
	if setTemplateName {
		claim.Name = "randomname"
	}
	deviceSet := rookv1.StorageClassDeviceSet{
		Name:                 "mydata",
		Count:                1,
		Portable:             true,
		VolumeClaimTemplates: []v1.PersistentVolumeClaim{claim},
		SchedulerName:        "custom-scheduler",
	}
	spec := cephv1.ClusterSpec{
		Storage: rookv1.StorageScopeSpec{StorageClassDeviceSets: []rookv1.StorageClassDeviceSet{deviceSet}},
	}
	cluster := &Cluster{
		context:     context,
		clusterInfo: client.AdminClusterInfo("testns"),
		spec:        spec,
	}

	config := &provisionConfig{}
	volumeSources := cluster.prepareStorageClassDeviceSets(config)
	assert.Equal(t, 1, len(volumeSources))
	assert.Equal(t, 0, len(config.errorMessages))
	assert.Equal(t, "mydata", volumeSources[0].Name)
	assert.True(t, volumeSources[0].Portable)
	_, dataOK := volumeSources[0].PVCSources["data"]
	assert.True(t, dataOK)
	assert.Equal(t, "custom-scheduler", volumeSources[0].SchedulerName)

	// Verify that the PVC has the expected generated name with the default of "data" in the name
	pvcs, err := clientset.CoreV1().PersistentVolumeClaims(cluster.clusterInfo.Namespace).List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(pvcs.Items))
	expectedName := claim.Name
	if !setTemplateName {
		expectedName = "data"
	}
	assert.Equal(t, fmt.Sprintf("mydata-%s-0-", expectedName), pvcs.Items[0].GenerateName)
	assert.Equal(t, cluster.clusterInfo.Namespace, pvcs.Items[0].Namespace)
}

func TestUpdatePVCSize(t *testing.T) {
	clientset := testexec.New(t, 1)
	context := &clusterd.Context{
		Clientset: clientset,
	}
	cluster := &Cluster{
		context:     context,
		clusterInfo: client.AdminClusterInfo("testns"),
	}
	current := &v1.PersistentVolumeClaim{}
	desired := &v1.PersistentVolumeClaim{}
	current.Spec.Resources.Requests = v1.ResourceList{}
	desired.Spec.Resources.Requests = v1.ResourceList{}
	current.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse("5Gi")

	// Nothing happens if no size is set on the new PVC
	cluster.updatePVCIfChanged(desired, current)
	result, ok := current.Spec.Resources.Requests[v1.ResourceStorage]
	assert.True(t, ok)
	assert.Equal(t, "5Gi", result.String())

	// Nothing happens if the size shrinks
	desired.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse("4Gi")
	cluster.updatePVCIfChanged(desired, current)
	result, ok = current.Spec.Resources.Requests[v1.ResourceStorage]
	assert.True(t, ok)
	assert.Equal(t, "5Gi", result.String())

	// The size is updated when it increases
	desired.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse("6Gi")
	cluster.updatePVCIfChanged(desired, current)
	result, ok = current.Spec.Resources.Requests[v1.ResourceStorage]
	assert.True(t, ok)
	assert.Equal(t, "6Gi", result.String())
}
