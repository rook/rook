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
	"github.com/quantum/castle/pkg/util"
	exectest "github.com/quantum/castle/pkg/util/exec/test"
	"github.com/quantum/castle/pkg/util/proc"
	"github.com/stretchr/testify/assert"
)

/*
func TestOSDAgentWithDevices(t *testing.T) {
	clusterName := "mycluster"
	bootstrapPath := getBootstrapOSDKeyringPath("/tmp", clusterName)
	defer os.Remove(bootstrapPath)
	defer os.RemoveAll("/tmp/osd3")

	etcdClient, agent, _ := createTestAgent(t, "sdx,sdy")

	execCount := 0
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommand = func(name string, command string, args ...string) error {
		log.Printf("EXECUTE %d for %s. %s %+v", execCount, name, command, args)
		parts := strings.Split(name, " ")
		nameSuffix := parts[len(parts)-1]
		switch {
		case execCount == 0:
		case (execCount % 9) < 5:
			assert.Equal(t, "parted", command)
			assert.Equal(t, "-s", args[0])
			assert.Equal(t, "/dev/"+nameSuffix, args[1])

		case (execCount % 9) == 5:
			assert.Equal(t, "format "+nameSuffix, name)
			assert.Equal(t, "/usr/sbin/mkfs.btrfs", args[0])
			assert.True(t, strings.HasPrefix(args[6], "/dev/"+nameSuffix), args[6])
		case (execCount % 9) == 6:
			assert.Equal(t, "mount "+nameSuffix, name)
			assert.Equal(t, "mount", command)
			assert.Equal(t, "user_subvol_rm_allowed", args[1])
			assert.Equal(t, "/tmp/osd3", args[3])
		case (execCount % 9) == 7:
			assert.Equal(t, "chown /tmp/osd3", name)
			assert.Equal(t, "chown", command)
			assert.Equal(t, "/tmp/osd3", args[2])
		case (execCount % 9) == 8:
			assert.Equal(t, "chown "+nameSuffix, name)
			assert.Equal(t, "chown", command)
		default:
			assert.Fail(t, fmt.Sprintf("unexpected case %d", execCount))
		}
		execCount++
		return nil
	}
	outputExecCount := 0
	executor.MockExecuteCommandWithOutput = func(name string, command string, args ...string) (string, error) {
		log.Printf("OUTPUT EXECUTE %d for %s. %s %+v", outputExecCount, name, command, args)
		parts := strings.Split(name, " ")
		nameSuffix := parts[len(parts)-1]
		assert.Equal(t, "lsblk "+nameSuffix, name)
		assert.Equal(t, "lsblk", command)
		assert.Equal(t, "/dev/"+nameSuffix, args[0])
		switch {
		case outputExecCount == 0:
		case outputExecCount == 1:
		default:
			assert.Fail(t, fmt.Sprintf("unexpected case %d", outputExecCount))
		}
		outputExecCount++
		return "skip-UUID-verification", nil
	}
	procCommands := 0

	context := &clusterd.Context{
		EtcdClient: etcdClient,
		Executor:   executor,
		NodeID:     "abc",
		ProcMan:    &proc.ProcManager{Trap: createOSDAgentProcTrap(t, &procCommands, map[int]string{1: proc.StartAction, 3: proc.StartAction, 4: proc.StopAction})},
	}

	// prep the etcd keys that would have been discovered by inventory
	disksKey := path.Join(inventory.GetNodeConfigKey(context.NodeID), inventory.DisksKey)
	etcdClient.SetValue(path.Join(disksKey, "sdxserial", "name"), "sdx")
	etcdClient.SetValue(path.Join(disksKey, "sdyserial", "name"), "sdy")

	// prep the OSD agent and related orcehstration data
	prepAgentOrchestrationData(t, agent, etcdClient, context, clusterName)

	err := agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 18, execCount)
	assert.Equal(t, 2, outputExecCount)
	assert.Equal(t, 4, procCommands)
	assert.Equal(t, 2, len(agent.osdProc))

	err = agent.DestroyLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(agent.osdProc))
}
*/

func TestOSDAgentNoDevices(t *testing.T) {
	clusterName := "mycluster"
	bootstrapPath := getBootstrapOSDKeyringPath("/tmp", clusterName)
	os.MkdirAll("/tmp/osd3", 0744)
	defer os.Remove(bootstrapPath)
	defer os.RemoveAll("/tmp/osd3")

	// create a test OSD agent with no devices specified
	etcdClient, agent, _ := createTestAgent(t, "")

	// should be no executeCommand calls
	execCount := 0
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommand = func(name string, command string, args ...string) error {
		execCount++
		return nil
	}

	// should be no executeCommandWithOutput calls
	outputExecCount := 0
	executor.MockExecuteCommandWithOutput = func(name string, command string, args ...string) (string, error) {
		assert.Fail(t, "executeCommandWithOutput is not expected for OSD local device")
		outputExecCount++
		return "", nil
	}

	// set up expected ProcManager commands
	procCommands := 0
	context := &clusterd.Context{
		EtcdClient: etcdClient,
		Executor:   executor,
		NodeID:     "abc",
		ProcMan:    &proc.ProcManager{Trap: createOSDAgentProcTrap(t, &procCommands, map[int]string{1: proc.StartAction, 2: proc.StopAction})},
		ConfigDir:  "/tmp",
	}

	// prep the OSD agent and related orcehstration data
	prepAgentOrchestrationData(t, agent, etcdClient, context, clusterName)

	// configure the OSD and verify the results
	err := agent.Initialize(context)
	assert.Nil(t, err)

	err = agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, execCount)
	assert.Equal(t, 0, outputExecCount)
	assert.Equal(t, 2, procCommands)
	assert.Equal(t, 1, len(agent.osdProc))

	// the local device should be marked as an applied OSD now
	osds, err := GetAppliedOSDs(context.NodeID, etcdClient)
	assert.Nil(t, err)
	assert.Equal(t, 0, osds.Count())
	// TODO: applied osds should return the configured dirs

	// destroy the OSD and verify the results
	err = agent.DestroyLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(agent.osdProc))
}

func TestAppliedDevices(t *testing.T) {
	nodeID := "abc"
	etcdClient := util.NewMockEtcdClient()

	// no applied osds
	osds, err := GetAppliedOSDs(nodeID, etcdClient)
	assert.Nil(t, err)
	assert.Equal(t, 0, osds.Count())

	// two applied osds
	nodeConfigKey := path.Join(inventory.NodesConfigKey, nodeID)
	etcdClient.CreateDir(nodeConfigKey)
	inventory.TestSetDiskInfo(etcdClient, nodeConfigKey, "serial1", "sda", "ff6d4869-29ee-4bfd-bf21-dfd597bd222e",
		100, true, false, "btrfs", "/mnt/xyz", inventory.Disk, "", false)
	inventory.TestSetDiskInfo(etcdClient, nodeConfigKey, "serial2", "sdb", "ff6d4869-29ee-4bfd-bf21-dfd597bd222e",
		50, false, false, "ext4", "/mnt/zyx", inventory.Disk, "", false)
	appliedOSDKey := "/castle/services/ceph/osd/applied/abc/device"
	etcdClient.SetValue(path.Join(appliedOSDKey, "serial1", "name"), "sda")
	etcdClient.SetValue(path.Join(appliedOSDKey, "serial2", "name"), "sdb")

	osds, err = GetAppliedOSDs(nodeID, etcdClient)
	assert.Nil(t, err)
	assert.Equal(t, 2, osds.Count())
	assert.True(t, osds.Equals(util.CreateSet([]string{"serial1", "serial2"})))
}

func TestRemoveDevice(t *testing.T) {
	etcdClient, agent, conn := createTestAgent(t, "")
	executor := &exectest.MockExecutor{}
	procTrap := func(action string, c *exec.Cmd) error {
		return nil
	}

	context := &clusterd.Context{EtcdClient: etcdClient, NodeID: "a", Executor: executor, ProcMan: &proc.ProcManager{Trap: procTrap}}
	desired := map[string]string{"sda": "123"}

	// create two applied osds, one of which is desired
	appliedRoot := "/castle/services/ceph/osd/applied/a/device"
	etcdClient.SetValue(path.Join(appliedRoot, "123", "name"), "sda")
	etcdClient.SetValue(path.Join(appliedRoot, "456", "name"), "sdb")

	// removing the device will fail without the id
	err := agent.stopUndesiredDevices(context, conn, desired)
	assert.NotNil(t, err)

	// set the id then successfully remove the device
	etcdClient.SetValue(path.Join(appliedRoot, "456", "id"), "1")
	err = agent.stopUndesiredDevices(context, conn, desired)
	assert.Nil(t, err)

	applied := etcdClient.GetChildDirs(appliedRoot)
	assert.True(t, applied.Equals(util.CreateSet([]string{"123"})), fmt.Sprintf("applied=%+v", applied))
}

func createTestAgent(t *testing.T, devices string) (*util.MockEtcdClient, *osdAgent, *testceph.MockConnection) {
	location := "root=here"
	forceFormat := false
	etcdClient := util.NewMockEtcdClient()
	factory := &testceph.MockConnectionFactory{}
	agent := newOSDAgent(factory, devices, forceFormat, location)
	agent.cluster = &ClusterInfo{Name: "myclust"}
	agent.Initialize(&clusterd.Context{EtcdClient: etcdClient, ConfigDir: "/tmp"})
	assert.Equal(t, "/tmp", etcdClient.GetValue("/castle/services/ceph/osd/desired/dir/tmp/path"))

	conn, _ := factory.NewConnWithClusterAndUser("default", "user")
	mockConn := conn.(*testceph.MockConnection)
	mockConn.MockMonCommand = func(buf []byte) (buffer []byte, info string, err error) {
		response := "{\"key\":\"mysecurekey\", \"osdid\":3.0}"
		return []byte(response), "", nil
	}

	return etcdClient, agent, mockConn
}

func prepAgentOrchestrationData(t *testing.T, agent *osdAgent, etcdClient *util.MockEtcdClient, context *clusterd.Context, clusterName string) {
	key := path.Join(cephKey, osdAgentName, desiredKey, context.NodeID)
	etcdClient.CreateDir(key)

	err := agent.Initialize(context)
	etcdClient.SetValue(path.Join(cephKey, osdAgentName, desiredKey, context.NodeID, "ready"), "1")
	assert.Nil(t, err)

	// prep the etcd keys as if the leader initiated the orchestration
	cluster := &ClusterInfo{FSID: "id", MonitorSecret: "monsecret", AdminSecret: "adminsecret", Name: clusterName}
	saveClusterInfo(cluster, etcdClient)
	monKey := path.Join(cephKey, monitorKey, desiredKey, context.NodeID)
	etcdClient.SetValue(path.Join(monKey, "id"), "1")
	etcdClient.SetValue(path.Join(monKey, "ipaddress"), "10.6.5.4")
	etcdClient.SetValue(path.Join(monKey, "port"), "8743")
}

func createOSDAgentProcTrap(t *testing.T, commands *int, actions map[int]string) func(action string, c *exec.Cmd) error {
	return func(action string, c *exec.Cmd) error {
		log.Printf("PROC TRAP %d for %s. %+v", *commands, action, c)

		assert.Equal(t, "daemon", c.Args[1])
		assert.Equal(t, "--type=osd", c.Args[2])
		assert.Equal(t, "--", c.Args[3])
		if a, ok := actions[*commands]; ok {
			assert.Equal(t, a, action)
		} else {
			// the default action expected is Run
			assert.Equal(t, proc.RunAction, action, fmt.Sprintf("command=%d, action=%s", *commands, action))
		}

		switch {
		case *commands == 0:
			err := ioutil.WriteFile("/tmp/osd3/keyring", []byte("mykeyring"), 0644)
			assert.Nil(t, err)
			assert.Equal(t, "--mkfs", c.Args[4])
			assert.Equal(t, "--mkkey", c.Args[5])
		case *commands == 1:
			assert.Equal(t, "--foreground", c.Args[4])
		case *commands == 2:
			if action == proc.StopAction {
				assert.Equal(t, "--foreground", c.Args[4])
			} else {
				assert.Equal(t, "--mkfs", c.Args[4])
			}
		case *commands == 3:
			assert.Equal(t, "--foreground", c.Args[4])
		default:
			return fmt.Errorf("unexpected case %d", *commands)
		}
		*commands++
		return nil
	}
}
