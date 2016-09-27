package cephmgr

import (
	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/castle/pkg/cephmgr/client"
)

type ConnectionFactory interface {
	ConnectAsAdmin(cephFactory client.ConnectionFactory, etcdClient etcd.KeysAPI) (client.Connection, error)
}

type castleConnFactory struct {
}

func NewConnectionFactory() ConnectionFactory { return &castleConnFactory{} }

func (c *castleConnFactory) ConnectAsAdmin(
	cephFactory client.ConnectionFactory, etcdClient etcd.KeysAPI) (client.Connection, error) {

	// load information about the cluster
	cluster, err := LoadClusterInfo(etcdClient)
	if err != nil {
		return nil, err
	}

	// open an admin connection to the cluster
	return ConnectToClusterAsAdmin(cephFactory, cluster)
}
