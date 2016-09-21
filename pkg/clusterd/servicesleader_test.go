package clusterd

import (
	"path"
	"testing"

	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestLoadDiscoveredNodes(t *testing.T) {
	etcdClient := &util.MockEtcdClient{}
	mockHandler := newTestServiceLeader()
	raised := make(chan bool)
	mockHandler.unhealthyNode = func(nodes []*UnhealthyNode) {
		assert.Equal(t, 1, len(nodes))
		assert.Equal(t, "23", nodes[0].NodeID)
		raised <- true
	}

	context := &Context{EtcdClient: etcdClient}
	context.Services = []*ClusterService{
		&ClusterService{Name: "test", Leader: mockHandler},
	}
	leader := &servicesLeader{context: context}
	leader.parent = &ClusterMember{isLeader: true}

	mockHandler.StartWatchEvents()
	defer mockHandler.Close()

	etcdClient.SetValue(path.Join(inventory.NodesConfigKey, "23", "ipaddress"), "1.2.3.4")

	// one unhealthy nodes to discover
	err := leader.discoverUnhealthyNodes()
	<-raised
	assert.Nil(t, err)
}
