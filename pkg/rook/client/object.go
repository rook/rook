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
package client

import (
	"encoding/json"
	"net/http"
	"path"

	"bytes"
	"fmt"

	"github.com/rook/rook/pkg/model"
)

const (
	objectStoreQueryName    = "objectstore"
	connectionInfoQueryName = "connectioninfo"
	bucketsQueryName        = "buckets"
	bucketACLQueryName      = "acl"
	usersQueryName          = "users"
)

func (c *RookNetworkRestClient) GetObjectStores() ([]model.ObjectStoreResponse, error) {
	body, err := c.DoGet(path.Join(objectStoreQueryName))
	if err != nil {
		return nil, err
	}

	var stores []model.ObjectStoreResponse
	err = json.Unmarshal(body, &stores)
	if err != nil {
		return nil, err
	}

	return stores, nil
}

func (c *RookNetworkRestClient) CreateObjectStore(store model.ObjectStore) (string, error) {
	if store.Name == "" {
		return "", fmt.Errorf("Name is required")
	}

	body, err := json.Marshal(store)
	if err != nil {
		return "", err
	}

	resp, err := c.DoPost(objectStoreQueryName, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	return string(resp), nil
}

func (c *RookNetworkRestClient) DeleteObjectStore(storeName string) error {
	query := path.Join(objectStoreQueryName, storeName)
	_, err := c.DoDelete(query)
	return err
}

func (c *RookNetworkRestClient) GetObjectStoreConnectionInfo(storeName string) (*model.ObjectStoreConnectInfo, error) {
	body, err := c.DoGet(path.Join(objectStoreQueryName, storeName, connectionInfoQueryName))
	if err != nil {
		return nil, err
	}

	var connInfo model.ObjectStoreConnectInfo
	err = json.Unmarshal(body, &connInfo)
	if err != nil {
		return nil, err
	}

	return &connInfo, nil
}

func (c *RookNetworkRestClient) ListBuckets(storeName string) ([]model.ObjectBucket, error) {
	body, err := c.DoGet(path.Join(objectStoreQueryName, storeName, bucketsQueryName))
	if err != nil {
		return nil, err
	}

	var buckets []model.ObjectBucket
	err = json.Unmarshal(body, &buckets)
	if err != nil {
		return nil, err
	}

	return buckets, nil
}

func (c *RookNetworkRestClient) GetBucket(storeName, bucketName string) (*model.ObjectBucket, error) {
	body, err := c.DoGet(path.Join(objectStoreQueryName, storeName, bucketsQueryName, bucketName))
	if err != nil {
		return nil, err
	}

	var bucket model.ObjectBucket
	err = json.Unmarshal(body, &bucket)
	if err != nil {
		return nil, err
	}

	return &bucket, nil
}

func (c *RookNetworkRestClient) DeleteBucket(storeName, bucketName string, purge bool) error {
	query := path.Join(objectStoreQueryName, storeName, bucketsQueryName, bucketName)
	if purge {
		query += "?purge=true"
	}

	_, err := c.DoDelete(query)
	if err != nil && !IsHttpStatusCode(err, http.StatusNoContent) {
		return err
	}

	return nil
}

func (c *RookNetworkRestClient) ListObjectUsers(storeName string) ([]model.ObjectUser, error) {
	body, err := c.DoGet(path.Join(objectStoreQueryName, storeName, usersQueryName))
	if err != nil {
		return nil, err
	}

	var users []model.ObjectUser
	err = json.Unmarshal(body, &users)
	if err != nil {
		return nil, err
	}

	return users, nil
}

func (c *RookNetworkRestClient) GetObjectUser(storeName, id string) (*model.ObjectUser, error) {
	body, err := c.DoGet(path.Join(objectStoreQueryName, storeName, usersQueryName, id))
	if err != nil {
		return nil, err
	}

	var user model.ObjectUser
	err = json.Unmarshal(body, &user)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (c *RookNetworkRestClient) CreateObjectUser(storeName string, user model.ObjectUser) (*model.ObjectUser, error) {
	if user.DisplayName == nil {
		return nil, fmt.Errorf("Display name is required")
	}

	body, err := json.Marshal(user)
	if err != nil {
		return nil, err
	}

	respBody, err := c.DoPost(path.Join(objectStoreQueryName, storeName, usersQueryName), bytes.NewReader(body))
	if err != nil && !IsHttpStatusCode(err, http.StatusCreated) {
		return nil, err
	}

	var createdUser model.ObjectUser
	err = json.Unmarshal(respBody, &createdUser)
	if err != nil {
		return nil, err
	}

	return &createdUser, nil
}

func (c *RookNetworkRestClient) UpdateObjectUser(storeName string, user model.ObjectUser) (*model.ObjectUser, error) {
	body, err := json.Marshal(user)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: %+v", err)
	}

	respBody, err := c.DoPut(path.Join(objectStoreQueryName, storeName, usersQueryName, user.UserID), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(respBody, &user)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %+v", err)
	}

	return &user, nil
}

func (c *RookNetworkRestClient) DeleteObjectUser(storeName, id string) error {
	query := path.Join(objectStoreQueryName, storeName, usersQueryName, id)
	_, err := c.DoDelete(query)
	if err != nil && !IsHttpStatusCode(err, http.StatusNoContent) {
		return err
	}

	return nil
}
