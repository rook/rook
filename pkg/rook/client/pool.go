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
	"bytes"
	"encoding/json"
	"path"

	"github.com/rook/rook/pkg/model"
)

const (
	poolQueryName = "pool"
)

func (c *RookNetworkRestClient) GetPools() ([]model.Pool, error) {
	body, err := c.DoGet(poolQueryName)
	if err != nil {
		return nil, err
	}

	var pools []model.Pool
	err = json.Unmarshal(body, &pools)
	if err != nil {
		return nil, err
	}

	return pools, nil
}

func (c *RookNetworkRestClient) CreatePool(newPool model.Pool) (string, error) {
	body, err := json.Marshal(newPool)
	if err != nil {
		return "", err
	}

	resp, err := c.DoPost(poolQueryName, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	return string(resp), nil
}

func (c *RookNetworkRestClient) DeletePool(name string) error {

	_, err := c.DoDelete(path.Join(poolQueryName, name))
	if err != nil {
		return err
	}

	return nil
}
