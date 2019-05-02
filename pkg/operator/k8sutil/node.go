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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
)

// ValidNode returns true if the node (1) is schedulable, (2) meets Rook's placement terms, and
// (3) is ready. False otherwise.
func ValidNode(node v1.Node, placement rookalpha.Placement) (bool, error) {
	if !GetNodeSchedulable(node) {
		return false, nil
	}

	p, err := NodeMeetsPlacementTerms(node, placement, false)
	if err != nil {
		return false, fmt.Errorf("failed to check if node meets Rook placement terms. %+v", err)
	}
	if !p {
		return false, nil
	}

	if !NodeIsReady(node) {
		return false, nil
	}

	return true, nil
}

// GetValidNodes returns all nodes that (1) are not cordoned, (2) meet Rook's placement terms, and
// (3) are ready.
func GetValidNodes(rookNodes []rookalpha.Node, clientset kubernetes.Interface, placement rookalpha.Placement) []rookalpha.Node {
	matchingK8sNodes, err := GetKubernetesNodesMatchingRookNodes(rookNodes, clientset)
	if err != nil {
		// cannot list nodes, return empty nodes
		logger.Errorf("failed to list nodes: %+v", err)
		return []rookalpha.Node{}
	}

	validK8sNodes := []v1.Node{}
	for _, n := range matchingK8sNodes {
		valid, err := ValidNode(n, placement)
		if err != nil {
			logger.Errorf("failed to validate node %s. %+v", n.Name, err)
		} else if valid {
			validK8sNodes = append(validK8sNodes, n)
		}
	}

	return RookNodesMatchingKubernetesNodes(rookNodes, validK8sNodes)
}

// GetNodeNameFromHostname returns the name of the node resource looked up by the hostname label
// Typically these will be the same name, but sometimes they are not such as when nodes have a longer
// dns name, but the hostname is short.
func GetNodeNameFromHostname(clientset kubernetes.Interface, hostName string) (string, error) {
	options := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", v1.LabelHostname, hostName)}
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
		nodeMap[node.Name] = node.Labels[v1.LabelHostname]
	}
	return nodeMap, nil
}

// GetNodeSchedulable returns a boolean if the node is tainted as Schedulable or not
// true -> Node is schedulable
// false -> Node is unschedulable
func GetNodeSchedulable(node v1.Node) bool {
	// some unit tests set this to quickly emulate an unschedulable node; if this is set to true,
	// we can shortcut deeper inspection for schedulability.
	if node.Spec.Unschedulable {
		return false
	}
	return true
}

// NodeMeetsPlacementTerms returns true if the Rook placement allows the node to have resources scheduled
// on it. A node is placeable if it (1) meets any affinity terms that may be set in the placement,
// and (2) its taints are tolerated by the placements tolerations.
// There is the option to ignore well known taints defined in WellKnownTaints. See WellKnownTaints
// for more information.
func NodeMeetsPlacementTerms(node v1.Node, placement rookalpha.Placement, ignoreWellKnownTaints bool) (bool, error) {
	a, err := NodeMeetsAffinityTerms(node, placement.NodeAffinity)
	if err != nil {
		return false, fmt.Errorf("failed to check if node %s meets affinity terms. regarding as not match. %+v", node.Name, err)
	}
	if !a {
		return false, nil
	}
	if !NodeIsTolerable(node, placement.Tolerations, ignoreWellKnownTaints) {
		return false, nil
	}
	return true, nil
}

// NodeMeetsAffinityTerms returns true if the node meets the terms of the node affinity.
// `PreferredDuringSchedulingIgnoredDuringExecution` terms are ignored and not used to judge a
// node's usability.
func NodeMeetsAffinityTerms(node v1.Node, affinity *v1.NodeAffinity) (bool, error) {
	// Terms are met automatically if relevant terms aren't set
	if affinity == nil || affinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return true, nil
	}
	for _, req := range affinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
		nodeSelector, err := helper.NodeSelectorRequirementsAsSelector(req.MatchExpressions)
		if err != nil {
			return false, fmt.Errorf("failed to parse affinity MatchExpressions: %+v, regarding as not match. %+v", req.MatchExpressions, err)
		}
		if nodeSelector.Matches(labels.Set(node.Labels)) {
			return true, nil
		}
	}
	return false, nil
}

// NodeIsTolerable returns true if the node's taints are all tolerated by the given tolerations.
// There is the option to ignore well known taints defined in WellKnownTaints. See WellKnownTaints
// for more information.
func NodeIsTolerable(node v1.Node, tolerations []v1.Toleration, ignoreWellKnownTaints bool) bool {
	for _, taint := range node.Spec.Taints {
		if ignoreWellKnownTaints && TaintIsWellKnown(taint) {
			continue
		}
		isTolerated := false
		for _, toleration := range tolerations {
			if toleration.ToleratesTaint(&taint) {
				isTolerated = true
				break
			}
		}
		if !isTolerated {
			return false
		}
	}
	return true
}

// NodeIsReady returns true if the node is ready. It returns false if the node is not ready.
func NodeIsReady(node v1.Node) bool {
	for _, c := range node.Status.Conditions {
		if c.Type == v1.NodeReady && c.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}

func rookNodeMatchesKubernetesNode(rookNode rookalpha.Node, kubernetesNode v1.Node) bool {
	hostname := normalizeHostname(kubernetesNode)
	return rookNode.Name == hostname || rookNode.Name == kubernetesNode.Name
}

func normalizeHostname(kubernetesNode v1.Node) string {
	hostname := kubernetesNode.Labels[v1.LabelHostname]
	if len(hostname) == 0 {
		// fall back to the node name if the hostname label is not set
		hostname = kubernetesNode.Name
	}
	return hostname
}

// GetKubernetesNodesMatchingRookNodes lists all the nodes in Kubernetes and returns all the
// Kubernetes nodes that have a corresponding match in the list of Rook nodes.
func GetKubernetesNodesMatchingRookNodes(rookNodes []rookalpha.Node, clientset kubernetes.Interface) ([]v1.Node, error) {
	nodes := []v1.Node{}
	nodeOptions := metav1.ListOptions{}
	nodeOptions.TypeMeta.Kind = "Node"
	k8sNodes, err := clientset.CoreV1().Nodes().List(nodeOptions)
	if err != nil {
		return nodes, fmt.Errorf("failed to list kubernetes nodes. %+v", err)
	}
	for _, kn := range k8sNodes.Items {
		for _, rn := range rookNodes {
			if rookNodeMatchesKubernetesNode(rn, kn) {
				nodes = append(nodes, kn)
			}
		}
	}
	return nodes, nil
}

// RookNodesMatchingKubernetesNodes returns only the given Rook nodes which have a corresponding
// match in the list of Kubernetes nodes.
func RookNodesMatchingKubernetesNodes(rookNodes []rookalpha.Node, kubernetesNodes []v1.Node) []rookalpha.Node {
	nodes := []rookalpha.Node{}
	for _, kn := range kubernetesNodes {
		for _, rn := range rookNodes {
			if rookNodeMatchesKubernetesNode(rn, kn) {
				rn.Name = normalizeHostname(kn)
				nodes = append(nodes, rn)
			}
		}
	}
	return nodes
}

// NodeIsInRookNodeList will return true if the target node is found in a given list of Rook nodes.
func NodeIsInRookNodeList(targetNodeName string, rookNodes []rookalpha.Node) bool {
	for _, rn := range rookNodes {
		if targetNodeName == rn.Name {
			return true
		}
	}
	return false
}
