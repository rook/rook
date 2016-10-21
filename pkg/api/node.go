package api

import (
	"log"
	"net/http"

	"github.com/quantum/castle/pkg/cephmgr"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/model"
)

// Gets the nodes that are part of this cluster.
// GET
// /node
func (h *Handler) GetNodes(w http.ResponseWriter, r *http.Request) {
	clusterInventory, err := inventory.LoadDiscoveredNodes(h.context.EtcdClient)
	if err != nil {
		log.Printf("failed to load discovered nodes: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	clusterName, err := cephmgr.GetClusterName(h.context.EtcdClient)
	if err != nil {
		log.Printf("failed to get cluster name: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	nodes := make([]model.Node, len(clusterInventory.Nodes))
	i := 0
	for nodeID, n := range clusterInventory.Nodes {
		// look up all the disks that the current node has applied OSDs on
		appliedIDs, err := cephmgr.GetAppliedOSDs(nodeID, h.context.EtcdClient)
		if err != nil {
			log.Printf("failed to get applied OSDs for node %s: %+v", nodeID, err)
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
			IPAddress:   n.IPAddress,
			Storage:     storage,
			LastUpdated: n.HeartbeatAge,
			State:       state,
			Location:    n.Location,
		}

		i++
	}

	FormatJsonResponse(w, nodes)
}
