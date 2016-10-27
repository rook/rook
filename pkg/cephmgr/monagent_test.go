package cephmgr

import (
	"fmt"
	"log"
	"os/exec"
	"path"
	"testing"

	testceph "github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/proc"
	"github.com/stretchr/testify/assert"
)

func TestMonAgent(t *testing.T) {

	commands := 0
	procTrap := func(action string, c *exec.Cmd) error {
		log.Printf("PROC TRAP %d for %s. %+v", commands, action, c)
		assert.Equal(t, "daemon", c.Args[1])
		assert.Equal(t, "--type=mon", c.Args[2])
		assert.Equal(t, "--", c.Args[3])
		assert.Equal(t, "--cluster=rookcluster", c.Args[5])
		assert.Equal(t, "--name=mon.mon0", c.Args[6])
		assert.Equal(t, "--mon-data=/tmp/mon0/mon.mon0", c.Args[7])
		assert.Equal(t, "--conf=/tmp/mon0/rookcluster.config", c.Args[8])
		switch {
		case commands == 0:
			assert.Equal(t, proc.RunAction, action)
			assert.Equal(t, "--mkfs", c.Args[4])
			assert.Equal(t, "--keyring=/tmp/mon0/keyring", c.Args[9])
		case commands == 1:
			assert.Equal(t, proc.StartAction, action)
			assert.Equal(t, "--foreground", c.Args[4])
			assert.Equal(t, "--public-addr=1.2.3.4:2345", c.Args[9])
		case commands == 2:
			assert.Equal(t, proc.StopAction, action)
		default:
			return fmt.Errorf("unexpected case %d", commands)
		}
		commands++
		return nil
	}
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{
		EtcdClient: etcdClient,
		NodeID:     "a",
		ProcMan:    &proc.ProcManager{Trap: procTrap},
		ConfigDir:  "/tmp",
	}

	factory := &testceph.MockConnectionFactory{Fsid: "f", SecretKey: "k"}
	cluster, err := createOrGetClusterInfo(factory, etcdClient)
	assert.Nil(t, err)
	assert.NotNil(t, cluster)

	// nothing expected to be configured because the node is not in the desired state
	agent := &monAgent{}
	err = agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, commands)
	assert.Nil(t, agent.monProc)

	// set the agent in the desired state
	key := path.Join(cephKey, monitorAgentName, desiredKey, context.NodeID)
	etcdClient.SetValue(path.Join(key, "id"), "mon0")
	etcdClient.SetValue(path.Join(key, "ipaddress"), "1.2.3.4")
	etcdClient.SetValue(path.Join(key, "port"), "2345")

	// now the monitor will be configured
	err = agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 2, commands)
	assert.NotNil(t, agent.monProc)

	// when the mon is not in desired state, it will be removed
	etcdClient.DeleteDir(key)
	err = agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 3, commands)
	assert.Nil(t, agent.monProc)
}
