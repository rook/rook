package etcdmgr

import (
	"path"
	"testing"

	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/etcdmgr/test"
	"github.com/quantum/castle/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestEtcdMgrAgent(t *testing.T) {
	mockContext := test.MockContext{}
	// adding 1.2.3.4 as the first/existing cluster member
	mockContext.AddMembers([]string{"http://1.2.3.4:53379"})
	mockEmbeddedEtcdFactory := test.MockEmbeddedEtcdFactory{}

	// agent2 is the agent on node 2 which is going to create a new embedded etcd
	agent2 := &etcdMgrAgent{context: &mockContext, etcdFactory: &mockEmbeddedEtcdFactory}
	etcdClient2 := util.NewMockEtcdClient()
	context2 := &clusterd.Context{
		EtcdClient: etcdClient2,
		NodeID:     "node2",
	}
	err := agent2.Initialize(context2)
	assert.Equal(t, "etcd", agent2.Name())
	assert.Nil(t, agent2.embeddedEtcd)

	err = agent2.ConfigureLocalService(context2)
	assert.Nil(t, err)

	//set the agent in the desired state
	desiredKey := path.Join(etcdmgrKey, etcdDesiredKey, context2.NodeID)
	etcdClient2.SetValue(path.Join(desiredKey, "ipaddress"), "2.3.4.5")

	err = agent2.ConfigureLocalService(context2)
	assert.Nil(t, err)
	assert.NotNil(t, agent2.embeddedEtcd)
	//remove the desired status
	etcdClient2.DeleteDir(desiredKey)
	err = agent2.ConfigureLocalService(context2)
	assert.Nil(t, err)
	assert.Nil(t, agent2.embeddedEtcd)
}
