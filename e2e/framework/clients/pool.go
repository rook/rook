package clients

import (
	"github.com/rook/rook/e2e/framework/contracts"
	"github.com/rook/rook/pkg/model"
)

type PoolClient struct {
	transportClient contracts.ITransportClient
	restClient      contracts.RestAPIOperator
}

func CreatePoolClient(rookRestClient contracts.RestAPIOperator) *PoolClient {
	return &PoolClient{restClient: rookRestClient}
}

func (rp *PoolClient) PoolList() ([]model.Pool, error) {
	return rp.restClient.GetPools()
}

func (rp *PoolClient) PoolCreate(pool model.Pool) (string, error) {
	return rp.restClient.CreatePool(pool)
}
