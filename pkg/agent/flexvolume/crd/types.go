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
	"fmt"

	"github.com/rook/rook/pkg/operator/k8sutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/util/version"
)

const (
	serverVersionV170 = "v1.7.0"
)

// schemeGroupVersion is group version used to register these objects
var schemeGroupVersion = schema.GroupVersion{Group: k8sutil.CustomResourceGroup, Version: k8sutil.V1Alpha1}

type VolumeAttachment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Attachments       []Attachment `json:"attachments"`
}

type Attachment struct {
	Node         string `json:"node"`
	PodNamespace string `json:"podNamespace"`
	PodName      string `json:"podName"`
	ClusterName  string `json:"clusterName"`
	MountDir     string `json:"mountDir"`
	ReadOnly     bool   `json:"readOnly"`
}

type VolumeAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VolumeAttachment `json:"items"`
}

// VolumeAttachmentController handles custom resource VolumeAttachment storage operations
type VolumeAttachmentController interface {
	Create(volumeAttachment VolumeAttachment) error
	Get(namespace, name string) (VolumeAttachment, error)
	List(namespace string) (VolumeAttachmentList, error)
	Update(volumeAttachment VolumeAttachment) error
	Delete(namespace, name string) error
}

func NewVolumeAttachmentController(clientset kubernetes.Interface,
	volumeAttachmentCRDClient rest.Interface) (VolumeAttachmentController, error) {

	var controller VolumeAttachmentController
	// CRD is available on v1.7.0. TPR became deprecated on v1.7.0
	// Remove this code when TPR is not longer supported
	kubeVersion, err := k8sutil.GetK8SVersion(clientset)
	if err != nil {
		return nil, fmt.Errorf("Error getting server version: %v", err)
	}
	if kubeVersion.AtLeast(version.MustParseSemantic(serverVersionV170)) {
		controller = New(volumeAttachmentCRDClient)
	} else {
		controller = NewTPR(clientset)
	}

	return controller, nil
}
