package cephmgr

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"

	testceph "github.com/quantum/castle/pkg/cephmgr/client/test"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/util"
	exectest "github.com/quantum/castle/pkg/util/exec/test"
	"github.com/quantum/castle/pkg/util/proc"
	"github.com/stretchr/testify/assert"
)

func TestOSDAgentWithDevices(t *testing.T) {
	clusterName := "mycluster"
	nodeID := "abc"
	bootstrapPath := getBootstrapOSDKeyringPath("/tmp", clusterName)
	defer os.Remove(bootstrapPath)
	defer os.RemoveAll("/tmp/osd3")

	etcdClient, agent, _ := createTestAgent(t, nodeID, "sdx,sdy")

	execCount := 0
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommand = func(name string, command string, args ...string) error {
		log.Printf("EXECUTE %d for %s. %s %+v", execCount, name, command, args)
		parts := strings.Split(name, " ")
		nameSuffix := parts[len(parts)-1]
		assert.Equal(t, "sgdisk", command)
		switch {
		case execCount == 0:
		case execCount%2 == 0:
			assert.Equal(t, "--zap-all", args[0])
			assert.Equal(t, "/dev/"+nameSuffix, args[1])
		case execCount%2 == 1:
			assert.Equal(t, "/dev/"+nameSuffix, args[10])
		default:
			assert.Fail(t, fmt.Sprintf("unexpected case %d", execCount))
		}
		execCount++
		return nil
	}
	outputExecCount := 0
	executor.MockExecuteCommandWithOutput = func(name string, command string, args ...string) (string, error) {
		log.Printf("OUTPUT EXECUTE %d for %s. %s %+v", outputExecCount, name, command, args)
		outputExecCount++
		return "", nil
	}
	procCommands := 0

	context := &clusterd.Context{
		EtcdClient: etcdClient,
		Executor:   executor,
		NodeID:     nodeID,
		ConfigDir:  "/tmp",
		ProcMan:    &proc.ProcManager{Trap: createOSDAgentProcTrap(t, &procCommands, map[int]string{1: proc.StartAction, 3: proc.StartAction, 4: proc.StopAction, 5: proc.StopAction})},
	}

	// prep the etcd keys that would have been discovered by inventory
	disksKey := path.Join(inventory.GetNodeConfigKey(context.NodeID), inventory.DisksKey)
	etcdClient.SetValue(path.Join(disksKey, "sdx", "uuid"), "12345")
	etcdClient.SetValue(path.Join(disksKey, "sdy", "uuid"), "54321")
	etcdClient.SetValue(path.Join(disksKey, "sdx", "size"), "1234567890")
	etcdClient.SetValue(path.Join(disksKey, "sdy", "size"), "2234567890")

	// Set one device as already having an assigned osd id. The other device will go through id selection,
	// which is mocked in the createTestAgent method to return an id of 3.
	etcdClient.SetValue(fmt.Sprintf("/castle/services/ceph/osd/desired/%s/device/sdx/osd-id", nodeID), "23")

	// prep the OSD agent and related orcehstration data
	prepAgentOrchestrationData(t, agent, etcdClient, context, clusterName)

	err := agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 4, execCount)
	assert.Equal(t, 2, outputExecCount)
	assert.Equal(t, 4, procCommands)
	assert.Equal(t, 2, len(agent.osdProc), fmt.Sprintf("procs=%+v", agent.osdProc))

	err = agent.DestroyLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(agent.osdProc))
}

func TestOSDAgentNoDevices(t *testing.T) {
	clusterName := "mycluster"
	bootstrapPath := getBootstrapOSDKeyringPath("/tmp", clusterName)
	os.MkdirAll("/tmp/osd3", 0744)
	defer os.Remove(bootstrapPath)
	defer os.RemoveAll("/tmp/osd3")

	// create a test OSD agent with no devices specified
	nodeID := "abc"
	etcdClient, agent, _ := createTestAgent(t, nodeID, "")

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
		NodeID:     nodeID,
		ProcMan:    &proc.ProcManager{Trap: createOSDAgentProcTrap(t, &procCommands, map[int]string{1: proc.StartAction, 2: proc.StopAction})},
		ConfigDir:  "/tmp",
	}

	// prep the OSD agent and related orcehstration data
	prepAgentOrchestrationData(t, agent, etcdClient, context, clusterName)

	// configure the OSD and verify the results
	err := agent.Initialize(context)
	assert.Nil(t, err)
	etcdClient.SetValue("/castle/services/ceph/osd/desired/abc/dir/tmp/osd-id", "3")

	err = agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, execCount)
	assert.Equal(t, 0, outputExecCount)
	assert.Equal(t, 2, procCommands)
	assert.Equal(t, 1, len(agent.osdProc))

	// the local device should be marked as an applied OSD now
	osds, err := GetAppliedOSDs(context.NodeID, etcdClient)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(osds))

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
	assert.Equal(t, 0, len(osds))

	// two applied osds
	nodeConfigKey := path.Join(inventory.NodesConfigKey, nodeID)
	etcdClient.CreateDir(nodeConfigKey)
	inventory.TestSetDiskInfo(etcdClient, nodeConfigKey, "sda", "ff6d4869-29ee-4bfd-bf21-dfd597bd222e",
		100, true, false, "btrfs", "/mnt/xyz", inventory.Disk, "", false)
	inventory.TestSetDiskInfo(etcdClient, nodeConfigKey, "sdb", "ff6d4869-29ee-4bfd-bf21-dfd597bd222e",
		50, false, false, "ext4", "/mnt/zyx", inventory.Disk, "", false)
	appliedOSDKey := "/castle/services/ceph/osd/applied/abc"
	etcdClient.SetValue(path.Join(appliedOSDKey, "1", "disk-uuid"), "1234")
	etcdClient.SetValue(path.Join(appliedOSDKey, "2", "disk-uuid"), "2345")

	osds, err = GetAppliedOSDs(nodeID, etcdClient)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(osds))
	assert.Equal(t, "1234", osds[1])
	assert.Equal(t, "2345", osds[2])
}

func TestRemoveDevice(t *testing.T) {
	nodeID := "a"
	etcdClient, agent, conn := createTestAgent(t, nodeID, "")
	executor := &exectest.MockExecutor{}
	procTrap := func(action string, c *exec.Cmd) error {
		return nil
	}

	context := &clusterd.Context{EtcdClient: etcdClient, NodeID: nodeID, Executor: executor, ProcMan: &proc.ProcManager{Trap: procTrap}}
	etcdClient.SetValue("/castle/services/ceph/osd/desired/a/device/sda/osd-id", "23")

	// create two applied osds, one of which is desired
	appliedRoot := "/castle/services/ceph/osd/applied/" + nodeID
	etcdClient.SetValue(path.Join(appliedRoot, "23", "disk-uuid"), "5435435333")
	etcdClient.SetValue(path.Join(appliedRoot, "56", "disk-uuid"), "2342342343")

	// removing the device will fail without the id
	err := agent.stopUndesiredDevices(context, conn)
	assert.Nil(t, err)

	applied := etcdClient.GetChildDirs(appliedRoot)
	assert.True(t, applied.Equals(util.CreateSet([]string{"23"})), fmt.Sprintf("applied=%+v", applied))
}

func createTestAgent(t *testing.T, nodeID, devices string) (*util.MockEtcdClient, *osdAgent, *testceph.MockConnection) {
	location := "root=here"
	forceFormat := false
	etcdClient := util.NewMockEtcdClient()
	factory := &testceph.MockConnectionFactory{}
	agent := newOSDAgent(factory, devices, forceFormat, location)
	agent.cluster = &ClusterInfo{Name: "myclust"}
	agent.Initialize(&clusterd.Context{EtcdClient: etcdClient, NodeID: nodeID, ConfigDir: "/tmp"})
	if devices == "" {
		assert.Equal(t, "/tmp", etcdClient.GetValue(fmt.Sprintf("/castle/services/ceph/osd/desired/%s/dir/tmp/path", nodeID)))
	}

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
		command := fmt.Sprintf("[%d] %s %+v", *commands, action, c)

		assert.Equal(t, "daemon", c.Args[1])
		assert.Equal(t, "--type=osd", c.Args[2])
		assert.Equal(t, "--", c.Args[3])
		if a, ok := actions[*commands]; ok {
			assert.Equal(t, a, action, command)
		} else {
			// the default action expected is Run
			assert.Equal(t, proc.RunAction, action, fmt.Sprintf("command=%d, action=%s", *commands, action))
		}

		var configDir string
		if len(c.Args) > 6 && strings.HasPrefix(c.Args[6], "--id") {
			configDir = "/tmp/osd" + c.Args[6][5:]
			err := os.MkdirAll(configDir, 0744)
			assert.Nil(t, err)
			err = ioutil.WriteFile(path.Join(configDir, "keyring"), []byte("mykeyring"), 0644)
			assert.Nil(t, err)
		}

		switch {
		case *commands == 0:
			assert.Equal(t, "--mkfs", c.Args[4], command)
			assert.Equal(t, "--mkkey", c.Args[5], command)
		case *commands == 1:
			assert.Equal(t, "--foreground", c.Args[4], command)
		case *commands == 2:
			if action == proc.StopAction {
				assert.Equal(t, "--foreground", c.Args[4], command)
			} else {
				assert.Equal(t, "--mkfs", c.Args[4], command)
			}
		case *commands >= 3 && *commands <= 5:
			assert.Equal(t, "--foreground", c.Args[4], command)
		default:
			assert.Fail(t, "unexpected case %d. %s", *commands, command)
		}
		*commands++
		return nil
	}
}
