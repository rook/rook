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
	"net/http"

	ceph "github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/model"
)

// Gets the storage pools that have been created in this cluster.
// GET
// /pool
func (h *Handler) GetPools(w http.ResponseWriter, r *http.Request) {
	pools, err := ceph.GetPools(h.context, h.config.clusterInfo.Name)
	if err != nil {
		logger.Errorf("failed to get pools: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
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
		logger.Errorf("failed to unmarshal create pool request body '%s': %+v", string(body), err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := ceph.CreatePoolWithProfile(h.context, h.config.clusterInfo.Name, newPool, newPool.Name)
	if err != nil {
		logger.Errorf("failed to create new pool '%s': %+v", newPool.Name, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write([]byte(fmt.Sprintf("pool '%s' created", newPool.Name)))
}
