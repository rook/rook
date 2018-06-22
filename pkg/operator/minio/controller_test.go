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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var appName = "some_object_store"
var magicPort int32 = 8675309

func TestOnAdd(t *testing.T) {
	namespace := "rook-minio-123"
	objectstore := &miniov.ObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
		},
		Spec: miniov.ObjectStoreSpec{
			Storage: rookalpha.StorageScopeSpec{NodeCount: 6},
			Port:    magicPort,
			Credentials: v1.SecretReference{
				Name:      "whatever",
				Namespace: namespace,
			},
			StorageSize: "1337G",
		},
	}

	// Initialize the controller and its dependencies.
	clientset := testop.New(3)
	context := &clusterd.Context{Clientset: clientset}
	controller := NewMinioController(context, "rook/minio:mockTag")

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
	assert.Equal(t, magicPort, svc.Spec.Ports[0].Port)

	// verify stateful set
	ss, err := clientset.AppsV1beta2().StatefulSets(namespace).Get(appName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, ss)
	assert.Equal(t, int32(6), *ss.Spec.Replicas)
	assert.Equal(t, 1, len(ss.Spec.VolumeClaimTemplates))
	assert.Equal(t, 1, len(ss.Spec.Template.Spec.Containers))
	container := ss.Spec.Template.Spec.Containers[0]
	expectedContainerPorts := []v1.ContainerPort{{ContainerPort: magicPort}}
	assert.Equal(t, expectedContainerPorts, container.Ports)
}

func TestSpecVerification(t *testing.T) {
	namespace := "rook-minio-123"
	validObjectstore := &miniov.ObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
		},
		Spec: miniov.ObjectStoreSpec{
			Storage: rookalpha.StorageScopeSpec{NodeCount: 6},
			Port:    magicPort,
			Credentials: v1.SecretReference{
				Name:      "whatever",
				Namespace: namespace,
			},
			StorageSize: "1337G",
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

	// Test invalid ports.
	objectstore = validObjectstore
	objectstore.Spec.Port = 1000
	err = validateObjectStoreSpec(objectstore.Spec)
	assert.NotNil(t, err)
}
