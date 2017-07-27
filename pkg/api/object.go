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
	"sort"

	"github.com/gorilla/mux"
	"github.com/rook/rook/pkg/ceph/rgw"
	"github.com/rook/rook/pkg/model"
)

// CreateObjectStore creates a new object store in this cluster.
// POST
// /objectstore
func (h *Handler) CreateObjectStore(w http.ResponseWriter, r *http.Request) {

	if err := h.config.ClusterHandler.EnableObjectStore(); err != nil {
		logger.Errorf("failed to create object store: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	logger.Debugf("started async creation of the object store")
	w.WriteHeader(http.StatusAccepted)
}

// RemoveObjectStore removes the object store from this cluster.
// DELETE
// /objectstore
func (h *Handler) RemoveObjectStore(w http.ResponseWriter, r *http.Request) {
	if err := h.config.ClusterHandler.RemoveObjectStore(); err != nil {
		logger.Errorf("failed to remove object store: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	logger.Debugf("started async deletion of the object store")
	w.WriteHeader(http.StatusAccepted)
}

// GetObjectStoreConnectionInfo gets connection information to the object store in this cluster.
// GET
// /objectstore/connectioninfo
func (h *Handler) GetObjectStoreConnectionInfo(w http.ResponseWriter, r *http.Request) {

	s3Info, found, err := h.config.ClusterHandler.GetObjectStoreConnectionInfo()
	if err != nil {
		logger.Errorf("failed get object store info. %+v", err)
		if found {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
		return
	}

	FormatJsonResponse(w, s3Info)
}

// ListUsers lists the users of the object store in this cluster.
// GET
// /objectstore/users
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	userNames, _, err := rgw.ListUsers(h.context, h.config.ClusterHandler.GetClusterInfo)
	if err != nil {
		logger.Errorf("Error listing users: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	users := []model.ObjectUser{}
	for _, userName := range userNames {
		user, _, err := rgw.GetUser(h.context, userName, h.config.ClusterHandler.GetClusterInfo)
		if err != nil {
			logger.Errorf("Error listing users: %+v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		users = append(users, *user)
	}

	FormatJsonResponse(w, users)
}

// GetUser gets the passed users info from the object store in this cluster.
// GET
// /objectstore/users/{USER_ID}
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	user, rgwError, err := rgw.GetUser(h.context, id, h.config.ClusterHandler.GetClusterInfo)
	if err != nil {
		logger.Errorf("Error getting user (%s): %+v", id, err)

		if rgwError == rgw.RGWErrorNotFound {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	FormatJsonResponse(w, user)
}

// CreateUser will create a new user from the passed info in the object store in this cluster.
// POST
// /objectstore/users
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var user model.ObjectUser

	err := json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		logger.Errorf("Error parsing user: %+v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	createdUser, rgwError, err := rgw.CreateUser(h.context, user, h.config.ClusterHandler.GetClusterInfo)
	if err != nil {
		logger.Errorf("Error creating user: %+v", err)

		if rgwError == rgw.RGWErrorBadData {
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Write([]byte(err.Error()))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusCreated)
	FormatJsonResponse(w, *createdUser)
}

// UpdateUser updates the passed user with the passed info for the object store in this cluster.
// PUT
// /objectstore/users/{USER_ID}
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var user model.ObjectUser
	err := json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		logger.Errorf("Error parsing user: %+v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	user.UserID = id

	updatedUser, rgwError, err := rgw.UpdateUser(h.context, user, h.config.ClusterHandler.GetClusterInfo)
	if err != nil {
		logger.Errorf("Error updating user: %+v", err)

		if rgwError == rgw.RGWErrorNotFound {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	FormatJsonResponse(w, updatedUser)
}

// DeleteUser deletes the passed user for the object store in this cluster.
// DELETE
// /objectstore/users/{USER_ID}
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	_, rgwError, err := rgw.DeleteUser(h.context, id, h.config.ClusterHandler.GetClusterInfo)
	if err != nil {
		if rgwError == rgw.RGWErrorNotFound {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		logger.Errorf("Error deleting user (%s): %+v", id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Listbuckets lists the buckets in the object store in this cluster.
// GET
// /objectstore/buckets
func (h *Handler) ListBuckets(w http.ResponseWriter, r *http.Request) {
	buckets, err := rgw.ListBuckets(h.context, h.config.ClusterHandler.GetClusterInfo)
	if err != nil {
		logger.Errorf("Error listing buckets: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	sortedBuckets := model.ObjectBuckets(buckets)
	sort.Sort(sortedBuckets)

	FormatJsonResponse(w, sortedBuckets)
}

// GetBucket gets the bucket from the object store in this cluster.
// GET
// /objectstore/buckets/{BUCKET_NAME}
func (h *Handler) GetBucket(w http.ResponseWriter, r *http.Request) {
	bucketName := mux.Vars(r)["bucketName"]

	user, rgwError, err := rgw.GetBucket(h.context, bucketName, h.config.ClusterHandler.GetClusterInfo)
	if err != nil {
		logger.Errorf("Error getting bucket (%s): %+v", bucketName, err)

		if rgwError == rgw.RGWErrorNotFound {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	FormatJsonResponse(w, user)
}

// DeleteBucket deletes the bucket in the object store in this cluster.
// DELETE
// /objectstore/buckets/{BUCKET_NAME}
func (h *Handler) DeleteBucket(w http.ResponseWriter, r *http.Request) {
	bucketName := mux.Vars(r)["bucketName"]

	purgeParams, found := r.URL.Query()["purge"]
	purge := found && len(purgeParams) == 1 && purgeParams[0] == "true"

	rgwError, err := rgw.DeleteBucket(h.context, bucketName, purge, h.config.ClusterHandler.GetClusterInfo)
	if err != nil {
		if rgwError == rgw.RGWErrorNotFound {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		logger.Errorf("Error deleting bucket: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
