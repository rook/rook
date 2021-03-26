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
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestOnDeviceCMUpdate(t *testing.T) {
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	service := &corev1.Service{}
	ns := "rook-ceph"
	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{})

	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ns,
			Namespace: ns,
		},
		Status: cephv1.ClusterStatus{
			Phase: "",
		},
	}

	object := []runtime.Object{
		cephCluster,
	}

	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	clientCluster := newClientCluster(cl, ns, &clusterd.Context{})

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
	cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	clientCluster.client = cl
	b = clientCluster.onDeviceCMUpdate(oldCM, newCM)
	assert.True(t, b)
}
