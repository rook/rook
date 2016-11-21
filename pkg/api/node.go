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
package api

import (
	"net/http"

	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/cephmgr/osd"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/model"
)

// Gets the nodes that are part of this cluster.
// GET
// /node
func (h *Handler) GetNodes(w http.ResponseWriter, r *http.Request) {
	clusterInventory, err := inventory.LoadDiscoveredNodes(h.context.EtcdClient)
	if err != nil {
		logger.Errorf("failed to load discovered nodes: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	clusterName, err := mon.GetClusterName(h.context.EtcdClient)
	if err != nil {
		logger.Errorf("failed to get cluster name: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	nodes := make([]model.Node, len(clusterInventory.Nodes))
	i := 0
	for nodeID, n := range clusterInventory.Nodes {
		// look up all the disks that the current node has applied OSDs on
		appliedIDs, err := osd.GetAppliedOSDs(nodeID, h.context.EtcdClient)
		if err != nil {
			logger.Errorf("failed to get applied OSDs for node %s: %+v", nodeID, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		storage := uint64(0)
		for _, d := range n.Disks {
			for _, uuid := range appliedIDs {
				if d.UUID == uuid {
					// current disk is in applied OSD set, add its storage to the running total
					storage += d.Size
				}
			}
		}

		// determine the node's state/health
		_, isUnhealthy := clusterd.IsNodeUnhealthy(n)
		var state model.NodeState
		if isUnhealthy {
			state = model.Unhealthy
		} else {
			state = model.Healthy
		}

		nodes[i] = model.Node{
			NodeID:      nodeID,
			ClusterName: clusterName,
			PublicIP:    n.PublicIP,
			PrivateIP:   n.PrivateIP,
			Storage:     storage,
			LastUpdated: n.HeartbeatAge,
			State:       state,
			Location:    n.Location,
		}

		i++
	}

	FormatJsonResponse(w, nodes)
}
