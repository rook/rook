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

package mon

import (
	"fmt"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeUsage is a mapping between a Node and computed metadata about the node
// that is used in monitor pod scheduling.
type NodeUsage struct {
	Node *v1.Node
	// The number of monitor pods assigned to the node
	MonCount int
	// The node is available for scheduling monitor pods. This is equivalent to
	// evaluating k8sutil.ValidNode(node, cephv1.GetMonPlacement(c.spec.Placement))
	MonValid bool
}

// Get the number of mons that the operator should be starting
func (c *Cluster) getTargetMonCount() (int, string, error) {

	// Get the full list of k8s nodes
	nodeOptions := metav1.ListOptions{}
	nodeOptions.TypeMeta.Kind = "Node"
	nodes, err := c.context.Clientset.CoreV1().Nodes().List(nodeOptions)
	if err != nil {
		return 0, "", fmt.Errorf("failed to get nodes. %+v", err)
	}

	// Get the nodes where it is possible to place mons
	availableNodes := []v1.Node{}
	for _, node := range nodes.Items {
		valid, err := k8sutil.ValidNode(node, cephv1.GetMonPlacement(c.spec.Placement))
		if err != nil {
			logger.Warning("failed to validate node %s %v", node.Name, err)
		} else if valid {
			availableNodes = append(availableNodes, node)
		}
	}

	target, msg := calcTargetMonCount(len(availableNodes), c.spec.Mon)
	return target, msg, nil
}

func calcTargetMonCount(nodes int, spec cephv1.MonSpec) (int, string) {
	minTarget := spec.Count
	preferredTarget := spec.PreferredCount

	if preferredTarget <= minTarget {
		msg := fmt.Sprintf("targeting the mon count %d", minTarget)
		return minTarget, msg
	}
	if nodes >= preferredTarget {
		msg := fmt.Sprintf("targeting the preferred mon count %d since there are %d available nodes", preferredTarget, nodes)
		return preferredTarget, msg
	}
	if spec.AllowMultiplePerNode {
		msg := fmt.Sprintf("targeting the preferred mon count %d even if not that many nodes since multiple mons are allowed per node", preferredTarget)
		return preferredTarget, msg
	}
	if nodes <= minTarget {
		msg := fmt.Sprintf("targeting the min mon count %d since there are only %d available nodes", minTarget, nodes)
		return minTarget, msg
	}

	// There are between minTarget and preferredTarget nodes. Find an odd number for mons closest to the number of nodes.
	intermediate := nodes
	if intermediate%2 == 0 {
		// Decrease to an odd number if there are an even number of nodes
		intermediate--
	}
	msg := fmt.Sprintf("targeting the calculated mon count %d since there are %d available nodes", intermediate, nodes)
	return intermediate, msg
}

// Look up the immutable node name from the hostname label
func getNodeNameFromHostname(nodes *v1.NodeList, hostname string) (string, bool) {
	for _, node := range nodes.Items {
		if node.Labels[v1.LabelHostname] == hostname {
			return node.Name, true
		}
		if node.Name == hostname {
			return node.Name, true
		}
	}
	return "", false
}

func getNodeInfoFromNode(n v1.Node) (*NodeInfo, error) {
	nr := &NodeInfo{
		Name:     n.Name,
		Hostname: n.Labels[v1.LabelHostname],
	}

	for _, ip := range n.Status.Addresses {
		if ip.Type == v1.NodeExternalIP || ip.Type == v1.NodeInternalIP {
			logger.Debugf("using IP %s for node %s", ip.Address, n.Name)
			nr.Address = ip.Address
			break
		}
	}
	if nr.Address == "" {
		return nil, fmt.Errorf("couldn't get IP of node %s", nr.Name)
	}
	return nr, nil
}

// Returns a hierarchical representation of the nodes in the cluster organized
// by zone failure domain and annotated with the number of monitor pods assigned
// to each node and if the node is valid for scheduling monitor pods. The
// purpose of this structure is to provide a unified view of information
// required for making monitor scheduling decisions.
func (c *Cluster) getNodeMonUsage() ([][]NodeUsage, error) {
	// get all k8s node objects
	nodeOptions := metav1.ListOptions{}
	nodeOptions.TypeMeta.Kind = "Node"
	nodes, err := c.context.Clientset.CoreV1().Nodes().List(nodeOptions)
	if err != nil {
		return nil, err
	}

	// get all pod objects labeled as a monitor
	podOptions := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", appName)}
	pods, err := c.context.Clientset.CoreV1().Pods(c.Namespace).List(podOptions)
	if err != nil {
		return nil, err
	}

	// generate a list of nodes that includes the number of monitor pods
	// assigned to each node. this is equivalent to:
	//
	//   SELECT Node, Count(*) FROM Nodes
	//   JOIN Pods ON Nodes.Hostname == Pods.Hostname
	//   GROUP BY Node
	//
	nodeUsages := []NodeUsage{}
	for i, node := range nodes.Items {
		valid, err := k8sutil.ValidNode(node, cephv1.GetMonPlacement(c.spec.Placement))
		if err != nil {
			logger.Warning("failed to validate node %s %v", node.Name, err)
			continue
		}
		nodeUsage := NodeUsage{Node: &nodes.Items[i], MonCount: 0, MonValid: valid}
		for _, pod := range pods.Items {
			hostname := pod.Spec.NodeSelector[v1.LabelHostname]
			if node.Name == hostname || node.Labels[v1.LabelHostname] == hostname {
				nodeUsage.MonCount++
			}
		}
		nodeUsages = append(nodeUsages, nodeUsage)
	}

	// generate a hierarchical representation of the nodes grouped by the name
	// of the node's zone failure domain label. the zone label named "" is used
	// for nodes without a zone annotation.
	nodesByZone := make(map[string][]NodeUsage)
	for _, nodeUsage := range nodeUsages {
		zone, ok := nodeUsage.Node.Labels["failure-domain.beta.kubernetes.io/zone"]
		if !ok {
			zone = ""
		}
		nodesByZone[zone] = append(nodesByZone[zone], nodeUsage)
	}

	// compute the final form used for scheduling. this form is an array of
	// nodes by zone, with non-labeled nodes appearing in the final zone. this
	// organization reflects the preference made by the scheduler of using nodes
	// with zone annotations before non-labeled nodes.
	res := [][]NodeUsage{}
	for zone, nodeUsage := range nodesByZone {
		if zone == "" {
			res = append(res, nodeUsage)
		} else {
			res = append([][]NodeUsage{nodeUsage}, res...)
		}
	}

	return res, nil
}
