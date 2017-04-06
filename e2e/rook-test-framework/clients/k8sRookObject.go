package clients

import (
	"github.com/quantum/rook-client-helpers/contracts"
)

type k8sRookObject struct {
	transportClient contracts.ITransportClient
}

func CreateK8sRookObject(client contracts.ITransportClient) k8sRookObject {
	return k8sRookObject{transportClient: client}
}

//TODO - implement all Rook object interface methods
