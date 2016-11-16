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
	"net/http"

	"github.com/rook/rook/pkg/cephmgr/osd"
)

// Adds the device and configures an OSD on the device.
// POST
// /device
func (h *Handler) AddDevice(w http.ResponseWriter, r *http.Request) {
	device, ok := handleLoadDeviceFromBody(w, r)
	if !ok {
		return
	}

	err := osd.AddDesiredDevice(h.context.EtcdClient, device.Name, device.NodeID)
	if err != nil {
		logger.Errorf("failed to add device. %v", err)
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

	err := osd.RemoveDesiredDevice(h.context.EtcdClient, device.Name, device.NodeID)
	if err != nil {
		logger.Errorf("failed to remove device. %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func handleLoadDeviceFromBody(w http.ResponseWriter, r *http.Request) (*osd.Device, bool) {
	body, ok := handleReadBody(w, r, "load device")
	if !ok {
		return nil, false
	}

	var device osd.Device
	if err := json.Unmarshal(body, &device); err != nil {
		logger.Errorf("failed to unmarshal request body '%s': %+v", string(body), err)
		w.WriteHeader(http.StatusBadRequest)
		return nil, false
	}

	if device.Name == "" || device.NodeID == "" {
		logger.Errorf("missing name or nodeId: %+v", device)
		w.WriteHeader(http.StatusBadRequest)
		return nil, false
	}

	return &device, true
}
