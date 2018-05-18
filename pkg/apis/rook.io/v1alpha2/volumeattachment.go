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
package v1alpha2

import (
	rookv1alpha1 "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewVolume creates a reference of a Volumeattach CRD object
func NewVolume(name, namespace, node, podNamespace, podName, clusterName, mountDir string, readOnly bool) *Volume {
	volumeAttachmentObj := &Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Attachments: []Attachment{
			{
				Node:         node,
				PodNamespace: podNamespace,
				PodName:      podName,
				ClusterName:  clusterName,
				MountDir:     mountDir,
				ReadOnly:     readOnly,
			},
		},
	}

	return volumeAttachmentObj
}

// ConvertLegacyVolume takes a legacy rookv1alpha1 VolumeAttacment object and converts it to a current
// rookv1alpha2 Volume object.
func ConvertLegacyVolume(legacyVolume rookv1alpha1.VolumeAttachment) *Volume {
	va := &Volume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      legacyVolume.Name,
			Namespace: legacyVolume.Namespace,
		},
		Attachments: make([]Attachment, len(legacyVolume.Attachments)),
	}

	for i, la := range legacyVolume.Attachments {
		a := Attachment{
			Node:         la.Node,
			PodNamespace: la.PodNamespace,
			PodName:      la.PodName,
			ClusterName:  la.ClusterName,
			MountDir:     la.MountDir,
			ReadOnly:     la.ReadOnly,
		}

		va.Attachments[i] = a
	}

	return va
}
