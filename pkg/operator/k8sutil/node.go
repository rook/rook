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

// Package k8sutil for Kubernetes helpers.
package k8sutil

import (
	"fmt"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

func ValidNode(node v1.Node, placement rookalpha.Placement) (bool, error) {
	// a node cannot be disabled
	if node.Spec.Unschedulable {
		return false, nil
	}

	// a node matches the NodeAffinity configuration
	// ignoring `PreferredDuringSchedulingIgnoredDuringExecution` terms: they
	// should not be used to judge a node unusable
	if placement.NodeAffinity != nil && placement.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		nodeMatches := false
		for _, req := range placement.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
			nodeSelector, err := helper.NodeSelectorRequirementsAsSelector(req.MatchExpressions)
			if err != nil {
				return false, fmt.Errorf("failed to parse MatchExpressions: %+v, regarding as not match.", req.MatchExpressions)
			}
			if nodeSelector.Matches(labels.Set(node.Labels)) {
				nodeMatches = true
				break
			}
		}
		if !nodeMatches {
			return false, nil
		}
	}

	// a node is tainted and cannot be tolerated
	for _, taint := range node.Spec.Taints {
		isTolerated := false
		for _, toleration := range placement.Tolerations {
			if toleration.ToleratesTaint(&taint) {
				isTolerated = true
				break
			}
		}
		if !isTolerated {
			return false, nil
		}
	}

	// a node must be Ready
	for _, c := range node.Status.Conditions {
		if c.Type == v1.NodeReady {
			return true, nil
		}
	}
	logger.Infof("node %s is not ready. %+v", node.Name, node.Status.Conditions)
	return false, nil
}

func GetValidNodes(rookNodes []rookalpha.Node, clientset kubernetes.Interface, placement rookalpha.Placement) []rookalpha.Node {
	validNodes := []rookalpha.Node{}

	nodeOptions := metav1.ListOptions{}
	nodeOptions.TypeMeta.Kind = "Node"
	allNodes, err := clientset.CoreV1().Nodes().List(nodeOptions)
	if err != nil {
		// cannot list nodes, return empty nodes
		logger.Warningf("failed to list nodes: %v", err)
		return validNodes
	}

	for _, node := range allNodes.Items {
		for _, rookNode := range rookNodes {
			hostname := node.Labels[apis.LabelHostname]
			if len(hostname) == 0 {
				// fall back to the node name if the hostname label is not set
				hostname = node.Name
			}
			if rookNode.Name == hostname || rookNode.Name == node.Name {
				rookNode.Name = hostname
				valid, err := ValidNode(node, placement)
				if err != nil {
					logger.Warning("failed to validate node %s %v", rookNode.Name, err)
				} else if valid {
					validNodes = append(validNodes, rookNode)
				}
				break
			}
		}
	}
	return validNodes
}

// GetNodeNameFromHostname returns the name of the node resource looked up by the hostname label
// Typically these will be the same name, but sometimes they are not such as when nodes have a longer
// dns name, but the hostname is short.
func GetNodeNameFromHostname(clientset kubernetes.Interface, hostName string) (string, error) {
	options := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", apis.LabelHostname, hostName)}
	nodes, err := clientset.CoreV1().Nodes().List(options)
	if err != nil {
		return hostName, err
	}

	for _, node := range nodes.Items {
		return node.Name, nil
	}
	return hostName, fmt.Errorf("node not found")
}

// GetNodeHostNames returns the name of the node resource mapped to their hostname label.
// Typically these will be the same name, but sometimes they are not such as when nodes have a longer
// dns name, but the hostname is short.
func GetNodeHostNames(clientset kubernetes.Interface) (map[string]string, error) {
	nodes, err := clientset.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	nodeMap := map[string]string{}
	for _, node := range nodes.Items {
		nodeMap[node.Name] = node.Labels[apis.LabelHostname]
	}
	return nodeMap, nil
}
