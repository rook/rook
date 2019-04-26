/*
Copyright 2018 The Rook Authors. All rights reserved.

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
package nfs

import (
	"fmt"
	"strings"
	"testing"

	nfsv1alpha1 "github.com/rook/rook/pkg/apis/nfs.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const appName = "nfs-server-X"

func TestValidateNFSServerSpec(t *testing.T) {

	// first, test that a good NFSServerSpec is good
	spec := nfsv1alpha1.NFSServerSpec{
		Replicas: 1,
		Exports: []nfsv1alpha1.ExportsSpec{
			{
				Name: "test",
				Server: nfsv1alpha1.ServerSpec{
					AccessMode: "readwrite",
					Squash:     "none",
				},
			},
		},
	}

	err := validateNFSServerSpec(spec)
	assert.Nil(t, err)

	// test that AccessMode is invalid
	spec = nfsv1alpha1.NFSServerSpec{
		Replicas: 1,
		Exports: []nfsv1alpha1.ExportsSpec{
			{
				Name: "test",
				Server: nfsv1alpha1.ServerSpec{
					AccessMode: "badValue",
					Squash:     "none",
				},
			},
		},
	}

	err = validateNFSServerSpec(spec)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid value (badValue) for accessMode"))

	// test that Squash is invalid
	spec = nfsv1alpha1.NFSServerSpec{
		Replicas: 1,
		Exports: []nfsv1alpha1.ExportsSpec{
			{
				Name: "test",
				Server: nfsv1alpha1.ServerSpec{
					AccessMode: "ReadWrite",
					Squash:     "badValue",
				},
			},
		},
	}

	err = validateNFSServerSpec(spec)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid value (badValue) for squash"))
}

func TestOnAdd(t *testing.T) {
	namespace := "rook-nfs-test"
	nfsserver := &nfsv1alpha1.NFSServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
		},
		Spec: nfsv1alpha1.NFSServerSpec{
			Replicas: 1,
			Exports: []nfsv1alpha1.ExportsSpec{
				{
					Name: "export-test",
					Server: nfsv1alpha1.ServerSpec{
						AccessMode: "ReadWrite",
						Squash:     "none",
					},
					PersistentVolumeClaim: v1.PersistentVolumeClaimVolumeSource{
						ClaimName: "test-claim",
					},
				},
			},
		},
	}

	// initialize the controller and its dependencies
	clientset := testop.New(3)
	context := &clusterd.Context{Clientset: clientset}
	controller := NewController(context, "rook/nfs:mockTag")

	// in a background thread, simulate the pods running (fake statefulsets don't automatically do that)
	go simulatePodsRunning(clientset, namespace, nfsserver.Spec.Replicas)

	// call onAdd given the specified nfsserver
	controller.onAdd(nfsserver)

	// verify client service
	clientService, err := clientset.CoreV1().Services(namespace).Get(nfsserver.GetName(), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, clientService)
	assert.Equal(t, v1.ServiceTypeClusterIP, clientService.Spec.Type)

	// verify nfs-ganesha config in the configmap
	configMap, err := clientset.CoreV1().ConfigMaps(namespace).Get(nfsserver.GetName(), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, configMap)
	nfsGaneshaConfig := `
EXPORT {
	Export_Id = 10;
	Path = /test-claim;
	Pseudo = /test-claim;
	Protocols = 4;
	Transports = TCP;
	Sectype = sys;
	Access_Type = RW;
	Squash = none;
	FSAL {
		Name = VFS;
	}
}
NFS_Core_Param
{
	fsid_device = true;
}`
	assert.Equal(t, nfsGaneshaConfig, configMap.Data[nfsserver.GetName()])

	// verify stateful set
	ss, err := clientset.AppsV1().StatefulSets(namespace).Get(appName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, ss)
	assert.Equal(t, int32(1), *ss.Spec.Replicas)
	assert.Equal(t, 1, len(ss.Spec.Template.Spec.Containers))

	container := ss.Spec.Template.Spec.Containers[0]
	assert.Equal(t, 2, len(container.VolumeMounts))

	expectedVolumeMounts := []v1.VolumeMount{{Name: "export-test", MountPath: "/test-claim"}, {Name: nfsserver.GetName(), MountPath: "/nfs-ganesha/config"}}
	assert.Equal(t, expectedVolumeMounts, container.VolumeMounts)
}

func simulatePodsRunning(clientset *fake.Clientset, namespace string, podCount int) {
	for i := 0; i < podCount; i++ {
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod%d", i),
				Namespace: namespace,
				Labels:    map[string]string{k8sutil.AppAttr: appName},
			},
			Status: v1.PodStatus{Phase: v1.PodRunning},
		}
		clientset.CoreV1().Pods(namespace).Create(pod)
	}
}
