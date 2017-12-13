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

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	ceph "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/model"
	k8smds "github.com/rook/rook/pkg/operator/mds"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Gets a listing of file systems in this cluster.
// GET
// /filesystem
func (h *Handler) GetFileSystems(w http.ResponseWriter, r *http.Request) {
	filesystems, err := ceph.ListFilesystems(h.context, h.config.namespace)
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

	logger.Infof("Starting the MDS")
	if err := k8smds.CreateFileSystem(h.config.context, h.config.namespace, fs, h.config.rookImage, h.config.hostNetwork); err != nil {
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

	f := rookalpha.Filesystem{ObjectMeta: metav1.ObjectMeta{Name: fs.Name, Namespace: h.config.namespace}}
	if err := k8smds.DeleteFilesystem(h.config.context, f); err != nil {
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
