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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sigs.k8s.io/yaml"
	"strings"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
)

// validNodeNoSched returns true if the node (1) meets Rook's placement terms,
// and (2) is ready. Unlike ValidNode, this method will ignore the
// Node.Spec.Unschedulable flag. False otherwise.
func validNodeNoSched(node v1.Node, placement cephv1.Placement) error {
	p, err := NodeMeetsPlacementTerms(node, placement, false)
	if err != nil {
		return fmt.Errorf("failed to check if node meets Rook placement terms. %+v", err)
	}
	if !p {
		return errors.New("placement settings do not match")
	}

	if !NodeIsReady(node) {
		return errors.New("node is not ready")
	}

	return nil
}

// ValidNode returns true if the node (1) is schedulable, (2) meets Rook's placement terms, and
// (3) is ready. False otherwise.
func ValidNode(node v1.Node, placement cephv1.Placement) error {
	if !GetNodeSchedulable(node) {
		return errors.New("node is unschedulable")
	}

	return validNodeNoSched(node, placement)
}

// GetValidNodes returns all nodes that (1) are not cordoned, (2) meet Rook's placement terms, and
// (3) are ready.
func GetValidNodes(ctx context.Context, rookStorage cephv1.StorageScopeSpec, clientset kubernetes.Interface, placement cephv1.Placement) []cephv1.Node {
	matchingK8sNodes, err := GetKubernetesNodesMatchingRookNodes(ctx, rookStorage.Nodes, clientset)
	if err != nil {
		// cannot list nodes, return empty nodes
		logger.Errorf("failed to list nodes: %+v", err)
		return []cephv1.Node{}
	}

	validK8sNodes := []v1.Node{}
	reasonsForSkippingNodes := map[string][]string{}
	for _, n := range matchingK8sNodes {
		err := ValidNode(n, placement)
		if err != nil {
			reason := err.Error()
			reasonsForSkippingNodes[reason] = append(reasonsForSkippingNodes[reason], n.Name)
		} else {
			validK8sNodes = append(validK8sNodes, n)
		}
	}

	for reason, nodes := range reasonsForSkippingNodes {
		logger.Infof("skipping creation of OSDs on nodes %v: %s", nodes, reason)
	}

	return RookNodesMatchingKubernetesNodes(rookStorage, validK8sNodes)
}

// GetNodeNameFromHostname returns the name of the node resource looked up by the hostname label
// Typically these will be the same name, but sometimes they are not such as when nodes have a longer
// dns name, but the hostname is short.
func GetNodeNameFromHostname(ctx context.Context, clientset kubernetes.Interface, hostName string) (string, error) {
	options := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", v1.LabelHostname, hostName)}
	nodes, err := clientset.CoreV1().Nodes().List(ctx, options)
	if err != nil {
		return hostName, err
	}

	for _, node := range nodes.Items {
		return node.Name, nil
	}
	return hostName, fmt.Errorf("node not found")
}

// GetNodeHostName returns the hostname label given the node name.
func GetNodeHostName(ctx context.Context, clientset kubernetes.Interface, nodeName string) (string, error) {
	node, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
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
func GetNodeHostNames(ctx context.Context, clientset kubernetes.Interface) (map[string]string, error) {
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
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
	return !node.Spec.Unschedulable
}

// NodeMeetsPlacementTerms returns true if the Rook placement allows the node to have resources scheduled
// on it. A node is placeable if it (1) meets any affinity terms that may be set in the placement,
// and (2) its taints are tolerated by the placements tolerations.
// There is the option to ignore well known taints defined in WellKnownTaints. See WellKnownTaints
// for more information.
func NodeMeetsPlacementTerms(node v1.Node, placement cephv1.Placement, ignoreWellKnownTaints bool) (bool, error) {
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
		nodeSelector, err := nodeSelectorRequirementsAsSelector(req.MatchExpressions)
		if err != nil {
			return false, fmt.Errorf("failed to parse affinity MatchExpressions: %+v, regarding as not match. %+v", req.MatchExpressions, err)
		}
		if nodeSelector.Matches(labels.Set(node.Labels)) {
			return true, nil
		}
	}
	return false, nil
}

// nodeSelectorRequirementsAsSelector method is copied from https://github.com/kubernetes/kubernetes. Since Rook uses this method and in
// Kubernetes v1.20.0 this method is not exported.

// nodeSelectorRequirementsAsSelector converts the []NodeSelectorRequirement api type into a struct that implements
// labels.Selector.
func nodeSelectorRequirementsAsSelector(nsm []v1.NodeSelectorRequirement) (labels.Selector, error) {
	if len(nsm) == 0 {
		return labels.Nothing(), nil
	}
	selector := labels.NewSelector()
	for _, expr := range nsm {
		var op selection.Operator
		switch expr.Operator {
		case v1.NodeSelectorOpIn:
			op = selection.In
		case v1.NodeSelectorOpNotIn:
			op = selection.NotIn
		case v1.NodeSelectorOpExists:
			op = selection.Exists
		case v1.NodeSelectorOpDoesNotExist:
			op = selection.DoesNotExist
		case v1.NodeSelectorOpGt:
			op = selection.GreaterThan
		case v1.NodeSelectorOpLt:
			op = selection.LessThan
		default:
			return nil, fmt.Errorf("%q is not a valid node selector operator", expr.Operator)
		}
		r, err := labels.NewRequirement(expr.Key, op, expr.Values)
		if err != nil {
			return nil, err
		}
		selector = selector.Add(*r)
	}
	return selector, nil
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
			localtaint := taint
			if toleration.ToleratesTaint(&localtaint) {
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

func rookNodeMatchesKubernetesNode(rookNode cephv1.Node, kubernetesNode v1.Node) bool {
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
func GetKubernetesNodesMatchingRookNodes(ctx context.Context, rookNodes []cephv1.Node, clientset kubernetes.Interface) ([]v1.Node, error) {
	nodes := []v1.Node{}
	k8sNodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nodes, fmt.Errorf("failed to list kubernetes nodes. %+v", err)
	}
	for _, rn := range rookNodes {
		nodeFound := false
		for _, kn := range k8sNodes.Items {
			if rookNodeMatchesKubernetesNode(rn, kn) {
				nodes = append(nodes, kn)
				nodeFound = true
				break
			}
		}
		if !nodeFound {
			logger.Warningf("failed to find matching kubernetes node for %q. Check the CephCluster's config and confirm each 'name' field in spec.storage.nodes matches their 'kubernetes.io/hostname' label", rn.Name)
		}
	}
	return nodes, nil
}

// GetNotReadyKubernetesNodes lists all the nodes that are in NotReady state
func GetNotReadyKubernetesNodes(ctx context.Context, clientset kubernetes.Interface) ([]v1.Node, error) {
	nodes := []v1.Node{}
	k8sNodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nodes, fmt.Errorf("failed to list kubernetes nodes. %v", err)
	}
	for _, node := range k8sNodes.Items {
		if !NodeIsReady(node) {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

// RookNodesMatchingKubernetesNodes returns only the given Rook nodes which have a corresponding
// match in the list of Kubernetes nodes.
func RookNodesMatchingKubernetesNodes(rookStorage cephv1.StorageScopeSpec, kubernetesNodes []v1.Node) []cephv1.Node {
	nodes := []cephv1.Node{}
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

// GenerateNodeAffinity will return v1.NodeAffinity or error
func GenerateNodeAffinity(nodeAffinity string) (*v1.NodeAffinity, error) {
	affinity, err := evaluateJSONOrYAMLInput(nodeAffinity)
	if err == nil {
		return affinity, nil
	}
	logger.Debugf("input not a valid JSON or YAML: %s, continuing", err)
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

func evaluateJSONOrYAMLInput(nodeAffinity string) (*v1.NodeAffinity, error) {
	var err error
	arr := []byte(nodeAffinity)
	if !json.Valid(arr) {
		arr, err = yaml.YAMLToJSON(arr)
		if err != nil {
			return nil, fmt.Errorf("failed to process YAML node affinity input: %v", err)
		}
	}
	var affinity *v1.NodeAffinity
	unmarshalErr := json.Unmarshal(arr, &affinity)
	if unmarshalErr != nil {
		return nil, fmt.Errorf("cannot unmarshal affinity: %s", unmarshalErr)
	}
	return affinity, nil
}
