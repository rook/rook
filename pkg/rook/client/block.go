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
	imageQueryName       = "image"
	imageRemoveQueryName = "remove"
)

func (c *RookNetworkRestClient) GetBlockImages() ([]model.BlockImage, error) {
	body, err := c.DoGet(imageQueryName)
	if err != nil {
		return nil, err
	}

	var images []model.BlockImage
	err = json.Unmarshal(body, &images)
	if err != nil {
		return nil, err
	}

	return images, nil
}

func (c *RookNetworkRestClient) CreateBlockImage(newImage model.BlockImage) (string, error) {
	return c.handleBlockRequest(newImage, imageQueryName)
}

func (c *RookNetworkRestClient) DeleteBlockImage(image model.BlockImage) (string, error) {
	return c.handleBlockRequest(image, path.Join(imageQueryName, imageRemoveQueryName))
}

func (c *RookNetworkRestClient) handleBlockRequest(i model.BlockImage, queryPath string) (string, error) {
	body, err := json.Marshal(i)
	if err != nil {
		return "", err
	}

	resp, err := c.DoPost(queryPath, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	return string(resp), nil
}
