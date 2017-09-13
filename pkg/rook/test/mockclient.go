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
	MockGetNodes                     func() ([]model.Node, error)
	MockGetPools                     func() ([]model.Pool, error)
	MockCreatePool                   func(pool model.Pool) (string, error)
	MockGetBlockImages               func() ([]model.BlockImage, error)
	MockCreateBlockImage             func(image model.BlockImage) (string, error)
	MockDeleteBlockImage             func(image model.BlockImage) (string, error)
	MockGetClientAccessInfo          func() (model.ClientAccessInfo, error)
	MockGetFilesystems               func() ([]model.Filesystem, error)
	MockCreateFilesystem             func(model.FilesystemRequest) (string, error)
	MockDeleteFilesystem             func(model.FilesystemRequest) (string, error)
	MockGetStatusDetails             func() (model.StatusDetails, error)
	MockGetObjectStores              func() ([]model.ObjectStoreResponse, error)
	MockCreateObjectStore            func(store model.ObjectStore) (string, error)
	MockDeleteObjectStore            func(string) error
	MockGetObjectStoreConnectionInfo func() (*model.ObjectStoreConnectInfo, error)
	MockCreateObjectUser             func(model.ObjectUser) (*model.ObjectUser, error)
	MockListBuckets                  func() ([]model.ObjectBucket, error)
	MockGetBucket                    func(string) (*model.ObjectBucket, error)
	MockDeleteBucket                 func(string, bool) error
	MockListObjectUsers              func() ([]model.ObjectUser, error)
	MockGetObjectUser                func(string) (*model.ObjectUser, error)
	MockUpdateObjectUser             func(model.ObjectUser) (*model.ObjectUser, error)
	MockDeleteObjectUser             func(string) error
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

func (m *MockRookRestClient) DeleteBlockImage(image model.BlockImage) (string, error) {
	if m.MockDeleteBlockImage != nil {
		return m.MockDeleteBlockImage(image)
	}

	return "", nil
}

func (m *MockRookRestClient) GetClientAccessInfo() (model.ClientAccessInfo, error) {
	if m.MockGetClientAccessInfo != nil {
		return m.MockGetClientAccessInfo()
	}

	return model.ClientAccessInfo{}, nil
}

func (m *MockRookRestClient) GetFilesystems() ([]model.Filesystem, error) {
	if m.MockGetFilesystems != nil {
		return m.MockGetFilesystems()
	}

	return []model.Filesystem{}, nil
}

func (m *MockRookRestClient) CreateFilesystem(fsr model.FilesystemRequest) (string, error) {
	if m.MockCreateFilesystem != nil {
		return m.MockCreateFilesystem(fsr)
	}

	return "", nil
}

func (m *MockRookRestClient) DeleteFilesystem(fsr model.FilesystemRequest) (string, error) {
	if m.MockDeleteFilesystem != nil {
		return m.MockDeleteFilesystem(fsr)
	}

	return "", nil
}

func (m *MockRookRestClient) GetStatusDetails() (model.StatusDetails, error) {
	if m.MockGetStatusDetails != nil {
		return m.MockGetStatusDetails()
	}

	return model.StatusDetails{}, nil
}

func (m *MockRookRestClient) CreateObjectStore(store model.ObjectStore) (string, error) {
	if m.MockCreateObjectStore != nil {
		return m.MockCreateObjectStore(store)
	}

	return "", nil
}

func (m *MockRookRestClient) DeleteObjectStore(name string) error {
	if m.MockDeleteObjectStore != nil {
		return m.MockDeleteObjectStore(name)
	}

	return nil
}

func (m *MockRookRestClient) GetObjectStores() ([]model.ObjectStoreResponse, error) {
	if m.MockGetObjectStores != nil {
		return m.MockGetObjectStores()
	}

	return nil, nil
}

func (m *MockRookRestClient) GetObjectStoreConnectionInfo() (*model.ObjectStoreConnectInfo, error) {
	if m.MockGetObjectStoreConnectionInfo != nil {
		return m.MockGetObjectStoreConnectionInfo()
	}

	return nil, nil
}

func (m *MockRookRestClient) URL() string {
	return ""
}

func (m *MockRookRestClient) ListBuckets() ([]model.ObjectBucket, error) {
	if m.MockListBuckets != nil {
		return m.MockListBuckets()
	}

	return nil, nil
}

func (m *MockRookRestClient) GetBucket(s string) (*model.ObjectBucket, error) {
	if m.MockGetBucket != nil {
		return m.MockGetBucket(s)
	}

	return nil, nil
}

func (m *MockRookRestClient) DeleteBucket(s string, b bool) error {
	if m.MockDeleteBucket != nil {
		return m.MockDeleteBucket(s, b)
	}

	return nil
}

func (m *MockRookRestClient) ListObjectUsers() ([]model.ObjectUser, error) {
	if m.MockListObjectUsers != nil {
		return m.MockListObjectUsers()
	}

	return nil, nil
}

func (m *MockRookRestClient) GetObjectUser(s string) (*model.ObjectUser, error) {
	if m.MockGetObjectUser != nil {
		return m.MockGetObjectUser(s)
	}

	return nil, nil
}

func (m *MockRookRestClient) CreateObjectUser(u model.ObjectUser) (*model.ObjectUser, error) {
	if m.MockCreateObjectUser != nil {
		return m.MockCreateObjectUser(u)
	}

	return nil, nil
}

func (m *MockRookRestClient) UpdateObjectUser(u model.ObjectUser) (*model.ObjectUser, error) {
	if m.MockUpdateObjectUser != nil {
		return m.MockUpdateObjectUser(u)
	}

	return nil, nil
}

func (m *MockRookRestClient) DeleteObjectUser(s string) error {
	if m.MockDeleteObjectUser != nil {
		return m.MockDeleteObjectUser(s)
	}

	return nil
}
