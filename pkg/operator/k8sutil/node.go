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
	"strings"

	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
)

const (
	TopologyLabelPrefix = "topology.rook.io/"
)

// ValidNodeNoSched returns true if the node (1) meets Rook's placement terms,
// and (2) is ready. Unlike ValidNode, this method will ignore the
// Node.Spec.Unschedulable flag. False otherwise.
func ValidNodeNoSched(node v1.Node, placement rookv1.Placement) (bool, error) {
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

// ValidNode returns true if the node (1) is schedulable, (2) meets Rook's placement terms, and
// (3) is ready. False otherwise.
func ValidNode(node v1.Node, placement rookv1.Placement) (bool, error) {
	if !GetNodeSchedulable(node) {
		return false, nil
	}

	return ValidNodeNoSched(node, placement)
}

// GetValidNodes returns all nodes that (1) are not cordoned, (2) meet Rook's placement terms, and
// (3) are ready.
func GetValidNodes(rookStorage rookv1.StorageScopeSpec, clientset kubernetes.Interface, placement rookv1.Placement) []rookv1.Node {
	matchingK8sNodes, err := GetKubernetesNodesMatchingRookNodes(rookStorage.Nodes, clientset)
	if err != nil {
		// cannot list nodes, return empty nodes
		logger.Errorf("failed to list nodes: %+v", err)
		return []rookv1.Node{}
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

	return RookNodesMatchingKubernetesNodes(rookStorage, validK8sNodes)
}

func generateUniqueVolumeSourceName(sourceName, pvcName string) string {
	return fmt.Sprintf("%s-%s", sourceName, pvcName)
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

// GetNodeHostName returns the hostname label given the node name.
func GetNodeHostName(clientset kubernetes.Interface, nodeName string) (string, error) {
	node, err := clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return GetNodeHostNameLabel(node)
}

func GetNodeHostNameLabel(node *v1.Node) (string, error) {
	hostname, ok := node.Labels[v1.LabelHostname]
	if !ok {
		return "", fmt.Errorf("hostname not found on the node")
	}
	return hostname, nil
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
func NodeMeetsPlacementTerms(node v1.Node, placement rookv1.Placement, ignoreWellKnownTaints bool) (bool, error) {
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

func rookNodeMatchesKubernetesNode(rookNode rookv1.Node, kubernetesNode v1.Node) bool {
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
func GetKubernetesNodesMatchingRookNodes(rookNodes []rookv1.Node, clientset kubernetes.Interface) ([]v1.Node, error) {
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
func RookNodesMatchingKubernetesNodes(rookStorage rookv1.StorageScopeSpec, kubernetesNodes []v1.Node) []rookv1.Node {
	nodes := []rookv1.Node{}
	for _, kn := range kubernetesNodes {
		for _, rn := range rookStorage.Nodes {
			if rookNodeMatchesKubernetesNode(rn, kn) {
				rn.Name = normalizeHostname(kn)
				nodes = append(nodes, rn)
			}
		}
	}
	return nodes
}

// ExtractTopologyFromLabels extracts rook topology from labels and returns a map from topology type to value
func ExtractTopologyFromLabels(labels map[string]string, additionalTopologyIDs []string) map[string]string {
	topology := make(map[string]string)

	// check for the region k8s topology label that was deprecated in 1.17
	const regionLabel = "region"
	region, ok := labels[corev1.LabelZoneRegion]
	if ok {
		topology[regionLabel] = region
	}

	// check for the region k8s topology label that is GA in 1.17.
	// TODO: Replace with a const when we update to the K8s 1.17 go-client.
	region, ok = labels["topology.kubernetes.io/region"]
	if ok {
		topology[regionLabel] = region
	}

	// check for the zone k8s topology label that was deprecated in 1.17
	const zoneLabel = "zone"
	zone, ok := labels[corev1.LabelZoneFailureDomain]
	if ok {
		topology[zoneLabel] = zone
	}

	// check for the zone k8s topology label that is GA in 1.17.
	// TODO: Replace with a const when we update to the K8s 1.17 go-client.
	zone, ok = labels["topology.kubernetes.io/zone"]
	if ok {
		topology[zoneLabel] = zone
	}

	// get host
	host, ok := labels[corev1.LabelHostname]
	if ok {
		topology["host"] = host
	}

	// get the labels for the CRUSH map hierarchy
	for _, topologyID := range additionalTopologyIDs {
		if value, ok := labels[TopologyLabelPrefix+topologyID]; ok {
			topology[topologyID] = value
		}
	}
	return topology
}

// GenerateNodeAffinity will return v1.NodeAffinity or error
func GenerateNodeAffinity(nodeAffinity string) (*v1.NodeAffinity, error) {
	newNodeAffinity := &v1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
			NodeSelectorTerms: []v1.NodeSelectorTerm{
				{},
			},
		},
	}
	nodeLabels := strings.Split(nodeAffinity, ";")
	// For each label in 'nodeLabels', retrieve (key,value) pair and create nodeAffinity
	// '=' separates key from values
	// ',' separates values
	for _, nodeLabel := range nodeLabels {
		// If tmpNodeLabel is an array of length > 1
		// [0] is Key and [1] is comma separated values
		tmpNodeLabel := strings.Split(nodeLabel, "=")
		if len(tmpNodeLabel) > 1 {
			nodeLabelKey := strings.Trim(tmpNodeLabel[0], " ")
			tmpNodeLabelValue := tmpNodeLabel[1]
			nodeLabelValues := strings.Split(tmpNodeLabelValue, ",")
			if nodeLabelKey != "" && len(nodeLabelValues) > 0 {
				err := validation.IsQualifiedName(nodeLabelKey)
				if err != nil {
					return nil, fmt.Errorf("invalid label key: %s err: %v", nodeLabelKey, err)
				}
				for _, nodeLabelValue := range nodeLabelValues {
					nodeLabelValue = strings.Trim(nodeLabelValue, " ")
					err := validation.IsValidLabelValue(nodeLabelValue)
					if err != nil {
						return nil, fmt.Errorf("invalid label value: %s err: %v", nodeLabelValue, err)
					}
				}
				matchExpression := v1.NodeSelectorRequirement{
					Key:      nodeLabelKey,
					Operator: v1.NodeSelectorOpIn,
					Values:   nodeLabelValues,
				}
				newNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions =
					append(newNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions, matchExpression)
			}
		} else {
			nodeLabelKey := strings.Trim(tmpNodeLabel[0], " ")
			if nodeLabelKey != "" {
				err := validation.IsQualifiedName(nodeLabelKey)
				if err != nil {
					return nil, fmt.Errorf("invalid label key: %s err: %v", nodeLabelKey, err)
				}
				matchExpression := v1.NodeSelectorRequirement{
					Key:      nodeLabelKey,
					Operator: v1.NodeSelectorOpExists,
				}
				newNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions =
					append(newNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions, matchExpression)
			}
		}
	}
	return newNodeAffinity, nil
}
