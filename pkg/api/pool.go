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
	// list pool summaries using the ceph client
	cephPoolSummaries, err := ceph.ListPoolSummaries(h.context, h.config.ClusterInfo.Name)
	if err != nil {
		logger.Errorf("failed to list pools: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// get the details for each pool from its summary information
	cephPools := make([]ceph.CephStoragePoolDetails, len(cephPoolSummaries))
	for i := range cephPoolSummaries {
		poolDetails, err := ceph.GetPoolDetails(h.context, h.config.ClusterInfo.Name, cephPoolSummaries[i].Name)
		if err != nil {
			logger.Errorf("%+v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		cephPools[i] = poolDetails
	}

	var ecProfileDetails map[string]ceph.CephErasureCodeProfile
	lookupECProfileDetails := false
	for i := range cephPools {
		if cephPools[i].ErasureCodeProfile != "" {
			// at least one pool is erasure coded, we'll need to look up erasure code profile details
			lookupECProfileDetails = true
			break
		}
	}
	if lookupECProfileDetails {
		// list each erasure code profile
		ecProfileNames, err := ceph.ListErasureCodeProfiles(h.context, h.config.ClusterInfo.Name)
		if err != nil {
			logger.Errorf("failed to list erasure code profiles: %+v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// get the details of each erasure code profile and store them in the map
		ecProfileDetails = make(map[string]ceph.CephErasureCodeProfile, len(ecProfileNames))
		for _, name := range ecProfileNames {
			ecp, err := ceph.GetErasureCodeProfileDetails(h.context, h.config.ClusterInfo.Name, name)
			if err != nil {
				logger.Errorf("failed to get erasure code profile details for '%s': %+v", name, err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			ecProfileDetails[name] = ecp
		}
	}

	// convert the ceph pools details to model pools
	pools := make([]model.Pool, len(cephPools))
	for i, p := range cephPools {
		pool, err := cephPoolToModelPool(p, ecProfileDetails)
		if err != nil {
			logger.Errorf("%+v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		pools[i] = pool
	}

	FormatJsonResponse(w, pools)
}

// Creates a storage pool as specified by the request body.
// POST
// /pool
func (h *Handler) CreatePool(w http.ResponseWriter, r *http.Request) {
	// read/unmarshal the new pool to create from the request body
	var newPoolReq model.Pool
	body, ok := handleReadBody(w, r, "create pool")
	if !ok {
		return
	}

	if err := json.Unmarshal(body, &newPoolReq); err != nil {
		logger.Errorf("failed to unmarshal create pool request body '%s': %+v", string(body), err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	newPool := modelPoolToCephPool(newPoolReq)

	if newPoolReq.Type == model.ErasureCoded {
		// create a new erasure code profile for the new pool
		if err := ceph.CreateErasureCodeProfile(h.context, h.config.ClusterInfo.Name, newPoolReq.ErasureCodedConfig, newPool.ErasureCodeProfile); err != nil {
			logger.Errorf("failed to create erasure code profile for pool '%s': %+v", newPoolReq.Name, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	info, err := ceph.CreatePool(h.context, h.config.ClusterInfo.Name, newPool)
	if err != nil {
		logger.Errorf("failed to create new pool '%s': %+v", newPool.Name, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write([]byte(info))
}

func modelPoolToCephPool(modelPool model.Pool) ceph.CephStoragePoolDetails {
	pool := ceph.CephStoragePoolDetails{
		Name:   modelPool.Name,
		Number: modelPool.Number,
	}

	if modelPool.Type == model.Replicated {
		pool.Size = modelPool.ReplicationConfig.Size
	} else if modelPool.Type == model.ErasureCoded {
		pool.ErasureCodeProfile = fmt.Sprintf("%s_ecprofile", pool.Name)
	}

	return pool
}

func cephPoolToModelPool(cephPool ceph.CephStoragePoolDetails, ecpDetails map[string]ceph.CephErasureCodeProfile) (model.Pool, error) {
	pool := model.Pool{
		Name:   cephPool.Name,
		Number: cephPool.Number,
	}

	if cephPool.ErasureCodeProfile != "" {
		ecpDetails, ok := ecpDetails[cephPool.ErasureCodeProfile]
		if !ok {
			return model.Pool{}, fmt.Errorf("failed to look up erasure code profile details for '%s'", cephPool.ErasureCodeProfile)
		}

		pool.Type = model.ErasureCoded
		pool.ErasureCodedConfig.DataChunkCount = ecpDetails.DataChunkCount
		pool.ErasureCodedConfig.CodingChunkCount = ecpDetails.CodingChunkCount
		pool.ErasureCodedConfig.Algorithm = fmt.Sprintf("%s::%s", ecpDetails.Plugin, ecpDetails.Technique)
	} else if cephPool.Size > 0 {
		pool.Type = model.Replicated
		pool.ReplicationConfig.Size = cephPool.Size
	} else {
		pool.Type = model.PoolTypeUnknown
	}

	return pool, nil
}
