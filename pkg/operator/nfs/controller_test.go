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

func TestOnAddComplexServer(t *testing.T) {
	namespace := "rook-nfs-test"
	nfsserver := &nfsv1alpha1.NFSServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nfs-server-X",
			Namespace: namespace,
		},
		Spec: nfsv1alpha1.NFSServerSpec{
			Replicas: 1,
			Exports: []nfsv1alpha1.ExportsSpec{
				{
					Name: "export-test1",
					Server: nfsv1alpha1.ServerSpec{
						Squash: "none",
						AllowedClients: []nfsv1alpha1.AllowedClientsSpec{
							{
								Name:       "client-test-1",
								Clients:    []string{"all"},
								AccessMode: "ReadOnly",
								Squash:     "root",
							},
							{
								Name:       "client-test-2",
								Clients:    []string{"172.17.0.0/16", "serverX"},
								AccessMode: "ReadWrite",
								Squash:     "none",
							},
						},
					},
					PersistentVolumeClaim: v1.PersistentVolumeClaimVolumeSource{
						ClaimName: "test-claim1",
					},
				},
				{
					Name: "export-test2",
					Server: nfsv1alpha1.ServerSpec{
						AccessMode: "ReadOnly",
						Squash:     "none",
					},
					PersistentVolumeClaim: v1.PersistentVolumeClaimVolumeSource{
						ClaimName: "test-claim2",
					},
				},
			},
		},
	}

	nfsGaneshaConfig := `
EXPORT {
	Export_Id = 10;
	Path = /test-claim1;
	Pseudo = /test-claim1;
	Protocols = 4;
	Transports = TCP;
	Sectype = sys;
	Squash = none;
	FSAL {
		Name = VFS;
	}
	CLIENT {
		Clients = *;
		Access_Type = RO;
		Squash = none;
	}
	CLIENT {
		Clients = 172.17.0.0/16, serverX;
		Access_Type = RW;
		Squash = none;
	}
}
EXPORT {
	Export_Id = 11;
	Path = /test-claim2;
	Pseudo = /test-claim2;
	Protocols = 4;
	Transports = TCP;
	Sectype = sys;
	Access_Type = RO;
	Squash = none;
	FSAL {
		Name = VFS;
	}
}
NFS_Core_Param
{
	fsid_device = true;
}`
	volMounts := []v1.VolumeMount{
		{Name: "test-claim1", MountPath: "/test-claim1"},
		{Name: "test-claim2", MountPath: "/test-claim2"},
		{Name: "nfs-ganesha-config", MountPath: "/nfs-ganesha/config"},
	}
	onAdd(t, namespace, nfsserver, nfsGaneshaConfig, volMounts)
}

func TestOnAddSimpleServer(t *testing.T) {
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
	volMounts := []v1.VolumeMount{{Name: "test-claim", MountPath: "/test-claim"}, {Name: "nfs-ganesha-config", MountPath: "/nfs-ganesha/config"}}
	onAdd(t, namespace, nfsserver, nfsGaneshaConfig, volMounts)

}

func onAdd(t *testing.T, namespace string, nfsServer *nfsv1alpha1.NFSServer, expectedConfig string, expectedVolumeMounts []v1.VolumeMount) {

	// initialize the controller and its dependencies
	clientset := testop.New(3)
	context := &clusterd.Context{Clientset: clientset}
	controller := NewController(context, "rook/nfs:mockTag")

	// in a background thread, simulate the pods running (fake statefulsets don't automatically do that)
	go simulatePodsRunning(clientset, namespace, nfsServer.Spec.Replicas)

	// call onAdd given the specified nfsserver
	controller.onAdd(nfsServer)

	// verify client service
	clientService, err := clientset.CoreV1().Services(namespace).Get(nfsserver.GetName(), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, clientService)
	assert.Equal(t, v1.ServiceTypeClusterIP, clientService.Spec.Type)

	// verify nfs-ganesha config in the configmap
	configMap, err := clientset.CoreV1().ConfigMaps(namespace).Get(nfsserver.GetName(), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, configMap)

	assert.Equal(t, expectedConfig, configMap.Data[nfsConfigMapName])

	// verify stateful set
	ss, err := clientset.AppsV1().StatefulSets(namespace).Get(appName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, ss)
	assert.Equal(t, int32(1), *ss.Spec.Replicas)
	assert.Equal(t, 1, len(ss.Spec.Template.Spec.Containers))

	container := ss.Spec.Template.Spec.Containers[0]
	assert.Equal(t, len(expectedVolumeMounts), len(container.VolumeMounts))

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
