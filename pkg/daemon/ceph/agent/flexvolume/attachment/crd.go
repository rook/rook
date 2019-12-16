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
*/

package attachment

import (
	"github.com/coreos/pkg/capnslog"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "rook-ceph-agent-crd")

// Attachment handles custom resource Volume storage operations.
type Attachment interface {
	Create(volumeAttachment *rookalpha.Volume) error
	Get(namespace, name string) (*rookalpha.Volume, error)
	List(namespace string) (*rookalpha.VolumeList, error)
	Update(volumeAttachment *rookalpha.Volume) error
	Delete(namespace, name string) error
}

// CRD is a controller to manage Volume CRD objects
type crd struct {
	context *clusterd.Context
}

// CreateController creates a new controller for volume attachment
func New(context *clusterd.Context) (Attachment, error) {
	return &crd{context: context}, nil
}

// Get queries the Volume CRD from Kubernetes
func (c *crd) Get(namespace, name string) (*rookalpha.Volume, error) {
	return c.context.RookClientset.RookV1alpha2().Volumes(namespace).Get(name, metav1.GetOptions{})
}

// List lists all the volume attachment CRD resources in the given namespace
func (c *crd) List(namespace string) (*rookalpha.VolumeList, error) {
	return c.context.RookClientset.RookV1alpha2().Volumes(namespace).List(metav1.ListOptions{})
}

// Create creates the volume attach CRD resource in Kubernetes
func (c *crd) Create(volumeAttachment *rookalpha.Volume) error {
	_, err := c.context.RookClientset.RookV1alpha2().Volumes(volumeAttachment.Namespace).Create(volumeAttachment)
	return err
}

// Update updates Volume resource
func (c *crd) Update(volumeAttachment *rookalpha.Volume) error {
	_, err := c.context.RookClientset.RookV1alpha2().Volumes(volumeAttachment.Namespace).Update(volumeAttachment)
	if err != nil {
		logger.Errorf("failed to update Volume CRD. %v", err)
		return err
	}
	logger.Infof("updated Volumeattach CRD %q", volumeAttachment.ObjectMeta.Name)
	return nil
}

// Delete deletes the volume attach CRD resource in Kubernetes
func (c *crd) Delete(namespace, name string) error {
	return c.context.RookClientset.RookV1alpha2().Volumes(namespace).Delete(name, &metav1.DeleteOptions{})
}
