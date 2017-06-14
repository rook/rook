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

// Gets the images that have been created in this cluster.
// GET
// /image
func (h *Handler) GetImages(w http.ResponseWriter, r *http.Request) {
	// first list all the pools so that we can retrieve images from all pools
	pools, err := ceph.ListPoolSummaries(h.context, h.config.ClusterInfo.Name)
	if err != nil {
		logger.Errorf("failed to list pools: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	result := []model.BlockImage{}

	// for each pool, get further details about all the images in the pool
	for _, p := range pools {
		images, ok := h.getImagesForPool(w, p.Name)
		if !ok {
			return
		}

		result = append(result, images...)
	}

	FormatJsonResponse(w, result)
}

func (h *Handler) getImagesForPool(w http.ResponseWriter, poolName string) ([]model.BlockImage, bool) {
	cephImages, err := ceph.ListImages(h.context, h.config.ClusterInfo.Name, poolName)
	if err != nil {
		logger.Errorf("failed to get images from pool %s: %+v", poolName, err)
		w.WriteHeader(http.StatusInternalServerError)
		return nil, false
	}

	images := make([]model.BlockImage, len(cephImages))
	for i, image := range cephImages {
		// add the current image's details to the result set
		images[i] = model.BlockImage{
			Name:     image.Name,
			PoolName: poolName,
			Size:     image.Size,
		}
	}

	return images, true
}

// Creates a new image in this cluster.
// POST
// /image
func (h *Handler) CreateImage(w http.ResponseWriter, r *http.Request) {
	var newImage model.BlockImage
	body, ok := handleReadBody(w, r, "create image")
	if !ok {
		return
	}

	if err := json.Unmarshal(body, &newImage); err != nil {
		logger.Errorf("failed to unmarshal create image request body '%s': %+v", string(body), err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if newImage.Name == "" || newImage.PoolName == "" || newImage.Size == 0 {
		logger.Errorf("image missing required fields: %+v", newImage)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	createdImage, err := ceph.CreateImage(h.context, h.config.ClusterInfo.Name, newImage.Name,
		newImage.PoolName, newImage.Size)
	if err != nil {
		logger.Errorf("failed to create image %+v: %+v", newImage, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write([]byte(fmt.Sprintf("succeeded created image %s", createdImage.Name)))
}

// Deletes a block image from this cluster.
// DELETE
// /image?name=<imageName>&pool=<imagePool>
func (h *Handler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	imageName := r.URL.Query().Get("name")
	imagePool := r.URL.Query().Get("pool")

	deleteImageReq := model.BlockImage{
		Name:     imageName,
		PoolName: imagePool,
	}

	if deleteImageReq.Name == "" || deleteImageReq.PoolName == "" {
		logger.Errorf("image missing required fields: %+v", deleteImageReq)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := ceph.DeleteImage(h.context, h.config.ClusterInfo.Name, deleteImageReq.Name, deleteImageReq.PoolName)
	if err != nil {
		logger.Errorf("failed to delete image %+v: %+v", deleteImageReq, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write([]byte(fmt.Sprintf("succeeded deleting image %s", deleteImageReq.Name)))
}
