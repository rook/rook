package test

import (
	"github.com/quantum/castle/pkg/cephmgr/client"
	"github.com/quantum/castle/pkg/clusterd"
)

type MockConnectionFactory struct {
	MockConnectAsAdmin func(context *clusterd.Context, cephFactory client.ConnectionFactory) (client.Connection, error)
}

func (m *MockConnectionFactory) ConnectAsAdmin(
	context *clusterd.Context, cephFactory client.ConnectionFactory) (client.Connection, error) {

	if m.MockConnectAsAdmin != nil {
		return m.MockConnectAsAdmin(context, cephFactory)
	}

	return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
}
