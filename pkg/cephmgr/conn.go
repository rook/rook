package cephmgr

import (
	"github.com/quantum/castle/pkg/cephmgr/client"
	"github.com/quantum/castle/pkg/clusterd"
)

type ConnectionFactory interface {
	ConnectAsAdmin(context *clusterd.Context, cephFactory client.ConnectionFactory) (client.Connection, error)
}

type castleConnFactory struct {
}

func NewConnectionFactory() ConnectionFactory { return &castleConnFactory{} }

func (c *castleConnFactory) ConnectAsAdmin(
	context *clusterd.Context, cephFactory client.ConnectionFactory) (client.Connection, error) {

	// load information about the cluster
	cluster, err := LoadClusterInfo(context.EtcdClient)
	if err != nil {
		return nil, err
	}

	// open an admin connection to the cluster
	return ConnectToClusterAsAdmin(context, cephFactory, cluster)
}
