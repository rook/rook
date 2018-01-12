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

// Package mon for the Ceph monitors.
package mon

import (
	"fmt"

	"github.com/rook/rook/pkg/util"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

// detect the nodes that are available for new mons to start.
func (c *Cluster) getMonNodes() ([]v1.Node, error) {
	availableNodes, err := c.getAvailableMonNodes()
	if err != nil {
		return nil, err
	}
	logger.Infof("Found %d running nodes without mons", len(availableNodes))

	if len(availableNodes) == 0 {
		return nil, fmt.Errorf("no nodes are available for mons")
	}

	return availableNodes, nil
}

func (c *Cluster) getAvailableMonNodes() ([]v1.Node, error) {
	nodeOptions := metav1.ListOptions{}
	nodeOptions.TypeMeta.Kind = "Node"
	nodes, err := c.context.Clientset.CoreV1().Nodes().List(nodeOptions)
	if err != nil {
		return nil, err
	}
	logger.Debugf("there are %d nodes available for existing %d mons", len(nodes.Items), len(c.clusterInfo.Monitors))

	// get the nodes that have mons assigned
	nodesInUse, err := c.getNodesWithMons()
	if err != nil {
		logger.Warningf("could not get nodes with mons. %+v", err)
		nodesInUse = util.NewSet()
	}

	// choose nodes for the new mons that don't have mons currently
	availableNodes := []v1.Node{}
	for _, node := range nodes.Items {
		if !nodesInUse.Contains(node.Name) && validNode(node, c.placement) {
			availableNodes = append(availableNodes, node)
		}
	}

	return availableNodes, nil
}

func (c *Cluster) getNodesWithMons() (*util.Set, error) {
	options := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", appName)}
	pods, err := c.context.Clientset.CoreV1().Pods(c.Namespace).List(options)
	if err != nil {
		return nil, err
	}
	nodes := util.NewSet()
	for _, pod := range pods.Items {
		hostname := pod.Spec.NodeSelector[apis.LabelHostname]
		logger.Debugf("mon pod on node %s", hostname)
		nodes.Add(hostname)
	}
	return nodes, nil
}
