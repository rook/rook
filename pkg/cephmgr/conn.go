package cephmgr

import (
	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/clusterd"
)

type ConnectionFactory interface {
	ConnectAsAdmin(context *clusterd.Context, cephFactory client.ConnectionFactory) (client.Connection, error)
}

type rookConnFactory struct {
}

func NewConnectionFactory() ConnectionFactory { return &rookConnFactory{} }

func (c *rookConnFactory) ConnectAsAdmin(
	context *clusterd.Context, cephFactory client.ConnectionFactory) (client.Connection, error) {

	// load information about the cluster
	cluster, err := LoadClusterInfo(context.EtcdClient)
	if err != nil {
		return nil, err
	}

	// open an admin connection to the cluster
	return ConnectToClusterAsAdmin(context, cephFactory, cluster)
}
