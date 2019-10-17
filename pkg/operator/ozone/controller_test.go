/*
Copyright 2019 The Rook Authors. All rights reserved.

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
package ozone

import (
	"github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/stretchr/testify/assert"
	"testing"

	ozonealpha "github.com/rook/rook/pkg/apis/ozone.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	testop "github.com/rook/rook/pkg/operator/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var appName = "some_object_store"

const (
	imageName = "custom/ozone:1.0.0"
)

//test the basic behavior of Ozone controller
func TestOnAdd(t *testing.T) {
	namespace := "rook-ozone-123"
	objectstore := &ozonealpha.OzoneObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
		},
		Spec: ozonealpha.OzoneObjectStoreSpec{
			OzoneVersion: ozonealpha.OzoneVersionSpec{
				Image: imageName,
			},
			Storage: v1alpha2.StorageScopeSpec{
				NodeCount: 5,
			},
		},
	}

	// Initialize the controller and its dependencies.
	clientset := testop.New(3)
	context := &clusterd.Context{Clientset: clientset}
	controller := NewController(context)
	controller.templateDir = "./templates"
	// Call onAdd given the specified object store.
	controller.onAdd(objectstore)

	// verify stateful set
	scm, err := clientset.AppsV1().StatefulSets(namespace).Get("scm", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "scm", scm.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, imageName, scm.Spec.Template.Spec.Containers[0].Image)

	//Ozone Manager
	om, err := clientset.AppsV1().StatefulSets(namespace).Get("om", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "om", om.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, imageName, om.Spec.Template.Spec.Containers[0].Image)

	//S3 gateway
	s3g, err := clientset.AppsV1().StatefulSets(namespace).Get("s3g", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "s3g", s3g.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, imageName, s3g.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, int32(1), *s3g.Spec.Replicas)

	//Datanode
	datanode, err := clientset.AppsV1().StatefulSets(namespace).Get("datanode", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "datanode", datanode.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, imageName, datanode.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, int32(5), *datanode.Spec.Replicas)
}
