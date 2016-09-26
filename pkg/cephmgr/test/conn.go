package test

import (
	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/castle/pkg/cephclient"
)

type MockConnectionFactory struct {
	MockConnectAsAdmin func(cephFactory cephclient.ConnectionFactory, etcdClient etcd.KeysAPI) (cephclient.Connection, error)
}

func (m *MockConnectionFactory) ConnectAsAdmin(
	cephFactory cephclient.ConnectionFactory, etcdClient etcd.KeysAPI) (cephclient.Connection, error) {

	if m.MockConnectAsAdmin != nil {
		return m.MockConnectAsAdmin(cephFactory, etcdClient)
	}

	return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
}
