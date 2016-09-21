package test

import (
	"net/http"

	"github.com/quantum/castle/pkg/castlectl/client"
	"github.com/quantum/castle/pkg/model"
)

type CastleMockRestClientFactory struct{}

func (*CastleMockRestClientFactory) CreateCastleRestClient(url string, httpClient *http.Client) client.CastleRestClient {
	return &MockCastleRestClient{}
}

// Mock Castle REST Client implementation
type MockCastleRestClient struct {
	MockGetNodes   func() ([]model.Node, error)
	MockGetPools   func() ([]model.Pool, error)
	MockCreatePool func(pool model.Pool) (string, error)
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

func (m *MockCastleRestClient) URL() string {
	return ""
}
