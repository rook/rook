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

	ceph "github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/model"
)

// Gets the information needed for a client to access the cluster.
// GET
// /client
func (h *Handler) GetClientAccessInfo(w http.ResponseWriter, r *http.Request) {
	// TODO: auth is extremely important here because we are returning cephx credentials
	// https://github.com/rook/rook/issues/209

	monStatus, err := ceph.GetMonStatus(h.context, h.config.ClusterInfo.Name)
	if err != nil {
		logger.Errorf("failed to get monitor status, err: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// TODO: don't always return admin creds
	entity := ceph.AdminUsername
	secret, err := ceph.AuthGetKey(h.context, h.config.ClusterInfo.Name, entity)
	if err != nil {
		logger.Errorf("failed to get key for %s: %+v", entity, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	monAddrs := make([]string, len(monStatus.MonMap.Mons))
	for i, m := range monStatus.MonMap.Mons {
		monAddrs[i] = m.Address
	}

	clientAccessInfo := model.ClientAccessInfo{
		MonAddresses: monAddrs,
		UserName:     "admin",
		SecretKey:    secret,
	}

	FormatJsonResponse(w, clientAccessInfo)
}
