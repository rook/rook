/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package newosd

import (
	"fmt"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

// TODO: test that user-specified node overrides aren't clobbered by anything in this file

// Get the full scope of the user's storage request. Basically, if `UseAllNodes` is set, populate
// the resultant StorageScopeSpec with every single Kubernetes node.
//
// The resultant StorageScopeSpec's Nodes will be resolved before the spec is returned.
func (c *Controller) getDesiredStorage() (*rookalpha.StorageScopeSpec, error) {
	desiredStorage := c.cephCluster.spec.Storage.DeepCopy()

	if desiredStorage.UseAllNodes {
		desiredStorage.Nodes = []rookalpha.Node{}
		hostnameMap, err := k8sutil.GetNodeHostNames(c.context.Clientset)
		if err != nil {
			return nil, fmt.Errorf("failed to determine the scope of the user-defined storage request. "+
				"failed to get Kubernetes nodes and hostnames. %+v", err)
		}
		for _, hostname := range hostnameMap {
			storageNode := rookalpha.Node{
				Name: hostname,
			}
			desiredStorage.Nodes = append(desiredStorage.Nodes, storageNode)
		}
	}

	// resolve desired nodes
	desiredStorage.Nodes = c.resolvedNodes(desiredStorage)

	return desiredStorage, nil
}

// return all of the nodes from the desiredStorage which can be resolved, in their resolved state
func (c *Controller) resolvedNodes(desiredStorage *rookalpha.StorageScopeSpec) []rookalpha.Node {
	resolvedNodes := make([]rookalpha.Node, 0, len(desiredStorage.Nodes))
	for _, n := range desiredStorage.Nodes {
		rn := c.resolveNode(desiredStorage, &n)
		if rn == nil {
			logger.Errorf("cannot use node for storage. node did not resolve: %+v", n)
			continue
		}
		resolvedNodes = append(resolvedNodes, *rn)
	}
	return resolvedNodes
}

// fully resolve the storage config and resources for this node
func (c *Controller) resolveNode(desiredStorage *rookalpha.StorageScopeSpec, node *rookalpha.Node) *rookalpha.Node {
	rookNode := desiredStorage.ResolveNode(node.Name)
	if rookNode == nil {
		return nil
	}
	resources := cephv1.GetOSDResources(c.cephCluster.spec.Resources)
	rookNode.Resources = k8sutil.MergeResourceRequirements(rookNode.Resources, resources)

	return rookNode
}

// Find the nodes on which Rook is allowed to provision new OSDs. This will return a subset of nodes
// defined in desiredStorage.
func (c *Controller) getProvisionableNodes(desiredStorage *rookalpha.StorageScopeSpec) []rookalpha.Node {
	if desiredStorage.UseAllNodes == false && len(desiredStorage.Nodes) == 0 {
		logger.Warningf("useAllNodes is set to false and no nodes are specified, no OSD pods are going to be created")
		return []rookalpha.Node{}
	}

	// if any nodes (even user-specified ones) don't meet placement criteria, don't count them provisionable
	provisionableNodes := k8sutil.GetValidNodes(
		*desiredStorage, c.context.Clientset, cephv1.GetOSDPlacement(c.cephCluster.spec.Placement))
	logger.Debugf("nodes available for OSD provisioning: %+v", provisionableNodes)

	// indicate in the info message report what nodes were considered for provisionability
	nodeSearchScope := "user-requested"
	if desiredStorage.UseAllNodes {
		nodeSearchScope = "Kubernetes"
	}
	logger.Infof("%d of %d %s nodes are available for OSD provisioning",
		len(provisionableNodes), len(desiredStorage.Nodes), nodeSearchScope)

	return provisionableNodes
}
