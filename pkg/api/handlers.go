package api

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"

	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/castle/pkg/cephmgr"
	ceph "github.com/quantum/castle/pkg/cephmgr/client"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/model"
)

type Handler struct {
	EtcdClient        etcd.KeysAPI
	ConnectionFactory cephmgr.ConnectionFactory
	CephFactory       ceph.ConnectionFactory
}

func NewHandler(etcdClient etcd.KeysAPI, connFactory cephmgr.ConnectionFactory, cephFactory ceph.ConnectionFactory) *Handler {
	return &Handler{
		EtcdClient:        etcdClient,
		ConnectionFactory: connFactory,
		CephFactory:       cephFactory,
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

	clusterName, err := cephmgr.GetClusterName(h.EtcdClient)
	if err != nil {
		log.Printf("failed to get cluster name: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	nodes := make([]model.Node, len(clusterInventory.Nodes))
	i := 0
	for nodeID, n := range clusterInventory.Nodes {
		// look up all the disks that the current node has applied OSDs on
		appliedSerials, err := cephmgr.GetAppliedOSDs(nodeID, h.EtcdClient)
		if err != nil {
			log.Printf("failed to get applied OSDs for node %s: %+v", nodeID, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		storage := uint64(0)
		for _, d := range n.Disks {
			for _, s := range appliedSerials {
				if s == d.Serial {
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

type overallMonStatus struct {
	Status  cephmgr.MonStatusResponse    `json:"status"`
	Desired []*cephmgr.CephMonitorConfig `json:"desired"`
}

// Adds the device and configures an OSD on the device.
// POST
// /device
func (h *Handler) AddDevice(w http.ResponseWriter, r *http.Request) {
	device, err := loadDeviceFromBody(w, r)
	if err != nil {
		log.Printf("failed to load request body. %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = cephmgr.AddDesiredDevice(h.EtcdClient, device)
	if err != nil {
		log.Printf("failed to add device. %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// Stops the OSD and removes a device from participating in the cluster.
// POST
// /device/remove
func (h *Handler) RemoveDevice(w http.ResponseWriter, r *http.Request) {
	device, err := loadDeviceFromBody(w, r)
	if err != nil {
		log.Printf("failed to load request body. %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = cephmgr.RemoveDesiredDevice(h.EtcdClient, device)
	if err != nil {
		log.Printf("failed to remove device. %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func loadDeviceFromBody(w http.ResponseWriter, r *http.Request) (*cephmgr.Device, error) {
	// read/unmarshal the new pool to create from the request body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1024))
	if err == nil {
		r.Body.Close()
	} else {
		return nil, fmt.Errorf("failed to read request body: %+v", err)
	}

	var device cephmgr.Device
	if err := json.Unmarshal(body, &device); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request body '%s': %+v", string(body), err)
	}

	if device.Name == "" || device.NodeID == "" {
		return nil, fmt.Errorf("missing name or nodeId")
	}

	return &device, nil
}

// Gets the current crush map for the cluster.
// GET
// /crushmap
func (h *Handler) GetCrushMap(w http.ResponseWriter, r *http.Request) {
	// connect to ceph
	conn, ok := h.connectToCeph(w)
	if !ok {
		return
	}
	defer conn.Shutdown()

	// get the crush map
	crushmap, err := cephmgr.GetCrushMap(conn)
	if err != nil {
		log.Printf("failed to get crush map, err: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.Write([]byte(crushmap))
}

// Gets the monitors that have been created in this cluster.
// GET
// /mon
func (h *Handler) GetMonitors(w http.ResponseWriter, r *http.Request) {

	desiredMons, err := cephmgr.GetDesiredMonitors(h.EtcdClient)
	if err != nil {
		log.Printf("failed to load monitors: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	mons := []*cephmgr.CephMonitorConfig{}
	if len(desiredMons) == 0 {
		// no monitors to connect to
		FormatJsonResponse(w, mons)
		return
	}

	// connect to ceph
	adminConn, ok := h.connectToCeph(w)
	if !ok {
		return
	}
	defer adminConn.Shutdown()

	// get the monitor status
	monStatusResp, err := cephmgr.GetMonStatus(adminConn)
	if err != nil {
		log.Printf("failed to get mon_status, err: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	status := &overallMonStatus{Status: monStatusResp}
	for _, mon := range desiredMons {
		status.Desired = append(status.Desired, mon)
	}

	FormatJsonResponse(w, status)
}

// Gets the storage pools that have been created in this cluster.
// GET
// /pool
func (h *Handler) GetPools(w http.ResponseWriter, r *http.Request) {
	adminConn, ok := h.connectToCeph(w)
	if !ok {
		return
	}
	defer adminConn.Shutdown()

	// list pools using the ceph client
	cephPools, err := ceph.ListPools(adminConn)
	if err != nil {
		log.Printf("failed to list pools: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// convert ceph pools to model pools
	pools := make([]model.Pool, len(cephPools))
	for i, p := range cephPools {
		pools[i] = model.Pool{
			Name:   p.Name,
			Number: p.Number,
		}
	}

	FormatJsonResponse(w, pools)
}

// Creates a storage pool as specified by the request body.
// POST
// /pool
func (h *Handler) CreatePool(w http.ResponseWriter, r *http.Request) {
	// read/unmarshal the new pool to create from the request body
	var newPool model.Pool
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1024))
	if err == nil {
		r.Body.Close()
	} else {
		log.Printf("failed to read create pool request body: %+v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(body, &newPool); err != nil {
		log.Printf("failed to unmarshal create pool request body '%s': %+v", string(body), err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// connect to the ceph cluster and create the storage pool
	adminConn, ok := h.connectToCeph(w)
	if !ok {
		return
	}
	defer adminConn.Shutdown()

	info, err := ceph.CreatePool(adminConn, newPool.Name)
	if err != nil {
		log.Printf("failed to create new pool '%+v': %+v", newPool, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write([]byte(info))
}

func (h *Handler) connectToCeph(w http.ResponseWriter) (ceph.Connection, bool) {
	adminConn, err := h.ConnectionFactory.ConnectAsAdmin(h.CephFactory, h.EtcdClient)
	if err != nil {
		log.Printf("failed to connect to cluster as admin: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return nil, false
	}

	return adminConn, true
}
