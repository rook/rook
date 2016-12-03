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

	"github.com/rook/rook/pkg/cephmgr/rgw"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/util"
)

// Creates a new object store in this cluster.
// POST
// /objectstore
func (h *Handler) CreateObjectStore(w http.ResponseWriter, r *http.Request) {

	if err := rgw.EnableObjectStore(h.context); err != nil {
		logger.Errorf("failed to create object store: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	logger.Debugf("started async creation of the object store")
	w.WriteHeader(http.StatusAccepted)
}

// Removes the object store from this cluster.
// POST
// /objectstore/remove
func (h *Handler) RemoveObjectStore(w http.ResponseWriter, r *http.Request) {
	if err := rgw.RemoveObjectStore(h.context); err != nil {
		logger.Errorf("failed to remove object store: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	logger.Debugf("started async deletion of the object store")
	w.WriteHeader(http.StatusAccepted)
}

// Gets connection information to the object store in this cluster.
// GET
// /objectstore/connectioninfo
func (h *Handler) GetObjectStoreConnectionInfo(w http.ResponseWriter, r *http.Request) {
	// TODO: auth is extremely important here because we are returning RGW credentials
	// https://github.com/rook/rook/issues/209

	// TODO: support other types of connection info to object store (such as swift)
	// https://github.com/rook/rook/issues/255

	accessKey, secretKey, err := rgw.GetBuiltinUserAccessInfo(h.context.EtcdClient)
	if err != nil {
		handleConnectionLookupFailure(err, "builtin user access info", w)
		return
	}

	clusterInventory, err := inventory.LoadDiscoveredNodes(h.context.EtcdClient)
	if err != nil {
		logger.Errorf("failed to load discovered nodes: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	host, ipEndpoint, found, err := rgw.GetRGWEndpoints(h.context.EtcdClient, clusterInventory)
	if err != nil {
		handleConnectionLookupFailure(err, "rgw endpoints", w)
		return
	} else if !found {
		logger.Errorf("failed to find rgw endpoints")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	s3Info := model.ObjectStoreS3Info{
		Host:       host,
		IPEndpoint: ipEndpoint,
		AccessKey:  accessKey,
		SecretKey:  secretKey,
	}

	FormatJsonResponse(w, s3Info)
}

func handleConnectionLookupFailure(err error, action string, w http.ResponseWriter) {
	logger.Errorf("failed to get %s: %+v", action, err)
	if util.IsEtcdKeyNotFound(err) {
		w.WriteHeader(http.StatusNotFound)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
}
