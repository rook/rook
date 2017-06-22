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

	ceph "github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/ceph/mds"
	"github.com/rook/rook/pkg/model"
)

// Gets a listing of file systems in this cluster.
// GET
// /filesystem
func (h *Handler) GetFileSystems(w http.ResponseWriter, r *http.Request) {
	filesystems, err := ceph.ListFilesystems(h.context, h.config.ClusterInfo.Name)
	if err != nil {
		logger.Errorf("failed to list file systems: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	result := make([]model.Filesystem, len(filesystems))
	for i, fs := range filesystems {
		result[i] = model.Filesystem{
			Name:         fs.Name,
			MetadataPool: fs.MetadataPool,
			DataPools:    fs.DataPools,
		}
	}

	FormatJsonResponse(w, result)
}

// Creates a new file system in this cluster.
// POST
// /filesystem
func (h *Handler) CreateFileSystem(w http.ResponseWriter, r *http.Request) {
	fs, ok := handleReadFilesystemRequest(w, r, "create filesystem")
	if !ok {
		return
	}

	clusterInfo, err := h.config.GetClusterInfo()
	if err != nil {
		logger.Errorf("failed to get cluster info: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	f := mds.NewFS(h.context, fs.Name, fs.PoolName)
	if err := f.CreateFilesystem(clusterInfo); err != nil {
		logger.Errorf("failed to create file system %s: %+v", fs.Name, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := h.config.ClusterHandler.StartFileSystem(fs); err != nil {
		logger.Errorf("failed to start mds: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Removes an existing filesystem from this cluster.
// DELETE
// /filesystem?name=<fsName>
func (h *Handler) RemoveFileSystem(w http.ResponseWriter, r *http.Request) {
	fsName := r.URL.Query().Get("name")
	fs := &model.FilesystemRequest{
		Name: fsName,
	}

	if fs.Name == "" {
		logger.Errorf("filesystem missing required fields: %+v", fs)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := h.config.ClusterHandler.RemoveFileSystem(fs); err != nil {
		logger.Errorf("failed to remove file system: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	logger.Debugf("started async deletion of file system")
	w.WriteHeader(http.StatusAccepted)
}

func handleReadFilesystemRequest(w http.ResponseWriter, r *http.Request, handlerName string) (*model.FilesystemRequest, bool) {
	body, ok := handleReadBody(w, r, handlerName)
	if !ok {
		return nil, false
	}

	var fsr model.FilesystemRequest
	if err := json.Unmarshal(body, &fsr); err != nil {
		logger.Errorf("failed to unmarshal filesystem request body '%s': %+v", string(body), err)
		w.WriteHeader(http.StatusBadRequest)
		return nil, false
	}

	if fsr.Name == "" {
		logger.Errorf("missing filesystem name: %+v", fsr)
		w.WriteHeader(http.StatusBadRequest)
		return nil, false
	}

	return &fsr, true
}
