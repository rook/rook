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
package k8s

import (
	"fmt"

	"github.com/rook/rook/pkg/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func getNodes(clientset *kubernetes.Clientset) ([]model.Node, error) {
	nodes := []model.Node{}
	options := metav1.ListOptions{}
	nl, err := clientset.Nodes().List(options)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes. %+v", err)
	}

	for _, n := range nl.Items {
		node := model.Node{
			NodeID:      n.Status.NodeInfo.SystemUUID,
			PublicIP:    n.Spec.ExternalID,
			ClusterName: n.Namespace,
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}
