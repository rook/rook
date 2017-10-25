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

package crd

import (
	"github.com/coreos/pkg/capnslog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "rook-agent-crd")

// VolumeAttachmentCRDController is a controller to manage VolumeAttachment CRD objects
type VolumeAttachmentCRDController struct {
	client rest.Interface
}

// New creates a new VolumeAttachmentCRDController controller
func New(client rest.Interface) *VolumeAttachmentCRDController {
	return &VolumeAttachmentCRDController{
		client: client,
	}
}

// NewVolumeAttachment creates a reference of a Volumeattach CRD object
func NewVolumeAttachment(name, namespace, node, podNamespace, podName, mountDir string, readOnly bool) VolumeAttachment {
	volumeAttachmentObj := VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Attachments: []Attachment{
			{
				Node:         node,
				PodNamespace: podNamespace,
				PodName:      podName,
				MountDir:     mountDir,
				ReadOnly:     readOnly,
			},
		},
	}

	return volumeAttachmentObj
}

// Get queries the VolumeAttachment CRD from Kubernetes
func (c *VolumeAttachmentCRDController) Get(namespace, name string) (VolumeAttachment, error) {
	var result VolumeAttachment
	return result, c.client.Get().
		Resource(CustomResourceNamePlural).
		Namespace(namespace).
		Name(name).
		Do().Into(&result)
}

// Create creates the volume attach CRD resource in Kubernetes
func (c *VolumeAttachmentCRDController) Create(volumeAttachment VolumeAttachment) error {
	return c.client.Post().
		Resource(CustomResourceNamePlural).
		Namespace(volumeAttachment.Namespace).
		Body(&volumeAttachment).
		Do().Error()
}

// Update updates VolumeAttachment resource
func (c *VolumeAttachmentCRDController) Update(volumeAttachment VolumeAttachment) error {
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
func (c *VolumeAttachmentCRDController) Delete(namespace, name string) error {
	return c.client.Delete().
		Resource(CustomResourceNamePlural).
		Namespace(namespace).
		Name(name).
		Do().Error()
}
