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
	"log"
	"net/http"

	ceph "github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/model"
)

// Gets the images that have been created in this cluster.
// GET
// /image
func (h *Handler) GetImages(w http.ResponseWriter, r *http.Request) {
	adminConn, ok := h.handleConnectToCeph(w)
	if !ok {
		return
	}
	defer adminConn.Shutdown()

	// first list all the pools so that we can retrieve images from all pools
	pools, err := ceph.ListPoolSummaries(adminConn)
	if err != nil {
		log.Printf("failed to list pools: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	result := []model.BlockImage{}

	// for each pool, open an IO context to get further details about all the images in the pool
	for _, p := range pools {
		ioctx, ok := handleOpenIOContext(w, adminConn, p.Name)
		if !ok {
			return
		}

		// get all the image names for the current pool
		imageNames, err := ioctx.GetImageNames()
		if err != nil {
			log.Printf("failed to get image names from pool %s: %+v", p.Name, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// for each image name, open the image and stat it for further details
		images := make([]model.BlockImage, len(imageNames))
		for i, name := range imageNames {
			image := ioctx.GetImage(name)
			image.Open(true)
			defer image.Close()
			imageStat, err := image.Stat()
			if err != nil {
				log.Printf("failed to stat image %s from pool %s: %+v", name, p.Name, err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			// add the current image's details to the result set
			images[i] = model.BlockImage{
				Name:     name,
				PoolName: p.Name,
				Size:     imageStat.Size,
			}
		}

		result = append(result, images...)
	}

	FormatJsonResponse(w, result)
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
		log.Printf("failed to unmarshal create image request body '%s': %+v", string(body), err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if newImage.Name == "" || newImage.PoolName == "" || newImage.Size == 0 {
		log.Printf("image missing required fields: %+v", newImage)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	adminConn, ok := h.handleConnectToCeph(w)
	if !ok {
		return
	}
	defer adminConn.Shutdown()

	ioctx, ok := handleOpenIOContext(w, adminConn, newImage.PoolName)
	if !ok {
		return
	}

	createdImage, err := ioctx.CreateImage(newImage.Name, newImage.Size, 22)
	if err != nil {
		log.Printf("failed to create image %+v: %+v", newImage, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write([]byte(fmt.Sprintf("succeeded created image %s", createdImage.Name())))
}

// Gets information needed to map an image to a local device
// GET
// /image/mapinfo
func (h *Handler) GetImageMapInfo(w http.ResponseWriter, r *http.Request) {
	// TODO: auth is extremely important here because we are returning cephx credentials

	adminConn, ok := h.handleConnectToCeph(w)
	if !ok {
		return
	}
	defer adminConn.Shutdown()

	monStatus, err := ceph.GetMonStatus(adminConn)
	if err != nil {
		log.Printf("failed to get monitor status, err: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// TODO: don't always return admin creds
	entity := "client.admin"
	user := "admin"
	secret, err := ceph.AuthGetKey(adminConn, entity)
	if err != nil {
		log.Printf("failed to get key for %s: %+v", entity, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	monAddrs := make([]string, len(monStatus.MonMap.Mons))
	for i, m := range monStatus.MonMap.Mons {
		monAddrs[i] = m.Address
	}

	mapInfo := model.BlockImageMapInfo{
		MonAddresses: monAddrs,
		UserName:     user,
		SecretKey:    secret,
	}

	FormatJsonResponse(w, mapInfo)
}
