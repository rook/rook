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
package minio

import (
	"testing"

	miniov "github.com/rook/rook/pkg/apis/minio.rook.io/v1alpha1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var appName = "some_object_store"

func TestOnAdd(t *testing.T) {
	namespace := "rook-minio-123"
	objectstore := &miniov.ObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
		},
		Spec: miniov.ObjectStoreSpec{
			Storage: rookalpha.StorageScopeSpec{
				NodeCount: 6,
				Selection: rookalpha.Selection{
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "rook-minio-test1",
							},
							Spec: v1.PersistentVolumeClaimSpec{
								AccessModes: []v1.PersistentVolumeAccessMode{
									v1.ReadWriteOnce,
								},
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceStorage: resource.MustParse("10Gi"),
									},
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "rook-minio-test2",
							},
							Spec: v1.PersistentVolumeClaimSpec{
								AccessModes: []v1.PersistentVolumeAccessMode{
									v1.ReadWriteOnce,
								},
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceStorage: resource.MustParse("10Gi"),
									},
								},
							},
						},
					},
				},
			},
			Credentials: v1.SecretReference{
				Name:      "whatever",
				Namespace: namespace,
			},
		},
	}

	// Initialize the controller and its dependencies.
	clientset := testop.New(3)
	context := &clusterd.Context{Clientset: clientset}
	controller := NewController(context, "rook/minio:mockTag")

	// Make the credentials.
	_, err := clientset.CoreV1().Secrets(namespace).Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "whatever",
			Namespace: namespace,
		},
		StringData: map[string]string{
			"MINIO_ACCESS_KEY": "user",
			"MINIO_SECRET_KEY": "pass",
		},
	})
	assert.Nil(t, err)

	// Call onAdd given the specified object store.
	controller.onAdd(objectstore)

	// Verify client service.
	svc, err := clientset.CoreV1().Services(namespace).Get(appName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, svc)
	assert.Equal(t, minioPort, svc.Spec.Ports[0].Port)

	// verify stateful set
	ss, err := clientset.AppsV1().StatefulSets(namespace).Get(appName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, ss)
	assert.Equal(t, int32(6), *ss.Spec.Replicas)
	assert.Equal(t, 2, len(ss.Spec.VolumeClaimTemplates))
	assert.Equal(t, 1, len(ss.Spec.Template.Spec.Containers))
	container := ss.Spec.Template.Spec.Containers[0]
	expectedContainerPorts := []v1.ContainerPort{{ContainerPort: minioPort}}
	assert.Equal(t, expectedContainerPorts, container.Ports)
	assert.Equal(t, 2, len(container.VolumeMounts))
	assert.Equal(t, 13, len(container.Args))

	expectedContainerArgs := []string{
		"server",
		"http://some_object_store-0.some_object_store.rook-minio-123.svc.cluster.local/data/rook-minio-test1",
		"http://some_object_store-0.some_object_store.rook-minio-123.svc.cluster.local/data/rook-minio-test2",
		"http://some_object_store-1.some_object_store.rook-minio-123.svc.cluster.local/data/rook-minio-test1",
		"http://some_object_store-1.some_object_store.rook-minio-123.svc.cluster.local/data/rook-minio-test2",
		"http://some_object_store-2.some_object_store.rook-minio-123.svc.cluster.local/data/rook-minio-test1",
		"http://some_object_store-2.some_object_store.rook-minio-123.svc.cluster.local/data/rook-minio-test2",
		"http://some_object_store-3.some_object_store.rook-minio-123.svc.cluster.local/data/rook-minio-test1",
		"http://some_object_store-3.some_object_store.rook-minio-123.svc.cluster.local/data/rook-minio-test2",
		"http://some_object_store-4.some_object_store.rook-minio-123.svc.cluster.local/data/rook-minio-test1",
		"http://some_object_store-4.some_object_store.rook-minio-123.svc.cluster.local/data/rook-minio-test2",
		"http://some_object_store-5.some_object_store.rook-minio-123.svc.cluster.local/data/rook-minio-test1",
		"http://some_object_store-5.some_object_store.rook-minio-123.svc.cluster.local/data/rook-minio-test2",
	}
	assert.Equal(t, expectedContainerArgs, ss.Spec.Template.Spec.Containers[0].Args)
}

func TestSpecVerification(t *testing.T) {
	namespace := "rook-minio-123"
	validObjectstore := &miniov.ObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
		},
		Spec: miniov.ObjectStoreSpec{
			Annotations: map[string]string{
				"test123": "this is a test",
				"rook.io": "this is a test",
			},
			Storage: rookalpha.StorageScopeSpec{
				NodeCount: 6,
				Selection: rookalpha.Selection{
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "rook-minio-test",
							},
							Spec: v1.PersistentVolumeClaimSpec{
								AccessModes: []v1.PersistentVolumeAccessMode{
									v1.ReadWriteOnce,
								},
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceStorage: resource.MustParse("10Gi"),
									},
								},
							},
						},
					},
				},
			},
			Credentials: v1.SecretReference{
				Name:      "whatever",
				Namespace: namespace,
			},
		},
	}

	objectstore := validObjectstore

	// Test sane objectstore.
	err := validateObjectStoreSpec(objectstore.Spec)
	assert.Nil(t, err)
	objectstore = validObjectstore

	// Test low and odd node counts.
	objectstore.Spec.Storage.NodeCount = 1
	err = validateObjectStoreSpec(objectstore.Spec)
	assert.NotNil(t, err)
	objectstore.Spec.Storage.NodeCount = 5
	err = validateObjectStoreSpec(objectstore.Spec)
	assert.NotNil(t, err)
	objectstore = validObjectstore
}

func TestMakeServerAddress(t *testing.T) {
	// pass empty string for cluster domain, default should be used
	serverAddress := makeServerAddress("my-store", "my-store", "rook-minio", "", 3, "/my-cool-data/dir123")
	assert.Equal(t, "http://my-store-3.my-store.rook-minio.svc.cluster.local/my-cool-data/dir123", serverAddress)

	// pass custom cluster domain, it should be used
	serverAddress = makeServerAddress("my-store", "my-store", "rook-minio", "acme.com", 3, "/data/mydir1")
	assert.Equal(t, "http://my-store-3.my-store.rook-minio.svc.acme.com/data/mydir1", serverAddress)
}

func TestGetPVCDataDir(t *testing.T) {
	assert.Equal(t, "/data/rook-test123", getPVCDataDir("rook-test123"))
}
