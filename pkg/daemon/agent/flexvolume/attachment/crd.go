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
	"fmt"

	"github.com/coreos/pkg/capnslog"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/util/version"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "rook-agent-crd")

const (
	serverVersionV170 = "v1.7.0"
)

// Attachment handles custom resource VolumeAttachment storage operations.
// This interface goes away when there is no longer a need to support TPRs since
// we can call the RookClientset directly.
type Attachment interface {
	Create(volumeAttachment *rookalpha.VolumeAttachment) error
	Get(namespace, name string) (*rookalpha.VolumeAttachment, error)
	List(namespace string) (*rookalpha.VolumeAttachmentList, error)
	Update(volumeAttachment *rookalpha.VolumeAttachment) error
	Delete(namespace, name string) error
}

// CRD is a controller to manage VolumeAttachment CRD objects
type crd struct {
	context *clusterd.Context
}

// CreateController creates a new controller for volume attachment
func New(context *clusterd.Context) (Attachment, error) {

	// CRD is available on v1.7.0. TPR became deprecated on v1.7.0
	// Remove this code when TPR is not longer supported
	kubeVersion, err := k8sutil.GetK8SVersion(context.Clientset)
	if err != nil {
		return nil, fmt.Errorf("Error getting server version: %v", err)
	}
	if kubeVersion.AtLeast(version.MustParseSemantic(serverVersionV170)) {
		return &crd{
			context: context,
		}, nil
	}

	return &tpr{
		clientset: context.Clientset,
	}, nil
}

// Get queries the VolumeAttachment CRD from Kubernetes
func (c *crd) Get(namespace, name string) (*rookalpha.VolumeAttachment, error) {
	return c.context.RookClientset.Rook().VolumeAttachments(namespace).Get(name, metav1.GetOptions{})
}

// List lists all the volume attachment CRD resources in the given namespace
func (c *crd) List(namespace string) (*rookalpha.VolumeAttachmentList, error) {
	return c.context.RookClientset.Rook().VolumeAttachments(namespace).List(metav1.ListOptions{})
}

// Create creates the volume attach CRD resource in Kubernetes
func (c *crd) Create(volumeAttachment *rookalpha.VolumeAttachment) error {
	_, err := c.context.RookClientset.Rook().VolumeAttachments(volumeAttachment.Namespace).Create(volumeAttachment)
	return err
}

// Update updates VolumeAttachment resource
func (c *crd) Update(volumeAttachment *rookalpha.VolumeAttachment) error {
	_, err := c.context.RookClientset.Rook().VolumeAttachments(volumeAttachment.Namespace).Update(volumeAttachment)
	if err != nil {
		logger.Errorf("failed to update VolumeAttachment CRD. %+v", err)
		return err
	}
	logger.Infof("updated Volumeattach CRD %s", volumeAttachment.ObjectMeta.Name)
	return nil
}

// Delete deletes the volume attach CRD resource in Kubernetes
func (c *crd) Delete(namespace, name string) error {
	return c.context.RookClientset.Rook().VolumeAttachments(namespace).Delete(name, &metav1.DeleteOptions{})
}
