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
	"encoding/json"
	"fmt"

	"github.com/coreos/pkg/capnslog"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"k8s.io/client-go/kubernetes"
)

const (
	tprKind = "Volumeattachment"
)

var tprlogger = capnslog.NewPackageLogger("github.com/rook/rook", "rook-agent-tpr")

// TPR is a controller to manage VolumeAttachment TPR objects
// CRD is available on v1.7.0. TPR became deprecated on v1.7.0
// Remove this code when TPR is not longer supported
type tpr struct {
	clientset kubernetes.Interface
}

// Get queries the VolumeAttachment TPR from Kubernetes
func (t *tpr) Get(namespace, name string) (*rookalpha.VolumeAttachment, error) {

	var result rookalpha.VolumeAttachment
	uri := fmt.Sprintf("apis/%s/%s/namespaces/%s/%s", rookalpha.CustomResourceGroup, rookalpha.Version, namespace, CustomResourceNamePlural)
	return &result, t.clientset.Core().RESTClient().Get().
		RequestURI(uri).
		Name(name).
		Do().
		Into(&result)
}

// List lists all the volume attachment TPR resources in the given namespace
func (t *tpr) List(namespace string) (*rookalpha.VolumeAttachmentList, error) {

	var result rookalpha.VolumeAttachmentList
	uri := fmt.Sprintf("apis/%s/%s/namespaces/%s/%s", rookalpha.CustomResourceGroup, rookalpha.Version, namespace, CustomResourceNamePlural)
	return &result, t.clientset.Core().RESTClient().Get().
		RequestURI(uri).
		Do().
		Into(&result)
}

// Create creates the volume attach TPR resource in Kubernetes
func (t *tpr) Create(volumeAttachment *rookalpha.VolumeAttachment) error {
	volumeAttachment.APIVersion = fmt.Sprintf("%s/%s", rookalpha.CustomResourceGroup, rookalpha.Version)
	volumeAttachment.Kind = tprKind
	body, _ := json.Marshal(volumeAttachment)
	uri := fmt.Sprintf("apis/%s/%s/namespaces/%s/%s", rookalpha.CustomResourceGroup, rookalpha.Version, volumeAttachment.Namespace, CustomResourceNamePlural)
	return t.clientset.Core().RESTClient().Post().
		RequestURI(uri).
		Body(body).
		Do().Error()
}

// Update updates VolumeAttachment TPR resource
func (t *tpr) Update(volumeAttachment *rookalpha.VolumeAttachment) error {
	volumeAttachment.APIVersion = fmt.Sprintf("%s/%s", rookalpha.CustomResourceGroup, rookalpha.Version)
	volumeAttachment.Kind = tprKind
	body, _ := json.Marshal(volumeAttachment)
	uri := fmt.Sprintf("apis/%s/%s/namespaces/%s/%s", rookalpha.CustomResourceGroup, rookalpha.Version, volumeAttachment.Namespace, CustomResourceNamePlural)
	err := t.clientset.Core().RESTClient().Put().
		RequestURI(uri).
		Name(volumeAttachment.Name).
		Body(body).
		Do().Error()
	if err != nil {
		tprlogger.Errorf("failed to update VolumeAttachment CRD. %+v", err)
		return err
	}
	tprlogger.Infof("updated Volumeattach TPR %s", volumeAttachment.ObjectMeta.Name)
	return nil
}

// Delete deletes the volume attach TPR resource in Kubernetes
func (t *tpr) Delete(namespace, name string) error {
	uri := fmt.Sprintf("apis/%s/%s/namespaces/%s/%s", rookalpha.CustomResourceGroup, rookalpha.Version, namespace, CustomResourceNamePlural)
	return t.clientset.Core().RESTClient().Delete().
		RequestURI(uri).
		Name(name).
		Do().
		Error()
}
