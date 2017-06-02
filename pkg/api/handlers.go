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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	ceph "github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/ceph/osd"
	"github.com/rook/rook/pkg/clusterd"
)

type Handler struct {
	context      *clusterd.Context
	config       *Config
	cephExporter *CephExporter
}

func newHandler(context *clusterd.Context, config *Config) *Handler {
	return &Handler{
		context: context,
		config:  config,
	}
}

// RegisterMetrics registers all collected metrics by this API server.  Note this should be called in a
// goroutine because it will retry upon failure and block until successful.
func (h *Handler) RegisterMetrics(retryMs int) error {
	h.cephExporter = NewCephExporter(h)
	if err := prometheus.Register(h.cephExporter); err != nil {
		return fmt.Errorf("failed to register metrics: %+v", err)
	}

	return nil
}

func (h *Handler) Shutdown() {
	if h.cephExporter != nil {
		prometheus.Unregister(h.cephExporter)
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
	// get the crush map
	crushmap, err := osd.GetCrushMap(h.context, h.config.ClusterInfo.Name)
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

	desiredMons, err := h.config.ClusterHandler.GetMonitors()
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

	// get the monitor status
	monStatusResp, err := ceph.GetMonStatus(h.context, h.config.ClusterInfo.Name)
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
