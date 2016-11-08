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
package test

import (
	"net/http"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/client"
)

type RookMockRestClientFactory struct{}

func (*RookMockRestClientFactory) CreateRookRestClient(url string, httpClient *http.Client) client.RookRestClient {
	return &MockRookRestClient{}
}

// Mock Rook REST Client implementation
type MockRookRestClient struct {
	MockGetNodes             func() ([]model.Node, error)
	MockGetPools             func() ([]model.Pool, error)
	MockCreatePool           func(pool model.Pool) (string, error)
	MockGetBlockImages       func() ([]model.BlockImage, error)
	MockCreateBlockImage     func(image model.BlockImage) (string, error)
	MockGetBlockImageMapInfo func() (model.BlockImageMapInfo, error)
	MockGetStatusDetails     func() (model.StatusDetails, error)
}

func (m *MockRookRestClient) GetNodes() ([]model.Node, error) {
	if m.MockGetNodes != nil {
		return m.MockGetNodes()
	}

	return nil, nil
}

func (m *MockRookRestClient) GetPools() ([]model.Pool, error) {
	if m.MockGetPools != nil {
		return m.MockGetPools()
	}

	return nil, nil
}

func (m *MockRookRestClient) CreatePool(pool model.Pool) (string, error) {
	if m.MockCreatePool != nil {
		return m.MockCreatePool(pool)
	}

	return "", nil
}

func (m *MockRookRestClient) GetBlockImages() ([]model.BlockImage, error) {
	if m.MockGetBlockImages != nil {
		return m.MockGetBlockImages()
	}

	return nil, nil
}

func (m *MockRookRestClient) CreateBlockImage(image model.BlockImage) (string, error) {
	if m.MockCreateBlockImage != nil {
		return m.MockCreateBlockImage(image)
	}

	return "", nil
}

func (m *MockRookRestClient) GetBlockImageMapInfo() (model.BlockImageMapInfo, error) {
	if m.MockGetBlockImageMapInfo != nil {
		return m.MockGetBlockImageMapInfo()
	}

	return model.BlockImageMapInfo{}, nil
}

func (m *MockRookRestClient) GetStatusDetails() (model.StatusDetails, error) {
	if m.MockGetStatusDetails != nil {
		return m.MockGetStatusDetails()
	}

	return model.StatusDetails{}, nil
}

func (m *MockRookRestClient) URL() string {
	return ""
}
