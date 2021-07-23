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
	"os"
	"testing"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func getFakeClient(obj ...runtime.Object) client.Client {
	// Register operator types with the runtime scheme.
	scheme := scheme.Scheme
	scheme.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{})
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
		Executor: executor,
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
	b := clientCluster.onK8sNode(node)
	assert.True(t, b)

	// node will not reconcile
	b = clientCluster.onK8sNode(node)
	assert.False(t, b)
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
