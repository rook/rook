package api

import (
	"encoding/json"
	"log"
	"net/http"

	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/castle/pkg/castled"
	"github.com/quantum/castle/pkg/clusterd/inventory"
)

type Handler struct {
	EtcdClient etcd.KeysAPI
}

func NewHandler(etcdClient etcd.KeysAPI) *Handler {
	return &Handler{
		EtcdClient: etcdClient,
	}
}

// Format a json response
func FormatJsonResponse(w http.ResponseWriter, object interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	output, err := json.Marshal(object)
	if err != nil {
		log.Printf("failed to marshal object '%+v': %+v", object, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(output)
}

// Gets the nodes that are part of this cluster.
// GET
// /node
func (h *Handler) GetNodes(w http.ResponseWriter, r *http.Request) {
	clusterInventory, err := inventory.LoadDiscoveredNodes(h.EtcdClient)
	if err != nil {
		log.Printf("failed to load discovered nodes: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	nodes := make([]Node, len(clusterInventory.Nodes))
	i := 0
	for nodeID, n := range clusterInventory.Nodes {
		// look up all the disks that the current node has applied OSDs on
		appliedSerials, err := castled.GetAppliedOSDs(nodeID, h.EtcdClient)
		if err != nil {
			log.Printf("failed to get applied OSDs for node %s: %+v", nodeID, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		storage := uint64(0)
		for _, d := range n.Disks {
			for s := range appliedSerials.Iter() {
				if s == d.Serial {
					// current disk is in applied OSD set, add its storage to the running total
					storage += d.Size
				}
			}
		}

		nodes[i] = Node{
			NodeID:    nodeID,
			IPAddress: n.IPAddress,
			Storage:   storage,
		}

		i++
	}

	FormatJsonResponse(w, nodes)
}
