package cephmgr

import (
	"testing"

	testceph "github.com/quantum/castle/pkg/cephmgr/client/test"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestBasicConn(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	createTestClusterInfo(etcdClient, []string{"a"})
	factory := NewConnectionFactory()
	fact := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	context := &clusterd.Context{EtcdClient: etcdClient, ConfigDir: "/tmp"}

	conn, err := factory.ConnectAsAdmin(context, fact)
	assert.Nil(t, err)
	assert.NotNil(t, conn)
}
