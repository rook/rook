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

package cluster

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/coreos/pkg/capnslog"
	addonsv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/apis/csiaddons/v1alpha1"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apifake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sFake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func getFakeClient(obj ...runtime.Object) client.Client {
	// Register operator types with the runtime scheme.
	scheme := scheme.Scheme
	scheme.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{}, &addonsv1alpha1.NetworkFence{}, &addonsv1alpha1.NetworkFenceList{})
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(obj...).Build()
	return client
}

func fakeCluster(ns string) *cephv1.CephCluster {
	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ns,
			Namespace: ns,
		},
		Status: cephv1.ClusterStatus{
			Phase: "",
		},
		Spec: cephv1.ClusterSpec{
			Storage: cephv1.StorageScopeSpec{},
		},
	}
	return cephCluster
}

func TestCheckStorageForNode(t *testing.T) {
	ns := "rook-ceph"
	cephCluster := fakeCluster(ns)

	assert.False(t, checkStorageForNode(cephCluster))

	cephCluster.Spec.Storage.UseAllNodes = true
	assert.True(t, checkStorageForNode(cephCluster))

	fakeNodes := []cephv1.Node{
		{
			Name: "nodeA",
		},
	}
	cephCluster.Spec.Storage.Nodes = fakeNodes
	assert.True(t, checkStorageForNode(cephCluster))

	fakeDeviceSets := []cephv1.StorageClassDeviceSet{
		{
			Name: "DeviceSet1",
		},
	}
	cephCluster.Spec.Storage.StorageClassDeviceSets = fakeDeviceSets
	assert.True(t, checkStorageForNode(cephCluster))
}

func TestOnK8sNode(t *testing.T) {
	ns := "rook-ceph"
	opns := "operator"
	ctx := context.TODO()
	cephCluster := fakeCluster(ns)
	objects := []runtime.Object{
		cephCluster,
	}

	// Create a fake client to mock API calls.
	client := getFakeClient(objects...)

	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		return "", errors.New("failed to get osd list on host")
	}
	clientCluster := newClientCluster(client, ns, &clusterd.Context{
		Executor:            executor,
		Clientset:           k8sFake.NewSimpleClientset(),
		ApiExtensionsClient: apifake.NewSimpleClientset(),
	})

	node := &corev1.Node{
		Spec: corev1.NodeSpec{
			Unschedulable: false,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Status: corev1.ConditionStatus(corev1.ConditionTrue),
					Type:   corev1.NodeConditionType(corev1.NodeReady),
				},
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "fakenode",
		},
	}

	// node will reconcile
	fakeNodes := []cephv1.Node{
		{
			Name: "nodeA",
		},
	}
	fakeDeviceSets := []cephv1.StorageClassDeviceSet{
		{
			Name: "DeviceSet1",
		},
	}
	cephCluster.Spec.Storage.Nodes = fakeNodes
	cephCluster.Spec.Storage.StorageClassDeviceSets = fakeDeviceSets
	cephCluster.Spec.Storage.UseAllNodes = true
	cephCluster.Status.Phase = k8sutil.ReadyStatus
	client = getFakeClient(objects...)
	clientCluster.client = client
	b := clientCluster.onK8sNode(ctx, node, opns)
	assert.True(t, b)

	// node will not reconcile
	b = clientCluster.onK8sNode(ctx, node, opns)
	assert.False(t, b)
}

func TestHandleNodeFailure(t *testing.T) {
	clusterns := "rook-ceph"
	opns := "operator"
	ctx := context.TODO()
	cephCluster := fakeCluster(clusterns)
	objects := []runtime.Object{
		cephCluster,
	}
	executor := &exectest.MockExecutor{}
	client := getFakeClient(objects...)
	c := newClientCluster(client, clusterns, &clusterd.Context{
		Executor:            executor,
		Clientset:           k8sFake.NewSimpleClientset(),
		ApiExtensionsClient: apifake.NewSimpleClientset(),
	})

	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		switch {
		case command == "rbd" && args[0] == "status":
			return `{"watchers":[{"address":"192.168.39.137:0/3762982934","client":4307,"cookie":18446462598732840961}]}`, nil
		case command == "ceph" && args[0] == "status":
			return `{"entity":[{"addr": [{"addr": "10.244.0.12:0", "nonce":3247243972}]}], "client_metadata":{"root":"/"}}`, nil
		case command == "ceph" && args[0] == "tell":
			return `[{"entity":{"addr":{"addr":"10.244.0.12:0","nonce":3247243972}}, "client_metadata":{"root":"/volumes/csi/csi-vol-58469d41-f6c0-4720-b23a-0a0826b842ca","hostname":"fakenode"}}]`, nil

		}
		return "", errors.Errorf("unexpected rbd/ceph command %q", args)
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fakenode",
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{
					Key:   corev1.TaintNodeOutOfService,
					Value: string(corev1.TaintEffectNoSchedule),
				},
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Status: corev1.ConditionStatus(corev1.ConditionTrue),
					Type:   corev1.NodeConditionType(corev1.NodeReady),
				},
			},
			VolumesInUse: []corev1.UniqueVolumeName{
				"kubernetes.io/csi/operator.rbd.csi.ceph.com^0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4002",
				"kubernetes.io/csi/operator.cephfs.csi.ceph.com^0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4001",
			},
		},
	}

	rbdPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pvc-58469d41-f6c0-4720-b23a-0a0826b841ca",
			Annotations: map[string]string{
				"pv.kubernetes.io/provisioned-by":                            fmt.Sprintf("%s.rbd.csi.ceph.com", opns),
				"volume.kubernetes.io/provisioner-deletion-secret-name":      "rook-csi-rbd-provisioner",
				"volume.kubernetes.io/provisioner-deletion-secret-namespace": clusterns,
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       fmt.Sprintf("%s.rbd.csi.ceph.com", opns),
					VolumeHandle: "0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4002",
					VolumeAttributes: map[string]string{
						"pool":      "replicapool",
						"imageName": "csi-vol-58469d41-f6c0-4720-b23a-0a0826b841ca",
					},
				},
			},
		},
	}

	cephfsPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pvc-58469d41-f6c0-4720-b23a-0a0826b842ca",
			Annotations: map[string]string{
				"pv.kubernetes.io/provisioned-by":                            fmt.Sprintf("%s.cephfs.csi.ceph.com", opns),
				"volume.kubernetes.io/provisioner-deletion-secret-name":      "rook-csi-cephfs-provisioner",
				"volume.kubernetes.io/provisioner-deletion-secret-namespace": clusterns,
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       fmt.Sprintf("%s.cephfs.csi.ceph.com", opns),
					VolumeHandle: "0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4001",
					VolumeAttributes: map[string]string{
						"fsName":        "myfs",
						"subvolumePath": "/volumes/csi/csi-vol-58469d41-f6c0-4720-b23a-0a0826b842ca",
						"subvolumeName": "csi-vol-58469d41-f6c0-4720-b23a-0a0826b842ca",
					},
				},
			},
		},
	}

	staticRbdPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pvc-58469d41-f6c0-4720-b23a-0a0826b841cb",
			Annotations: map[string]string{
				"pv.kubernetes.io/provisioned-by":                            fmt.Sprintf("%s.rbd.csi.ceph.com", opns),
				"volume.kubernetes.io/provisioner-deletion-secret-name":      "rook-csi-rbd-provisioner",
				"volume.kubernetes.io/provisioner-deletion-secret-namespace": clusterns,
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:           fmt.Sprintf("%s.rbd.csi.ceph.com", opns),
					VolumeHandle:     "0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4002",
					VolumeAttributes: map[string]string{},
				},
			},
		},
	}

	staticCephfsPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pvc-58469d41-f6c0-4720-b23a-0a0826b842cb",
			Annotations: map[string]string{
				"pv.kubernetes.io/provisioned-by":                            fmt.Sprintf("%s.cephfs.csi.ceph.com", opns),
				"volume.kubernetes.io/provisioner-deletion-secret-name":      "rook-csi-cephfs-provisioner",
				"volume.kubernetes.io/provisioner-deletion-secret-namespace": clusterns,
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       fmt.Sprintf("%s.cephfs.csi.ceph.com", opns),
					VolumeHandle: "0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4001",
					VolumeAttributes: map[string]string{
						"staticVolume": "true",
					},
				},
			},
		},
	}

	pvNotProvisionByCSI := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pvc-58469d41-f6c0-4720-b23a-0a0826b841cc",
			Annotations: map[string]string{
				"pv.kubernetes.io/provisioned-by":                            fmt.Sprintf("%s.csi.rbd.com", opns),
				"volume.kubernetes.io/provisioner-deletion-secret-name":      "csi-rbd-provisioner",
				"volume.kubernetes.io/provisioner-deletion-secret-namespace": clusterns,
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       fmt.Sprintf("%s.rbd.csi.com", opns),
					VolumeHandle: "0001-0009-csi-0000000000000002-24862838-240d-4215-9183-abfc0e9e4002",
					VolumeAttributes: map[string]string{
						"pool":      "replicapool",
						"imageName": "csi-vol-58469d41-f6c0-4720-b23a-0a0826b841ca",
					},
				},
			},
		},
	}

	// Mock clusterInfo
	secrets := map[string][]byte{
		"fsid":         []byte("c47cac40-9bee-4d52-823b-ccd803ba5bfe"),
		"mon-secret":   []byte("monsecret"),
		"admin-secret": []byte("adminsecret"),
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-ceph-mon",
			Namespace: clusterns,
		},
		Data: secrets,
		Type: k8sutil.RookType,
	}
	_, err := c.context.Clientset.CoreV1().Secrets(clusterns).Create(ctx, secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	_, err = c.context.Clientset.CoreV1().PersistentVolumes().Create(ctx, rbdPV, metav1.CreateOptions{})
	assert.NoError(t, err)

	_, err = c.context.Clientset.CoreV1().PersistentVolumes().Create(ctx, cephfsPV, metav1.CreateOptions{})
	assert.NoError(t, err)

	_, err = c.context.ApiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, &v1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "networkfences.csiaddons.openshift.io"}}, metav1.CreateOptions{})
	assert.NoError(t, err)

	// When out-of-service taint is added
	err = c.handleNodeFailure(ctx, cephCluster, node, opns)
	assert.NoError(t, err)

	networkFenceRbd := &addonsv1alpha1.NetworkFence{}
	err = c.client.Get(ctx, types.NamespacedName{Name: fenceResourceName(node.Name, rbdDriver, clusterns)}, networkFenceRbd)
	assert.NoError(t, err)

	networkFenceCephFs := &addonsv1alpha1.NetworkFence{}
	err = c.client.Get(ctx, types.NamespacedName{Name: fenceResourceName(node.Name, cephfsDriver, clusterns)}, networkFenceCephFs)
	assert.NoError(t, err)

	networkFences := &addonsv1alpha1.NetworkFenceList{}
	err = c.client.List(ctx, networkFences)
	assert.NoError(t, err)
	var rbdCount, cephFsCount int

	for _, fence := range networkFences.Items {
		// Check if the resource is in the desired namespace
		if strings.Contains(fence.Name, rbdDriver) {
			rbdCount++
		} else if strings.Contains(fence.Name, cephfsDriver) {
			cephFsCount++
		}
	}

	assert.Equal(t, 1, rbdCount)
	assert.Equal(t, 1, cephFsCount)

	// For static rbd pv
	_, err = c.context.Clientset.CoreV1().PersistentVolumes().Create(ctx, staticRbdPV, metav1.CreateOptions{})
	assert.NoError(t, err)

	pvList, err := c.context.Clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)

	rbdVolumesInUse, _ := getCephVolumesInUse(cephCluster, node.Status.VolumesInUse, opns)
	rbdPVList := listRBDPV(pvList, cephCluster, rbdVolumesInUse, opns)
	assert.Equal(t, len(rbdPVList), 1) // it will be equal to one since we have one pv provisioned by csi named `rbdPV`

	err = c.handleNodeFailure(ctx, cephCluster, node, opns)
	assert.NoError(t, err)

	// For static cephfs pv
	_, err = c.context.Clientset.CoreV1().PersistentVolumes().Create(ctx, staticCephfsPV, metav1.CreateOptions{})
	assert.NoError(t, err)

	pvList, err = c.context.Clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)

	_, cephFSVolumesInUse := getCephVolumesInUse(cephCluster, node.Status.VolumesInUse, opns)
	cephFSVolumesInUseMap := make(map[string]struct{})
	for _, vol := range cephFSVolumesInUse {
		cephFSVolumesInUseMap[vol] = struct{}{}
	}
	cephFSPVList := listRWOCephFSPV(pvList, cephCluster, cephFSVolumesInUseMap, opns)
	assert.Equal(t, len(cephFSPVList), 1) // it will be equal to one since we have one pv provisioned by csi named `cephfsPV`

	err = c.handleNodeFailure(ctx, cephCluster, node, opns)
	assert.NoError(t, err)

	// For pv not provisioned by CSI
	_, err = c.context.Clientset.CoreV1().PersistentVolumes().Create(ctx, pvNotProvisionByCSI, metav1.CreateOptions{})
	assert.NoError(t, err)

	pvList, err = c.context.Clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)

	rbdVolumesInUse, _ = getCephVolumesInUse(cephCluster, node.Status.VolumesInUse, opns)
	rbdPVList = listRBDPV(pvList, cephCluster, rbdVolumesInUse, opns)
	assert.Equal(t, len(rbdPVList), 1) // it will be equal to one since we have one pv provisioned by csi named `PV`

	err = c.handleNodeFailure(ctx, cephCluster, node, opns)
	assert.NoError(t, err)

	// When out-of-service taint is removed
	node.Spec.Taints = []corev1.Taint{}
	networkFenceRbd.Status.Message = addonsv1alpha1.UnFenceOperationSuccessfulMessage
	err = c.client.Update(ctx, networkFenceRbd)
	assert.NoError(t, err)

	networkFenceCephFs.Status.Message = addonsv1alpha1.UnFenceOperationSuccessfulMessage
	err = c.client.Update(ctx, networkFenceCephFs)
	assert.NoError(t, err)

	err = c.handleNodeFailure(ctx, cephCluster, node, opns)
	assert.NoError(t, err)

	err = c.client.Get(ctx, types.NamespacedName{Name: fenceResourceName(node.Name, rbdDriver, clusterns), Namespace: cephCluster.Namespace}, networkFenceRbd)
	assert.Error(t, err, kerrors.IsNotFound(err))

	err = c.client.Get(ctx, types.NamespacedName{Name: fenceResourceName(node.Name, cephfsDriver, clusterns), Namespace: cephCluster.Namespace}, networkFenceCephFs)
	assert.Error(t, err, kerrors.IsNotFound(err))

}

func TestGetCephVolumesInUse(t *testing.T) {
	cephCluster := fakeCluster("rook-ceph")
	volInUse := []corev1.UniqueVolumeName{
		"kubernetes.io/csi/rook-ceph.rbd.csi.ceph.com^0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4002",
		"kubernetes.io/csi/rook-ceph.rbd.csi.ceph.com^0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4003",
		"kubernetes.io/csi/rook-ceph.cephfs.csi.ceph.com^0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4001",
		"kubernetes.io/csi/rook-ceph.cephfs.csi.ceph.com^0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4004",
	}

	splitVolInUse := trimeVolumeInUse(volInUse[0])
	assert.Equal(t, splitVolInUse[0], "rook-ceph.rbd.csi.ceph.com")
	assert.Equal(t, splitVolInUse[1], "0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4002")

	splitVolInUse = trimeVolumeInUse(volInUse[1])
	assert.Equal(t, splitVolInUse[0], "rook-ceph.rbd.csi.ceph.com")
	assert.Equal(t, splitVolInUse[1], "0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4003")

	splitVolInUse = trimeVolumeInUse(volInUse[2])
	assert.Equal(t, splitVolInUse[0], "rook-ceph.cephfs.csi.ceph.com")
	assert.Equal(t, splitVolInUse[1], "0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4001")

	splitVolInUse = trimeVolumeInUse(volInUse[3])
	assert.Equal(t, splitVolInUse[0], "rook-ceph.cephfs.csi.ceph.com")
	assert.Equal(t, splitVolInUse[1], "0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4004")

	trimRbdVolInUse, trimCephFSVolInUse := getCephVolumesInUse(cephCluster, volInUse, cephCluster.Namespace)

	expectedRbd := []string{"0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4002", "0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4003"}
	expectedCephfs := []string{"0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4001", "0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4004"}

	assert.Equal(t, expectedRbd, trimRbdVolInUse)
	assert.Equal(t, expectedCephfs, trimCephFSVolInUse)
}

func TestRBDStatusUnMarshal(t *testing.T) {
	output := `{"watchers":[{"address":"192.168.39.137:0/3762982934","client":4307,"cookie":18446462598732840961}]}`

	listIP, err := rbdStatusUnMarshal([]byte(output))
	assert.NoError(t, err)
	assert.Equal(t, listIP[0], "192.168.39.137/32")
}

func TestConcatenateWatcherIp(t *testing.T) {
	WatcherIP := concatenateWatcherIp("192.168.39.137:0/3762982934")
	assert.Equal(t, WatcherIP, "192.168.39.137/32")
}

func TestFenceResourceName(t *testing.T) {
	FenceName := fenceResourceName("fakenode", "rbd", "rook-ceph")
	assert.Equal(t, FenceName, "fakenode-rbd-rook-ceph")
}

func TestOnDeviceCMUpdate(t *testing.T) {
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	service := &corev1.Service{}
	ns := "rook-ceph"
	cephCluster := fakeCluster(ns)
	objects := []runtime.Object{
		cephCluster,
	}

	// Create a fake client to mock API calls.
	client := getFakeClient(objects...)
	clientCluster := newClientCluster(client, ns, &clusterd.Context{})

	// Dummy object
	b := clientCluster.onDeviceCMUpdate(service, service)
	assert.False(t, b)

	// No Data in the cm
	oldCM := &corev1.ConfigMap{}
	newCM := &corev1.ConfigMap{}
	b = clientCluster.onDeviceCMUpdate(oldCM, newCM)
	assert.False(t, b)

	devices := []byte(`
	[
		{
		  "name": "dm-0",
		  "parent": ".",
		  "hasChildren": false,
		  "devLinks": "/dev/disk/by-id/dm-name-ceph--bee31cdd--e899--4f9a--9e77--df71cfad66f9-osd--data--b5df7900--0cf0--4b1a--a337--7b57c9f0111b/dev/disk/by-id/dm-uuid-LVM-B10SBHeAy5yF6l2OM3p3EqTQbUAYc6JI63n8ZZPTmxRHXTJHmQ4YTAIBCJqY931Z",
		  "size": 31138512896,
		  "uuid": "aafee853-1b8d-4a15-83a9-17825728befc",
		  "serial": "",
		  "type": "lvm",
		  "rotational": true,
		  "readOnly": false,
		  "Partitions": [
			{
			  "Name": "ceph--bee31cdd--e899--4f9a--9e77--df71cfad66f9-osd--data--b5df7900--0cf0--4b1a--a337--7b57c9f0111b",
			  "Size": 0,
			  "Label": "",
			  "Filesystem": ""
			}
		  ],
		  "filesystem": "ceph_bluestore",
		  "vendor": "",
		  "model": "",
		  "wwn": "",
		  "wwnVendorExtension": "",
		  "empty": false,
		  "real-path": "/dev/mapper/ceph--bee31cdd--e899--4f9a--9e77--df71cfad66f9-osd--data--b5df7900--0cf0--4b1a--a337--7b57c9f0111b"
		}
	  ]`)

	oldData := make(map[string]string, 1)
	oldData["devices"] = "[{}]"
	oldCM.Data = oldData

	newData := make(map[string]string, 1)
	newData["devices"] = string(devices)
	newCM.Data = newData

	// now there is a diff but cluster is not ready
	b = clientCluster.onDeviceCMUpdate(oldCM, newCM)
	assert.False(t, b)

	// finally the cluster is ready and we can reconcile
	// Add ready status to the CephCluster
	cephCluster.Status.Phase = k8sutil.ReadyStatus
	client = getFakeClient(objects...)
	clientCluster.client = client
	b = clientCluster.onDeviceCMUpdate(oldCM, newCM)
	assert.True(t, b)
}

func Test_pvSupportMultiNodeAccess(t *testing.T) {
	type args struct {
		accessModes []corev1.PersistentVolumeAccessMode
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "single access mode, contains RWX",
			args: args{
				accessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteMany,
				},
			},
			want: true,
		},
		{
			name: "single access mode, contains ROX",
			args: args{
				accessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadOnlyMany,
				},
			},
			want: true,
		},
		{
			name: "single access mode, contains only RWO",
			args: args{
				accessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
			},
			want: false,
		},
		{
			name: "single access mode, contains only RWOP",
			args: args{
				accessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOncePod,
				},
			},
			want: false,
		},
		{
			name: "multiple access mode, contains RWX",
			args: args{
				accessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
					corev1.ReadWriteMany,
				},
			},
			want: true,
		},
		{
			name: "multiple access mode, contains RWX and ROX",
			args: args{
				accessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteMany,
					corev1.ReadOnlyMany,
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pvSupportsMultiNodeAccess(tt.args.accessModes)
			assert.Equal(t, tt.want, got)
		})
	}
}
