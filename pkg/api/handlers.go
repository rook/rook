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
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"

	ceph "github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/cephmgr/osd"
	"github.com/rook/rook/pkg/clusterd"
)

type Handler struct {
	context           *clusterd.Context
	ConnectionFactory mon.ConnectionFactory
	CephFactory       ceph.ConnectionFactory
}

func NewHandler(context *clusterd.Context, connFactory mon.ConnectionFactory, cephFactory ceph.ConnectionFactory) *Handler {
	return &Handler{
		context:           context,
		ConnectionFactory: connFactory,
		CephFactory:       cephFactory,
	}
}

// Format a json response
func FormatJsonResponse(w http.ResponseWriter, object interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	output, err := json.Marshal(object)
	if err != nil {
		logger.Errorf("failed to marshal object '%+v': %+v", object, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(output)
}

type overallMonStatus struct {
	Status  ceph.MonStatusResponse   `json:"status"`
	Desired []*mon.CephMonitorConfig `json:"desired"`
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
	crushmap, err := osd.GetCrushMap(conn)
	if err != nil {
		logger.Errorf("failed to get crush map, err: %+v", err)
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

	desiredMons, err := mon.GetDesiredMonitors(h.context.EtcdClient)
	if err != nil {
		logger.Errorf("failed to load monitors: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	mons := []*mon.CephMonitorConfig{}
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
		logger.Errorf("failed to get mon_status, err: %+v", err)
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
		logger.Errorf("nil request body for %s", opName)
		w.WriteHeader(http.StatusBadRequest)
		return nil, false
	}

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1024))
	if err == nil {
		r.Body.Close()
	} else {
		logger.Errorf("failed to read %s request body: %+v", opName, err)
		w.WriteHeader(http.StatusBadRequest)
		return nil, false
	}

	return body, true
}

func (h *Handler) handleConnectToCeph(w http.ResponseWriter) (ceph.Connection, bool) {
	adminConn, err := h.ConnectionFactory.ConnectAsAdmin(h.context, h.CephFactory)
	if err != nil {
		logger.Errorf("failed to connect to cluster as admin: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return nil, false
	}

	return adminConn, true
}

func handleOpenIOContext(w http.ResponseWriter, conn ceph.Connection, pool string) (ceph.IOContext, bool) {
	ioctx, err := conn.OpenIOContext(pool)
	if err != nil {
		logger.Errorf("failed to open ioctx on pool %s: %+v", pool, err)
		w.WriteHeader(http.StatusInternalServerError)
		return nil, false
	}
	return ioctx, true
}
