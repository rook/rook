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
	"path"

	"bytes"
	"fmt"

	"github.com/rook/rook/pkg/model"
)

const (
	objectStoreQueryName    = "objectstore"
	connectionInfoQueryName = "connectioninfo"
	bucketsQueryName        = "buckets"
	usersQueryName          = "users"
)

func (c *RookNetworkRestClient) CreateObjectStore() (string, error) {
	resp, err := c.DoPost(objectStoreQueryName, nil)
	if err != nil {
		return "", err
	}

	return string(resp), nil
}

func (c *RookNetworkRestClient) GetObjectStoreConnectionInfo() (model.ObjectStoreS3Info, error) {
	body, err := c.DoGet(path.Join(objectStoreQueryName, connectionInfoQueryName))
	if err != nil {
		return model.ObjectStoreS3Info{}, err
	}

	var connInfo model.ObjectStoreS3Info
	err = json.Unmarshal(body, &connInfo)
	if err != nil {
		return model.ObjectStoreS3Info{}, err
	}

	return connInfo, nil
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

func (c *RookNetworkRestClient) GetObjectUser(id string) (model.ObjectUser, error) {
	body, err := c.DoGet(path.Join(objectStoreQueryName, usersQueryName, id))
	if err != nil {
		return model.ObjectUser{}, err
	}

	var user model.ObjectUser
	err = json.Unmarshal(body, &user)
	if err != nil {
		return model.ObjectUser{}, err
	}

	return user, nil
}

func (c *RookNetworkRestClient) CreateObjectUser(user model.ObjectUser) (model.ObjectUser, error) {
	if user.DisplayName == nil {
		return model.ObjectUser{}, fmt.Errorf("Display name is required")
	}

	body, err := json.Marshal(user)
	if err != nil {
		return model.ObjectUser{}, err
	}

	respBody, err := c.DoPost(path.Join(objectStoreQueryName, usersQueryName), bytes.NewReader(body))
	if err != nil {
		return model.ObjectUser{}, err
	}

	err = json.Unmarshal(respBody, &user)
	if err != nil {
		return model.ObjectUser{}, err
	}

	return user, nil
}

func (c *RookNetworkRestClient) UpdateObjectUser(user model.ObjectUser) (model.ObjectUser, error) {
	body, err := json.Marshal(user)
	if err != nil {
		return model.ObjectUser{}, fmt.Errorf("failed to marshal: %+v", err)
	}

	respBody, err := c.DoPut(path.Join(objectStoreQueryName, usersQueryName, user.UserID), bytes.NewReader(body))
	if err != nil {
		return model.ObjectUser{}, err
	}

	err = json.Unmarshal(respBody, &user)
	if err != nil {
		return model.ObjectUser{}, fmt.Errorf("failed to unmarshal: %+v", err)
	}

	return user, nil
}

func (c *RookNetworkRestClient) DeleteObjectUser(id string) error {
	query := path.Join(objectStoreQueryName, usersQueryName, id)
	_, err := c.DoDelete(query)
	if err != nil {
		return err
	}

	return nil
}
