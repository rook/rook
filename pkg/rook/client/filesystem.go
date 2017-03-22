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
	"net/url"

	"github.com/rook/rook/pkg/model"
)

const (
	filesystemQueryName = "filesystem"
)

func (c *RookNetworkRestClient) GetFilesystems() ([]model.Filesystem, error) {
	body, err := c.DoGet(filesystemQueryName)
	if err != nil {
		return nil, err
	}

	var filesystems []model.Filesystem
	err = json.Unmarshal(body, &filesystems)
	if err != nil {
		return nil, err
	}

	return filesystems, nil
}

func (c *RookNetworkRestClient) CreateFilesystem(newFilesystem model.FilesystemRequest) (string, error) {
	body, err := json.Marshal(newFilesystem)
	if err != nil {
		return "", err
	}

	resp, err := c.DoPost(filesystemQueryName, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	return string(resp), nil
}

func (c *RookNetworkRestClient) DeleteFilesystem(deleteFilesystem model.FilesystemRequest) (string, error) {
	baseURL, err := url.Parse(filesystemQueryName)
	if err != nil {
		return "", err
	}

	params := url.Values{}
	params.Add("name", deleteFilesystem.Name)
	baseURL.RawQuery = params.Encode()

	resp, err := c.DoDelete(baseURL.String())
	if err != nil {
		return "", err
	}

	return string(resp), nil
}
