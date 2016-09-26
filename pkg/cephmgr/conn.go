package cephmgr

import (
	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/castle/pkg/cephclient"
)

type ConnectionFactory interface {
	ConnectAsAdmin(cephFactory cephclient.ConnectionFactory, etcdClient etcd.KeysAPI) (cephclient.Connection, error)
}

type castleConnFactory struct {
}

func NewConnectionFactory() ConnectionFactory { return &castleConnFactory{} }

func (c *castleConnFactory) ConnectAsAdmin(
	cephFactory cephclient.ConnectionFactory, etcdClient etcd.KeysAPI) (cephclient.Connection, error) {

	// load information about the cluster
	cluster, err := LoadClusterInfo(etcdClient)
	if err != nil {
		return nil, err
	}

	// open an admin connection to the cluster
	return ConnectToClusterAsAdmin(cephFactory, cluster)
}
