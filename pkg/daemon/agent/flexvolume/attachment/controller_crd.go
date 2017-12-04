/*
Copyright 2017 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/

package attachment

import (
	"fmt"

	"github.com/coreos/pkg/capnslog"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/util/version"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "rook-agent-crd")

const (
	serverVersionV170 = "v1.7.0"
)

// VolumeAttachmentController handles custom resource VolumeAttachment storage operations
type Controller interface {
	Create(volumeAttachment rookalpha.VolumeAttachment) error
	Get(namespace, name string) (rookalpha.VolumeAttachment, error)
	List(namespace string) (rookalpha.VolumeAttachmentList, error)
	Update(volumeAttachment rookalpha.VolumeAttachment) error
	Delete(namespace, name string) error
}

// CRDController is a controller to manage VolumeAttachment CRD objects
type CRDController struct {
	client rest.Interface
}

// CreateController creates a new controller for volume attachment
func CreateController(clientset kubernetes.Interface, volumeAttachmentCRDClient rest.Interface) (Controller, error) {

	// CRD is available on v1.7.0. TPR became deprecated on v1.7.0
	// Remove this code when TPR is not longer supported
	kubeVersion, err := k8sutil.GetK8SVersion(clientset)
	if err != nil {
		return nil, fmt.Errorf("Error getting server version: %v", err)
	}
	if kubeVersion.AtLeast(version.MustParseSemantic(serverVersionV170)) {
		return &CRDController{
			client: volumeAttachmentCRDClient,
		}, nil
	}

	return &TPRController{
		clientset: clientset,
	}, nil
}

// Get queries the VolumeAttachment CRD from Kubernetes
func (c *CRDController) Get(namespace, name string) (rookalpha.VolumeAttachment, error) {
	var result rookalpha.VolumeAttachment
	return result, c.client.Get().
		Resource(CustomResourceNamePlural).
		Namespace(namespace).
		Name(name).
		Do().Into(&result)
}

// List lists all the volume attachment CRD resources in the given namespace
func (c *CRDController) List(namespace string) (rookalpha.VolumeAttachmentList, error) {
	var result rookalpha.VolumeAttachmentList
	return result, c.client.Get().
		Resource(CustomResourceNamePlural).
		Namespace(namespace).
		Do().Into(&result)
}

// Create creates the volume attach CRD resource in Kubernetes
func (c *CRDController) Create(volumeAttachment rookalpha.VolumeAttachment) error {
	return c.client.Post().
		Resource(CustomResourceNamePlural).
		Namespace(volumeAttachment.Namespace).
		Body(&volumeAttachment).
		Do().Error()
}

// Update updates VolumeAttachment resource
func (c *CRDController) Update(volumeAttachment rookalpha.VolumeAttachment) error {
	err := c.client.Put().
		Name(volumeAttachment.ObjectMeta.Name).
		Namespace(volumeAttachment.ObjectMeta.Namespace).
		Resource(CustomResourceNamePlural).
		Body(&volumeAttachment).
		Do().Error()
	if err != nil {
		logger.Errorf("failed to update VolumeAttachment CRD. %+v", err)
		return err
	}
	logger.Infof("updated Volumeattach CRD %s", volumeAttachment.ObjectMeta.Name)
	return nil
}

// Delete deletes the volume attach CRD resource in Kubernetes
func (c *CRDController) Delete(namespace, name string) error {
	return c.client.Delete().
		Resource(CustomResourceNamePlural).
		Namespace(namespace).
		Name(name).
		Do().Error()
}
