package cephmgr

import (
	"testing"

	testceph "github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
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
