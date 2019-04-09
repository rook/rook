/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package test

import (
	"fmt"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// New creates a fake K8s cluster
func New(nodes int) *fake.Clientset {
	clientset := fake.NewSimpleClientset()
	for i := 0; i < nodes; i++ {
		ready := v1.NodeCondition{Type: v1.NodeReady, Status: v1.ConditionTrue}
		name := fmt.Sprintf("node%d", i)
		n := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					ready,
				},
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeExternalIP,
						Address: fmt.Sprintf("%d.%d.%d.%d", i, i, i, i),
					},
				},
			},
		}
		clientset.CoreV1().Nodes().Create(n)
	}
	return clientset
}
