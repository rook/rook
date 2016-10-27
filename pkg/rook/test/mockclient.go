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

func (m *MockRookRestClient) URL() string {
	return ""
}
