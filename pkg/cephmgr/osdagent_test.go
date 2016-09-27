package cephmgr

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"testing"

	testceph "github.com/quantum/castle/pkg/cephmgr/client/test"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/proc"
	"github.com/quantum/castle/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestOSDAgent(t *testing.T) {
	clusterName := "mycluster"
	targetPath := getBootstrapOSDKeyringPath(clusterName)
	defer os.Remove(targetPath)
	defer os.RemoveAll("/tmp/osd3")

	factory := &testceph.MockConnectionFactory{}
	conn, _ := factory.NewConnWithClusterAndUser(clusterName, "user")
	conn.(*testceph.MockConnection).MockMonCommand = func(buf []byte) (buffer []byte, info string, err error) {
		response := "{\"key\":\"mysecurekey\", \"osdid\":3.0}"
		return []byte(response), "", nil
	}
	forceFormat := false
	devices := "sdx,sdy"
	location := &CrushLocation{Root: "here"}
	agent := newOSDAgent(factory, devices, forceFormat, location)

	execCount := 0
	executor := &proc.MockExecutor{}
	executor.MockExecuteCommand = func(name string, command string, args ...string) error {
		log.Printf("EXECUTE %d for %s. %s %+v", execCount, name, command, args)
		switch {
		case execCount == 0:
			assert.Equal(t, "format sdx", name)
			assert.Equal(t, "/usr/sbin/mkfs.btrfs", args[0])
			assert.Equal(t, "/dev/sdx", args[6])
		case execCount == 1:
			assert.Equal(t, "mount sdx", name)
			assert.Equal(t, "sudo", command)
			assert.Equal(t, "mount", args[0])
			assert.Equal(t, "user_subvol_rm_allowed", args[2])
			assert.Equal(t, "/tmp/osd3", args[4])
		case execCount == 2:
			assert.Equal(t, "chown /tmp/osd3", name)
			assert.Equal(t, "sudo", command)
			assert.Equal(t, "chown", args[0])
			assert.Equal(t, "/tmp/osd3", args[3])
		case execCount == 3:
			assert.Equal(t, "format sdy", name)
			assert.Equal(t, "sudo", command)
			assert.Equal(t, "/usr/sbin/mkfs.btrfs", args[0])
			assert.Equal(t, "/dev/sdy", args[6])
		case execCount == 4:
			assert.Equal(t, "mount sdy", name)
			assert.Equal(t, "sudo", command)
			assert.Equal(t, "mount", args[0])
			assert.Equal(t, "user_subvol_rm_allowed", args[2])
			assert.Equal(t, "/tmp/osd3", args[4])
		case execCount == 5:
			assert.Equal(t, "chown /tmp/osd3", name)
			assert.Equal(t, "sudo", command)
			assert.Equal(t, "chown", args[0])
			assert.Equal(t, "/tmp/osd3", args[3])
		default:
			assert.Fail(t, fmt.Sprintf("unexpected case %d", execCount))
		}
		execCount++
		return nil
	}
	outputExecCount := 0
	executor.MockExecuteCommandWithOutput = func(name string, command string, args ...string) (string, error) {
		log.Printf("OUTPUT EXECUTE %d for %s. %s %+v", outputExecCount, name, command, args)
		switch {
		case outputExecCount == 0:
			assert.Equal(t, "lsblk sdx", name)
			assert.Equal(t, "lsblk", command)
			assert.Equal(t, "/dev/sdx", args[0])
		case outputExecCount == 1:
			assert.Equal(t, "lsblk sdy", name)
			assert.Equal(t, "lsblk", command)
			assert.Equal(t, "/dev/sdy", args[0])
		default:

			assert.Fail(t, fmt.Sprintf("unexpected case %d", outputExecCount))
		}
		outputExecCount++
		return "skip-UUID-verification", nil
	}
	commands := 0
	procTrap := func(action string, c *exec.Cmd) error {
		log.Printf("PROC TRAP %d for %s. %+v", commands, action, c)
		assert.Equal(t, "daemon", c.Args[1])
		assert.Equal(t, "--type=osd", c.Args[2])
		assert.Equal(t, "--", c.Args[3])
		switch {
		case commands == 0:
			err := ioutil.WriteFile("/tmp/osd3/mycluster-3/keyring", []byte("mykeyring"), 0644)
			assert.Nil(t, err)
			assert.Equal(t, "--mkfs", c.Args[4])
			assert.Equal(t, "--mkkey", c.Args[5])
		case commands == 1:
			assert.Equal(t, "--foreground", c.Args[4])
		case commands == 2:
			assert.Equal(t, "--mkfs", c.Args[4])
		case commands == 3:
			assert.Equal(t, "--foreground", c.Args[4])
		default:
			return fmt.Errorf("unexpected case %d", commands)
		}
		commands++
		return nil
	}

	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{
		EtcdClient: etcdClient,
		Executor:   executor,
		NodeID:     "abc",
		ProcMan:    &proc.ProcManager{Trap: procTrap},
	}
	key := path.Join(cephKey, osdAgentName, desiredKey, context.NodeID)
	etcdClient.CreateDir(key)

	// prep the etcd keys as if the leader initiated the orchestration
	cluster := &ClusterInfo{FSID: "id", MonitorSecret: "monsecret", AdminSecret: "adminsecret", Name: clusterName}
	saveClusterInfo(cluster, etcdClient)
	monKey := path.Join(cephKey, monitorKey, desiredKey, context.NodeID)
	etcdClient.SetValue(path.Join(monKey, "id"), "1")
	etcdClient.SetValue(path.Join(monKey, "ipaddress"), "10.6.5.4")
	etcdClient.SetValue(path.Join(monKey, "port"), "8743")

	// prep the etcd keys that would have been discovered by inventory
	disksKey := path.Join(inventory.GetNodeConfigKey(context.NodeID), inventory.DisksKey)
	etcdClient.SetValue(path.Join(disksKey, "sdxserial", "name"), "sdx")
	etcdClient.SetValue(path.Join(disksKey, "sdyserial", "name"), "sdy")

	err := agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 6, execCount)
	assert.Equal(t, 2, outputExecCount)
	assert.Equal(t, 4, commands)
	assert.Equal(t, 1, len(agent.osdCmd))

	err = agent.DestroyLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(agent.osdCmd))
}
