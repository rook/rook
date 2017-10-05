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
	"sort"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/gorilla/mux"
	"github.com/rook/rook/pkg/ceph/rgw"
	"github.com/rook/rook/pkg/model"
	k8srgw "github.com/rook/rook/pkg/operator/rgw"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultObjectStoreName = "default"
	defaultRGWInstances    = 1
)

func (h *Handler) objectContext(r *http.Request) *rgw.Context {
	storeName := defaultObjectStoreName
	if name, ok := mux.Vars(r)["name"]; ok {
		storeName = name
	}

	return rgw.NewContext(h.context, storeName, h.config.clusterInfo.Name)
}

// GetObjectStores gets the object stores in this cluster.
// GET
// /objectstore
func (h *Handler) GetObjectStores(w http.ResponseWriter, r *http.Request) {
	stores := []model.ObjectStoreResponse{}

	// require both the realm and k8s service to exist to consider an object store available
	realms, err := rgw.GetObjectStores(rgw.NewContext(h.config.context, "", h.config.clusterInfo.Name))
	if err != nil {
		logger.Errorf("failed to get rgw realms. %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// get the rgw service for each realm
	for _, realm := range realms {
		service, err := h.config.context.Clientset.CoreV1().Services(h.config.clusterInfo.Name).Get(k8srgw.InstanceName(realm), metav1.GetOptions{})
		if err != nil {
			logger.Warningf("RGW realm found, but no k8s service found for %s", realm)
			continue
		}
		stores = append(stores, model.ObjectStoreResponse{
			Name:        realm,
			Ports:       service.Spec.Ports,
			ClusterIP:   service.Spec.ClusterIP,
			ExternalIPs: service.Spec.ExternalIPs,
		})
	}

	if err != nil {
		logger.Errorf("failed to get object stores: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	FormatJsonResponse(w, stores)
}

// CreateObjectStore creates a new object store in this cluster.
// POST
// /objectstore
func (h *Handler) CreateObjectStore(w http.ResponseWriter, r *http.Request) {
	var objectStore model.ObjectStore
	err := json.NewDecoder(r.Body).Decode(&objectStore)
	if err != nil {
		logger.Errorf("Error parsing object store settings: %+v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if objectStore.Gateway.Port == 0 && objectStore.Gateway.SecurePort == 0 {
		logger.Errorf("Must specify port or securePort")
		w.WriteHeader(http.StatusBadRequest)
	}

	// only the default store is supported through the rest api
	if objectStore.Name == "" {
		objectStore.Name = defaultObjectStoreName
	}
	if objectStore.Gateway.Instances == 0 {
		objectStore.Gateway.Instances = defaultRGWInstances
	}

	if err := enableObjectStore(h.config, objectStore); err != nil {
		logger.Errorf("failed to create object store: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	logger.Debugf("created object store")
}

// RemoveObjectStore removes the object store from this cluster.
// DELETE
// /objectstore/{name}
func (h *Handler) RemoveObjectStore(w http.ResponseWriter, r *http.Request) {
	storeName := mux.Vars(r)["name"]

	store := &k8srgw.ObjectStore{ObjectMeta: metav1.ObjectMeta{Name: storeName, Namespace: h.config.clusterInfo.Name}}
	if err := store.Delete(h.config.context); err != nil {
		logger.Errorf("failed to remove object store: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	logger.Debugf("started async deletion of the object store")
}

// GetObjectStoreConnectionInfo gets connection information to the object store in this cluster.
// GET
// /objectstore/{name}/connectioninfo
func (h *Handler) GetObjectStoreConnectionInfo(w http.ResponseWriter, r *http.Request) {
	storeName := mux.Vars(r)["name"]

	n := k8srgw.InstanceName(storeName)
	logger.Infof("Getting the object store connection info for %s", n)
	service, err := h.config.context.Clientset.CoreV1().Services(h.config.namespace).Get(n, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		logger.Errorf("did not find rgw service %s. %+v", storeName, err)
		return
	}

	s3Info := &model.ObjectStoreConnectInfo{
		Host:      k8srgw.InstanceName(storeName),
		IPAddress: service.Spec.ClusterIP,
		Ports:     []int32{},
	}

	// append all of the ports
	for _, port := range service.Spec.Ports {
		s3Info.Ports = append(s3Info.Ports, port.Port)
	}

	FormatJsonResponse(w, s3Info)
}

// ListUsers lists the users of the object store in this cluster.
// GET
// /objectstore/{name}/users
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	userNames, _, err := rgw.ListUsers(h.objectContext(r))
	if err != nil {
		logger.Errorf("Error listing users: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	users := []model.ObjectUser{}
	for _, userName := range userNames {
		user, _, err := rgw.GetUser(h.objectContext(r), userName)
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
// /objectstore/{name}/users/{USER_ID}
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	user, rgwError, err := rgw.GetUser(h.objectContext(r), id)
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
// /objectstore/{name}/users
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var user model.ObjectUser

	err := json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		logger.Errorf("Error parsing user: %+v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	createdUser, rgwError, err := rgw.CreateUser(h.objectContext(r), user)
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
// /objectstore/{name}/users/{USER_ID}
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

	updatedUser, rgwError, err := rgw.UpdateUser(h.objectContext(r), user)
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
// /objectstore/{name}/users/{USER_ID}
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	_, rgwError, err := rgw.DeleteUser(h.objectContext(r), id)
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
// /objectstore/{name}/buckets
func (h *Handler) ListBuckets(w http.ResponseWriter, r *http.Request) {
	buckets, err := rgw.ListBuckets(h.objectContext(r))
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
// /objectstore/{name}/buckets/{BUCKET_NAME}
func (h *Handler) GetBucket(w http.ResponseWriter, r *http.Request) {
	bucketName := mux.Vars(r)["bucketName"]

	user, rgwError, err := rgw.GetBucket(h.objectContext(r), bucketName)
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
// /objectstore/{name}/buckets/{BUCKET_NAME}
func (h *Handler) DeleteBucket(w http.ResponseWriter, r *http.Request) {
	bucketName := mux.Vars(r)["bucketName"]

	purgeParams, found := r.URL.Query()["purge"]
	purge := found && len(purgeParams) == 1 && purgeParams[0] == "true"

	rgwError, err := rgw.DeleteBucket(h.objectContext(r), bucketName, purge)
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

func enableObjectStore(c *Config, config model.ObjectStore) error {
	logger.Infof("Starting the Object store")

	// save the certificate in a secret if we weren't given a reference to a secret
	if config.Gateway.Certificate != "" && config.Gateway.CertificateRef == "" {
		certName := fmt.Sprintf("rook-rgw-%s-cert", config.Name)
		config.Gateway.CertificateRef = certName

		data := map[string][]byte{"cert": []byte(config.Gateway.Certificate)}
		certSecret := &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: certName, Namespace: c.namespace}, Data: data}

		_, err := c.context.Clientset.Core().Secrets(c.namespace).Create(certSecret)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create cert secret. %+v", err)
			}
			if _, err := c.context.Clientset.Core().Secrets(c.namespace).Update(certSecret); err != nil {
				return fmt.Errorf("failed to update secret. %+v", err)
			}
			logger.Infof("updated the certificate secret %s", certName)
		}
	}

	store := k8srgw.ModelToSpec(config, c.namespace)
	err := store.Update(c.context, c.versionTag, c.hostNetwork)
	if err != nil {
		return fmt.Errorf("failed to start rgw. %+v", err)
	}
	return nil
}
