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
