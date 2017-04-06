package clients

import (
	"github.com/quantum/rook-client-helpers/contracts"
)

type k8sRookPool struct {
	transportClient contracts.ITransportClient
}

func CreateK8sPool(client contracts.ITransportClient) k8sRookPool {
	return k8sRookPool{transportClient: client}
}

//TODO - implement all Rook object interface methods
