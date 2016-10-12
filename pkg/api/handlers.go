package api

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"

	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/castle/pkg/cephmgr"
	ceph "github.com/quantum/castle/pkg/cephmgr/client"
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

type overallMonStatus struct {
	Status  ceph.MonStatusResponse       `json:"status"`
	Desired []*cephmgr.CephMonitorConfig `json:"desired"`
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
