package cephmgr

import (
	"testing"

	testceph "github.com/quantum/castle/pkg/cephmgr/client/test"
	"github.com/quantum/castle/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestBasicConn(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	createTestClusterInfo(etcdClient, []string{"a"})
	factory := NewConnectionFactory()
	fact := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}

	conn, err := factory.ConnectAsAdmin(fact, etcdClient)
	assert.Nil(t, err)
	assert.NotNil(t, conn)
}
