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
	"github.com/rook/rook/pkg/util"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// getMonNodes detects the nodes that are available for new mons to start.
func (c *Cluster) getMonNodes() ([]v1.Node, error) {
	availableNodes, nodes, err := c.getAvailableMonNodes()
	if err != nil {
		return nil, err
	}
	logger.Infof("Found %d running nodes without mons", len(availableNodes))

	// if all nodes already have mons and the user has given the mon.count, add all nodes to be available
	if c.spec.Mon.AllowMultiplePerNode && len(availableNodes) == 0 {
		logger.Infof("All nodes are running mons. Adding all %d nodes to the availability.", len(nodes.Items))
		for _, node := range nodes.Items {
			valid, err := k8sutil.ValidNode(node, cephv1.GetMonPlacement(c.spec.Placement))
			if err != nil {
				logger.Warning("failed to validate node %s %v", node.Name, err)
			} else if valid {
				availableNodes = append(availableNodes, node)
			}
		}
	}

	return availableNodes, nil
}

func (c *Cluster) getAvailableMonNodes() ([]v1.Node, *v1.NodeList, error) {
	nodeOptions := metav1.ListOptions{}
	nodeOptions.TypeMeta.Kind = "Node"
	nodes, err := c.context.Clientset.CoreV1().Nodes().List(nodeOptions)
	if err != nil {
		return nil, nil, err
	}
	logger.Debugf("there are %d nodes available for %d mons", len(nodes.Items), len(c.clusterInfo.Monitors))

	// get the nodes that have mons assigned
	nodesInUse, err := c.getNodesWithMons(nodes)
	if err != nil {
		logger.Warningf("could not get nodes with mons. %+v", err)
		nodesInUse = util.NewSet()
	}

	// choose nodes for the new mons that don't have mons currently
	availableNodes := []v1.Node{}
	for _, node := range nodes.Items {
		if !nodesInUse.Contains(node.Name) {
			valid, err := k8sutil.ValidNode(node, cephv1.GetMonPlacement(c.spec.Placement))
			if err != nil {
				logger.Warning("failed to validate node %s %v", node.Name, err)
			} else if valid {
				availableNodes = append(availableNodes, node)
			}
		}
	}

	return availableNodes, nodes, nil
}

func (c *Cluster) getNodesWithMons(nodes *v1.NodeList) (*util.Set, error) {
	// get the mon pods and their node affinity
	options := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", appName)}
	pods, err := c.context.Clientset.CoreV1().Pods(c.Namespace).List(options)
	if err != nil {
		return nil, err
	}
	nodesInUse := util.NewSet()
	for _, pod := range pods.Items {
		hostname := pod.Spec.NodeSelector[v1.LabelHostname]
		logger.Debugf("mon pod on node %s", hostname)
		name, ok := getNodeNameFromHostname(nodes, hostname)
		if !ok {
			logger.Errorf("mon %s on hostname %s not found in node list", pod.Name, hostname)
		}
		nodesInUse.Add(name)
	}
	return nodesInUse, nil
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
