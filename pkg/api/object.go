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
)

// Creates a new object store in this cluster.
// POST
// /object
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
// /object/remove
func (h *Handler) RemoveObjectStore(w http.ResponseWriter, r *http.Request) {
	if err := rgw.RemoveObjectStore(h.context); err != nil {
		logger.Errorf("failed to remove object store: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	logger.Debugf("started async deletion of the object store")
	w.WriteHeader(http.StatusAccepted)
}
