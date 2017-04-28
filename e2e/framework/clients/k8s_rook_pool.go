package clients

import (
	"fmt"
	"github.com/rook/rook/e2e/framework/contracts"
)

type k8sRookPool struct {
	transportClient contracts.ITransportClient
}

func CreateK8sPool(client contracts.ITransportClient) *k8sRookPool {
	return &k8sRookPool{transportClient: client}
}

func (rp *k8sRookPool) PoolList() (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", fmt.Errorf("NOT YET IMPLEMENTED")
}

func (rp *k8sRookPool) PoolCreate() (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", fmt.Errorf("NOT YET IMPLEMENTED")
}
