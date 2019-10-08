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
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
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
		return nil, errors.Errorf("couldn't get IP of node %s", nr.Name)
	}
	return nr, nil
}
