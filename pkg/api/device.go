package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/rook/rook/pkg/cephmgr"
)

// Adds the device and configures an OSD on the device.
// POST
// /device
func (h *Handler) AddDevice(w http.ResponseWriter, r *http.Request) {
	device, ok := handleLoadDeviceFromBody(w, r)
	if !ok {
		return
	}

	err := cephmgr.AddDesiredDevice(h.context.EtcdClient, device.Name, device.NodeID)
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

	err := cephmgr.RemoveDesiredDevice(h.context.EtcdClient, device.Name, device.NodeID)
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
