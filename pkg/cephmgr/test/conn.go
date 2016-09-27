package test

import (
	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/castle/pkg/cephmgr/client"
)

type MockConnectionFactory struct {
	MockConnectAsAdmin func(cephFactory client.ConnectionFactory, etcdClient etcd.KeysAPI) (client.Connection, error)
}

func (m *MockConnectionFactory) ConnectAsAdmin(
	cephFactory client.ConnectionFactory, etcdClient etcd.KeysAPI) (client.Connection, error) {

	if m.MockConnectAsAdmin != nil {
		return m.MockConnectAsAdmin(cephFactory, etcdClient)
	}

	return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
}
