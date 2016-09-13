package castled

import (
	"fmt"
	"os/exec"
	"path"
	"testing"

	"github.com/quantum/clusterd/pkg/orchestrator"
	"github.com/quantum/clusterd/pkg/proc"
	"github.com/quantum/clusterd/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestMonAgent(t *testing.T) {

	commands := 0
	procTrap := func(action string, c *exec.Cmd) error {
		switch {
		case commands == 0:
			assert.Equal(t, proc.RunAction, action)
			assert.Equal(t, "daemon", c.Args[1])
			assert.Equal(t, "--type=mon", c.Args[2])
			assert.Equal(t, "--", c.Args[3])
			assert.Equal(t, "--mkfs", c.Args[4])
			assert.Equal(t, "--cluster=castlecluster", c.Args[5])
			assert.Equal(t, "--name=mon.mon0", c.Args[6])
			assert.Equal(t, "--mon-data=/tmp/mon0/mon.mon0", c.Args[7])
			assert.Equal(t, "--conf=/tmp/mon0/castlecluster.config", c.Args[8])
			assert.Equal(t, "--keyring=/tmp/mon0/keyring", c.Args[9])
		case commands == 1:
			assert.Equal(t, proc.StartAction, action)
			assert.Equal(t, "daemon", c.Args[1])
			assert.Equal(t, "--type=mon", c.Args[2])
			assert.Equal(t, "--", c.Args[3])
			assert.Equal(t, "--foreground", c.Args[4])
			assert.Equal(t, "--cluster=castlecluster", c.Args[5])
			assert.Equal(t, "--name=mon.mon0", c.Args[6])
			assert.Equal(t, "--mon-data=/tmp/mon0/mon.mon0", c.Args[7])
			assert.Equal(t, "--conf=/tmp/mon0/castlecluster.config", c.Args[8])
			assert.Equal(t, "--public-addr=1.2.3.4:2345", c.Args[9])
		default:
			return fmt.Errorf("unexpected case %d", commands)
		}
		commands++
		return nil
	}
	etcdClient := util.NewMockEtcdClient()
	context := &orchestrator.ClusterContext{
		EtcdClient: etcdClient,
		NodeID:     "a",
		ProcMan:    &proc.ProcManager{Trap: procTrap},
	}

	cluster, err := createOrGetClusterInfo(etcdClient)
	assert.Nil(t, err)
	assert.NotNil(t, cluster)

	// nothing expected to be configured because the node is not in the desired state
	agent := &monAgent{}
	err = agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, commands)

	// set the agent in the desired state
	key := path.Join(cephKey, monitorAgentName, desiredKey, context.NodeID)
	etcdClient.SetValue(path.Join(key, "id"), "mon0")
	etcdClient.SetValue(path.Join(key, "ipaddress"), "1.2.3.4")
	etcdClient.SetValue(path.Join(key, "port"), "2345")

	// now the monitor will be configured
	err = agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 2, commands)
}
