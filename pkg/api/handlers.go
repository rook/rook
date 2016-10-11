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
	Status  ceph.MonStatusResponse       `json:"status"`
	Desired []*cephmgr.CephMonitorConfig `json:"desired"`
}

// Adds the device and configures an OSD on the device.
// POST
// /device
func (h *Handler) AddDevice(w http.ResponseWriter, r *http.Request) {
	device, ok := handleLoadDeviceFromBody(w, r)
	if !ok {
		return
	}

	err := cephmgr.AddDesiredDevice(h.EtcdClient, device)
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
	device, ok := handleLoadDeviceFromBody(w, r)
	if !ok {
		return
	}

	err := cephmgr.RemoveDesiredDevice(h.EtcdClient, device)
	if err != nil {
		log.Printf("failed to remove device. %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func handleLoadDeviceFromBody(w http.ResponseWriter, r *http.Request) (*cephmgr.Device, bool) {
	body, ok := handleReadBody(w, r, "load device")
	if !ok {
		return nil, false
	}

	var device cephmgr.Device
	if err := json.Unmarshal(body, &device); err != nil {
		log.Printf("failed to unmarshal request body '%s': %+v", string(body), err)
		w.WriteHeader(http.StatusBadRequest)
		return nil, false
	}

	if device.Name == "" || device.NodeID == "" {
		log.Printf("missing name or nodeId: %+v", device)
		w.WriteHeader(http.StatusBadRequest)
		return nil, false
	}

	return &device, true
}

// Gets the current crush map for the cluster.
// GET
// /crushmap
func (h *Handler) GetCrushMap(w http.ResponseWriter, r *http.Request) {
	// connect to ceph
	conn, ok := h.handleConnectToCeph(w)
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
	adminConn, ok := h.handleConnectToCeph(w)
	if !ok {
		return
	}
	defer adminConn.Shutdown()

	// get the monitor status
	monStatusResp, err := ceph.GetMonStatus(adminConn)
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
	adminConn, ok := h.handleConnectToCeph(w)
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
	body, ok := handleReadBody(w, r, "create pool")
	if !ok {
		return
	}

	if err := json.Unmarshal(body, &newPool); err != nil {
		log.Printf("failed to unmarshal create pool request body '%s': %+v", string(body), err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// connect to the ceph cluster and create the storage pool
	adminConn, ok := h.handleConnectToCeph(w)
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

// Gets the images that have been created in this cluster.
// GET
// /image
func (h *Handler) GetImages(w http.ResponseWriter, r *http.Request) {
	adminConn, ok := h.handleConnectToCeph(w)
	if !ok {
		return
	}
	defer adminConn.Shutdown()

	// first list all the pools so that we can retrieve images from all pools
	pools, err := ceph.ListPools(adminConn)
	if err != nil {
		log.Printf("failed to list pools: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	result := []model.BlockImage{}

	// for each pool, open an IO context to get further details about all the images in the pool
	for _, p := range pools {
		ioctx, ok := handleOpenIOContext(w, adminConn, p.Name)
		if !ok {
			return
		}

		// get all the image names for the current pool
		imageNames, err := ioctx.GetImageNames()
		if err != nil {
			log.Printf("failed to get image names from pool %s: %+v", p.Name, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// for each image name, open the image and stat it for further details
		images := make([]model.BlockImage, len(imageNames))
		for i, name := range imageNames {
			image := ioctx.GetImage(name)
			image.Open(true)
			defer image.Close()
			imageStat, err := image.Stat()
			if err != nil {
				log.Printf("failed to stat image %s from pool %s: %+v", name, p.Name, err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			// add the current image's details to the result set
			images[i] = model.BlockImage{
				Name:     name,
				PoolName: p.Name,
				Size:     imageStat.Size,
			}
		}

		result = append(result, images...)
	}

	FormatJsonResponse(w, result)
}

// Creates a new image in this cluster.
// POST
// /image
func (h *Handler) CreateImage(w http.ResponseWriter, r *http.Request) {
	var newImage model.BlockImage
	body, ok := handleReadBody(w, r, "create image")
	if !ok {
		return
	}

	if err := json.Unmarshal(body, &newImage); err != nil {
		log.Printf("failed to unmarshal create image request body '%s': %+v", string(body), err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if newImage.Name == "" || newImage.PoolName == "" || newImage.Size == 0 {
		log.Printf("image missing required fields: %+v", newImage)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	adminConn, ok := h.handleConnectToCeph(w)
	if !ok {
		return
	}
	defer adminConn.Shutdown()

	ioctx, ok := handleOpenIOContext(w, adminConn, newImage.PoolName)
	if !ok {
		return
	}

	createdImage, err := ioctx.CreateImage(newImage.Name, newImage.Size, 22)
	if err != nil {
		log.Printf("failed to create image %+v: %+v", newImage, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write([]byte(fmt.Sprintf("succeeded created image %s", createdImage.Name())))
}

// Gets information needed to map an image to a local device
// GET
// /image/mapinfo
func (h *Handler) GetImageMapInfo(w http.ResponseWriter, r *http.Request) {
	// TODO: auth is extremely important here because we are returning cephx credentials

	adminConn, ok := h.handleConnectToCeph(w)
	if !ok {
		return
	}
	defer adminConn.Shutdown()

	monStatus, err := ceph.GetMonStatus(adminConn)
	if err != nil {
		log.Printf("failed to get monitor status, err: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// TODO: don't always return admin creds
	entity := "client.admin"
	user := "admin"
	secret, err := ceph.AuthGetKey(adminConn, entity)
	if err != nil {
		log.Printf("failed to get key for %s: %+v", entity, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	monAddrs := make([]string, len(monStatus.MonMap.Mons))
	for i, m := range monStatus.MonMap.Mons {
		monAddrs[i] = m.Address
	}

	mapInfo := model.BlockImageMapInfo{
		MonAddresses: monAddrs,
		UserName:     user,
		SecretKey:    secret,
	}

	FormatJsonResponse(w, mapInfo)
}

func handleReadBody(w http.ResponseWriter, r *http.Request, opName string) ([]byte, bool) {
	if r.Body == nil {
		log.Printf("nil request body for %s", opName)
		w.WriteHeader(http.StatusBadRequest)
		return nil, false
	}

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1024))
	if err == nil {
		r.Body.Close()
	} else {
		log.Printf("failed to read %s request body: %+v", opName, err)
		w.WriteHeader(http.StatusBadRequest)
		return nil, false
	}

	return body, true
}

func (h *Handler) handleConnectToCeph(w http.ResponseWriter) (ceph.Connection, bool) {
	adminConn, err := h.ConnectionFactory.ConnectAsAdmin(h.CephFactory, h.EtcdClient)
	if err != nil {
		log.Printf("failed to connect to cluster as admin: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return nil, false
	}

	return adminConn, true
}

func handleOpenIOContext(w http.ResponseWriter, conn ceph.Connection, pool string) (ceph.IOContext, bool) {
	ioctx, err := conn.OpenIOContext(pool)
	if err != nil {
		log.Printf("failed to open ioctx on pool %s: %+v", pool, err)
		w.WriteHeader(http.StatusInternalServerError)
		return nil, false
	}
	return ioctx, true
}
