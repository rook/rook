package clients

import (
	"errors"
	"github.com/quantum/rook-client-helpers/contracts"
)

type k8sRookPool struct {
	transportClient contracts.ITransportClient
}

func CreateK8sPool(client contracts.ITransportClient) *k8sRookPool {
	return &k8sRookPool{transportClient: client}
}

func (rp *k8sRookPool) Pool_List() (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}

func (rp *k8sRookPool) Pool_Create() (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}
