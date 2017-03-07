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

func (c *RookNetworkRestClient) CreateObjectStore() (string, error) {
	resp, err := c.DoPost(objectStoreQueryName, nil)
	if err != nil {
		return "", err
	}

	return string(resp), nil
}

func (c *RookNetworkRestClient) GetObjectStoreConnectionInfo() (*model.ObjectStoreConnectInfo, error) {
	body, err := c.DoGet(path.Join(objectStoreQueryName, connectionInfoQueryName))
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

func (c *RookNetworkRestClient) ListBuckets() ([]model.ObjectBucket, error) {
	body, err := c.DoGet(path.Join(objectStoreQueryName, bucketsQueryName))
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

func (c *RookNetworkRestClient) GetBucket(bucketName string) (*model.ObjectBucket, error) {
	body, err := c.DoGet(path.Join(objectStoreQueryName, bucketsQueryName, bucketName))
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

func (c *RookNetworkRestClient) DeleteBucket(bucketName string, purge bool) error {
	query := path.Join(objectStoreQueryName, bucketsQueryName, bucketName)
	if purge {
		query += "?purge=true"
	}

	_, err := c.DoDelete(query)
	if err != nil && !IsHttpStatusCode(err, http.StatusNoContent) {
		return err
	}

	return nil
}

func (c *RookNetworkRestClient) ListObjectUsers() ([]model.ObjectUser, error) {
	body, err := c.DoGet(path.Join(objectStoreQueryName, usersQueryName))
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

func (c *RookNetworkRestClient) GetObjectUser(id string) (*model.ObjectUser, error) {
	body, err := c.DoGet(path.Join(objectStoreQueryName, usersQueryName, id))
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

func (c *RookNetworkRestClient) CreateObjectUser(user model.ObjectUser) (*model.ObjectUser, error) {
	if user.DisplayName == nil {
		return nil, fmt.Errorf("Display name is required")
	}

	body, err := json.Marshal(user)
	if err != nil {
		return nil, err
	}

	respBody, err := c.DoPost(path.Join(objectStoreQueryName, usersQueryName), bytes.NewReader(body))
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

func (c *RookNetworkRestClient) UpdateObjectUser(user model.ObjectUser) (*model.ObjectUser, error) {
	body, err := json.Marshal(user)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: %+v", err)
	}

	respBody, err := c.DoPut(path.Join(objectStoreQueryName, usersQueryName, user.UserID), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(respBody, &user)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %+v", err)
	}

	return &user, nil
}

func (c *RookNetworkRestClient) DeleteObjectUser(id string) error {
	query := path.Join(objectStoreQueryName, usersQueryName, id)
	_, err := c.DoDelete(query)
	if err != nil && !IsHttpStatusCode(err, http.StatusNoContent) {
		return err
	}

	return nil
}
