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
	"github.com/quantum/castle/pkg/proc"
	"github.com/quantum/castle/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestOSDAgentWithDevices(t *testing.T) {
	clusterName := "mycluster"
	bootstrapPath := getBootstrapOSDKeyringPath(clusterName)
	defer os.Remove(bootstrapPath)
	defer os.RemoveAll("/tmp/osd3")

	etcdClient, agent, _ := createTestAgent("sdx,sdy")

	execCount := 0
	executor := &proc.MockExecutor{}
	executor.MockExecuteCommand = func(name string, command string, args ...string) error {
		log.Printf("EXECUTE %d for %s. %s %+v", execCount, name, command, args)
		parts := strings.Split(name, " ")
		nameSuffix := parts[len(parts)-1]
		switch {
		case execCount == 0:
			assert.Equal(t, "format "+nameSuffix, name)
			assert.Equal(t, "/usr/sbin/mkfs.btrfs", args[0])
			assert.Equal(t, "/dev/"+nameSuffix, args[6])
		case execCount == 1:
			assert.Equal(t, "mount "+nameSuffix, name)
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
			assert.Equal(t, "format "+nameSuffix, name)
			assert.Equal(t, "sudo", command)
			assert.Equal(t, "/usr/sbin/mkfs.btrfs", args[0])
			assert.Equal(t, "/dev/"+nameSuffix, args[6])
		case execCount == 4:
			assert.Equal(t, "mount "+nameSuffix, name)
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

	// prep the OSD agent and related orcehstration data
	prepAgentOrchestrationData(t, agent, etcdClient, context, clusterName)

	// prep the etcd keys that would have been discovered by inventory
	disksKey := path.Join(inventory.GetNodeConfigKey(context.NodeID), inventory.DisksKey)
	etcdClient.SetValue(path.Join(disksKey, "sdxserial", "name"), "sdx")
	etcdClient.SetValue(path.Join(disksKey, "sdyserial", "name"), "sdy")

	err := agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 6, execCount)
	assert.Equal(t, 2, outputExecCount)
	assert.Equal(t, 4, procCommands)
	assert.Equal(t, 2, len(agent.osdProc))

	err = agent.DestroyLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(agent.osdProc))
}

func TestOSDAgentNoDevices(t *testing.T) {
	clusterName := "mycluster"
	bootstrapPath := getBootstrapOSDKeyringPath(clusterName)
	defer os.Remove(bootstrapPath)
	defer os.RemoveAll("/tmp/osd3")

	// create a test OSD agent with no devices specified
	etcdClient, agent, _ := createTestAgent("")

	// should be no executeCommand calls
	execCount := 0
	executor := &proc.MockExecutor{}
	executor.MockExecuteCommand = func(name string, command string, args ...string) error {
		assert.Fail(t, "executeCommand is not expected for OSD local device")
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
	}

	// prep the OSD agent and related orcehstration data
	prepAgentOrchestrationData(t, agent, etcdClient, context, clusterName)

	// configure the OSD and verify the results
	err := agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, execCount)
	assert.Equal(t, 0, outputExecCount)
	assert.Equal(t, 2, procCommands)
	assert.Equal(t, 1, len(agent.osdProc))

	// the local device should be marked as an applied OSD now
	osds, err := GetAppliedOSDs(context.NodeID, etcdClient)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(osds))
	assert.Equal(t, localDeviceName, osds[localDeviceName])

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
	inventory.TestSetDiskInfo(etcdClient, nodeConfigKey, "serial1", "sda", "ff6d4869-29ee-4bfd-bf21-dfd597bd222e",
		100, true, false, "btrfs", "/mnt/xyz", inventory.Disk, "", false)
	inventory.TestSetDiskInfo(etcdClient, nodeConfigKey, "serial2", "sdb", "ff6d4869-29ee-4bfd-bf21-dfd597bd222e",
		50, false, false, "ext4", "/mnt/zyx", inventory.Disk, "", false)
	appliedOSDKey := "/castle/services/ceph/osd/applied/abc/devices"
	etcdClient.SetValue(path.Join(appliedOSDKey, "sda", "serial"), "serial1")
	etcdClient.SetValue(path.Join(appliedOSDKey, "sdb", "serial"), "serial2")

	osds, err = GetAppliedOSDs(nodeID, etcdClient)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(osds))
	assert.Equal(t, "serial1", osds["sda"])
	assert.Equal(t, "serial2", osds["sdb"])
}

func TestRemoveDevice(t *testing.T) {
	etcdClient, agent, conn := createTestAgent("")
	executor := &proc.MockExecutor{}
	procTrap := func(action string, c *exec.Cmd) error {
		return nil
	}

	context := &clusterd.Context{EtcdClient: etcdClient, NodeID: "a", Executor: executor, ProcMan: &proc.ProcManager{Trap: procTrap}}
	desired := util.CreateSet([]string{"sda"})

	// create two applied osds
	root := "/castle/services/ceph/osd/applied/a/devices"
	etcdClient.SetValue(path.Join(root, "sda/serial"), "123")
	etcdClient.SetValue(path.Join(root, "sdb/serial"), "456")
	etcdClient.SetValue(path.Join(root, "sdc/serial"), "789")

	// the request will fail without the device id set
	err := agent.stopUndesiredDevices(context, conn, desired)
	assert.NotNil(t, err)

	etcdClient.SetValue(path.Join(root, "sda/id"), "1")
	etcdClient.SetValue(path.Join(root, "sdb/id"), "2")
	etcdClient.SetValue(path.Join(root, "sdc/id"), "3")

	err = agent.stopUndesiredDevices(context, conn, desired)
	assert.Nil(t, err)
	applied := etcdClient.GetChildDirs(root)
	assert.True(t, applied.Equals(desired))
}

func createTestAgent(devices string) (*util.MockEtcdClient, *osdAgent, *testceph.MockConnection) {
	location := "root=here"
	forceFormat := false
	etcdClient := util.NewMockEtcdClient()
	factory := &testceph.MockConnectionFactory{}
	agent := newOSDAgent(factory, devices, forceFormat, location)
	agent.cluster = &ClusterInfo{Name: "myclust"}
	agent.Initialize(&clusterd.Context{EtcdClient: etcdClient})
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
			err := ioutil.WriteFile("/tmp/osd3/mycluster-3/keyring", []byte("mykeyring"), 0644)
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
