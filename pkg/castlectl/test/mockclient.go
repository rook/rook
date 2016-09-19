package test

import (
	"net/http"

	"github.com/quantum/castle/pkg/castlectl/client"
)

type CastleMockRestClientFactory struct{}

func (*CastleMockRestClientFactory) CreateCastleRestClient(url string, httpClient *http.Client) client.CastleRestClient {
	return &MockCastleRestClient{}
}

// Mock Castle REST Client implementation
type MockCastleRestClient struct {
	MockGetNodes func() ([]client.Node, error)
}

func (m *MockCastleRestClient) GetNodes() ([]client.Node, error) {
	if m.MockGetNodes != nil {
		return m.MockGetNodes()
	}

	return nil, nil
}

func (m *MockCastleRestClient) URL() string {
	return ""
}
