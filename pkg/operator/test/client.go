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

	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/pkg/api/v1"
)

func New(nodes int) *fake.Clientset {
	clientset := fake.NewSimpleClientset()
	for i := 0; i < nodes; i++ {
		n := &v1.Node{}
		n.Name = fmt.Sprintf("node%d", i)
		clientset.CoreV1().Nodes().Create(n)
	}
	return clientset
}
