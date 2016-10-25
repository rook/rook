package test

import (
	"net/http"

	"github.com/rook/rook/pkg/castle/client"
	"github.com/rook/rook/pkg/model"
)

type CastleMockRestClientFactory struct{}

func (*CastleMockRestClientFactory) CreateCastleRestClient(url string, httpClient *http.Client) client.CastleRestClient {
	return &MockCastleRestClient{}
}

// Mock Castle REST Client implementation
type MockCastleRestClient struct {
	MockGetNodes             func() ([]model.Node, error)
	MockGetPools             func() ([]model.Pool, error)
	MockCreatePool           func(pool model.Pool) (string, error)
	MockGetBlockImages       func() ([]model.BlockImage, error)
	MockCreateBlockImage     func(image model.BlockImage) (string, error)
	MockGetBlockImageMapInfo func() (model.BlockImageMapInfo, error)
}

func (m *MockCastleRestClient) GetNodes() ([]model.Node, error) {
	if m.MockGetNodes != nil {
		return m.MockGetNodes()
	}

	return nil, nil
}

func (m *MockCastleRestClient) GetPools() ([]model.Pool, error) {
	if m.MockGetPools != nil {
		return m.MockGetPools()
	}

	return nil, nil
}

func (m *MockCastleRestClient) CreatePool(pool model.Pool) (string, error) {
	if m.MockCreatePool != nil {
		return m.MockCreatePool(pool)
	}

	return "", nil
}

func (m *MockCastleRestClient) GetBlockImages() ([]model.BlockImage, error) {
	if m.MockGetBlockImages != nil {
		return m.MockGetBlockImages()
	}

	return nil, nil
}

func (m *MockCastleRestClient) CreateBlockImage(image model.BlockImage) (string, error) {
	if m.MockCreateBlockImage != nil {
		return m.MockCreateBlockImage(image)
	}

	return "", nil
}

func (m *MockCastleRestClient) GetBlockImageMapInfo() (model.BlockImageMapInfo, error) {
	if m.MockGetBlockImageMapInfo != nil {
		return m.MockGetBlockImageMapInfo()
	}

	return model.BlockImageMapInfo{}, nil
}

func (m *MockCastleRestClient) URL() string {
	return ""
}
