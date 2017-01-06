/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package osd

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"

	testceph "github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/proc"
	"github.com/stretchr/testify/assert"
)

func TestOSDAgentWithDevices(t *testing.T) {
	clusterName := "mycluster"
	nodeID := "abc"
	bootstrapPath := getBootstrapOSDKeyringPath("/tmp", clusterName)
	defer os.Remove(bootstrapPath)
	defer os.RemoveAll("/tmp/osd3")

	etcdClient, agent, _ := createTestAgent(t, nodeID, "sdx,sdy")

	startCount := 0
	executor := &exectest.MockExecutor{}
	executor.MockStartExecuteCommand = func(name string, command string, args ...string) (*exec.Cmd, error) {
		logger.Infof("START %d for %s. %s %+v", startCount, name, command, args)
		cmd := &exec.Cmd{Args: append([]string{command}, args...)}

		switch {
		case startCount < 2:
			assert.Equal(t, "--type=osd", args[1])
		default:
			assert.Fail(t, fmt.Sprintf("unexpected case %d", startCount))
		}
		startCount++
		return cmd, nil
	}

	execCount := 0
	executor.MockExecuteCommand = func(name string, command string, args ...string) error {
		logger.Infof("RUN %d for %s. %s %+v", execCount, name, command, args)
		parts := strings.Split(name, " ")
		nameSuffix := parts[0]
		if len(parts) > 1 {
			nameSuffix = parts[1]
		}
		switch {
		case execCount%4 == 0:
			assert.Equal(t, "sgdisk", command)
			assert.Equal(t, "--zap-all", args[0])
			assert.Equal(t, "/dev/"+nameSuffix, args[1])
		case execCount%4 == 1:
			assert.Equal(t, "sgdisk", command)
			assert.Equal(t, "--clear", args[0])
			assert.Equal(t, "/dev/"+nameSuffix, args[2])
		case execCount%4 == 2:
			assert.Equal(t, "/dev/"+nameSuffix, args[10])
		case execCount%4 == 3:
			assert.Equal(t, "--mkfs", args[3])
			createTestKeyring(t, args)
		default:
			assert.Fail(t, fmt.Sprintf("unexpected case %d", execCount))
		}
		execCount++
		return nil
	}
	outputExecCount := 0
	executor.MockExecuteCommandWithOutput = func(name string, command string, args ...string) (string, error) {
		logger.Infof("OUTPUT %d for %s. %s %+v", outputExecCount, name, command, args)
		outputExecCount++
		return "", nil
	}

	context := &clusterd.Context{
		EtcdClient: etcdClient,
		Executor:   executor,
		NodeID:     nodeID,
		ConfigDir:  "/tmp",
		ProcMan:    proc.New(executor),
		Inventory:  createInventory(),
	}
	context.Inventory.Local.Disks = []*inventory.LocalDisk{
		&inventory.LocalDisk{Name: "sdx", Size: 1234567890, UUID: "12345"},
		&inventory.LocalDisk{Name: "sdy", Size: 2234567890, UUID: "54321"},
	}

	// Set one device as already having an assigned osd id. The other device will go through id selection,
	// which is mocked in the createTestAgent method to return an id of 3.
	etcdClient.SetValue(fmt.Sprintf("/rook/services/ceph/osd/desired/%s/device/12345/osd-id", nodeID), "23")

	// prep the OSD agent and related orcehstration data
	prepAgentOrchestrationData(t, agent, etcdClient, context, clusterName)

	err := agent.ConfigureLocalService(context)
	assert.Nil(t, err)

	// wait for the async osds to complete
	<-agent.osdsCompleted

	assert.Equal(t, 0, agent.configCounter)
	assert.Equal(t, 8, execCount)
	assert.Equal(t, 2, outputExecCount)
	assert.Equal(t, 2, startCount)
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

	startCount := 0
	executor := &exectest.MockExecutor{}
	executor.MockStartExecuteCommand = func(name string, command string, args ...string) (*exec.Cmd, error) {
		startCount++
		cmd := &exec.Cmd{Args: append([]string{command}, args...)}
		return cmd, nil
	}

	// should be no executeCommand calls
	runCount := 0
	executor.MockExecuteCommand = func(name string, command string, args ...string) error {
		runCount++
		createTestKeyring(t, args)
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
	context := &clusterd.Context{
		EtcdClient: etcdClient,
		Executor:   executor,
		NodeID:     nodeID,
		ProcMan:    proc.New(executor),
		ConfigDir:  "/tmp",
		Inventory:  createInventory(),
	}

	// prep the OSD agent and related orcehstration data
	prepAgentOrchestrationData(t, agent, etcdClient, context, clusterName)

	// configure the OSD and verify the results
	err := agent.Initialize(context)
	assert.Nil(t, err)
	etcdClient.SetValue("/rook/services/ceph/osd/desired/abc/dir/tmp/osd-id", "3")

	err = agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 1, runCount)
	assert.Equal(t, 1, startCount)
	assert.Equal(t, 0, outputExecCount)
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
	appliedOSDKey := "/rook/services/ceph/osd/applied/abc"
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

	context := &clusterd.Context{EtcdClient: etcdClient, NodeID: nodeID, Executor: executor, ProcMan: proc.New(executor), Inventory: createInventory()}
	context.Inventory.Local.Disks = []*inventory.LocalDisk{&inventory.LocalDisk{Name: "sda", Size: 1234567890, UUID: "5435435333"}}
	etcdClient.SetValue("/rook/services/ceph/osd/desired/a/device/5435435333/osd-id", "23")

	// create two applied osds, one of which is desired
	appliedRoot := "/rook/services/ceph/osd/applied/" + nodeID
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
	agent := NewAgent(factory, devices, forceFormat, location)
	agent.cluster = &mon.ClusterInfo{Name: "myclust"}
	agent.Initialize(&clusterd.Context{EtcdClient: etcdClient, NodeID: nodeID, ConfigDir: "/tmp"})
	if devices == "" {
		assert.Equal(t, "/tmp", etcdClient.GetValue(fmt.Sprintf("/rook/services/ceph/osd/desired/%s/dir/tmp/path", nodeID)))
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
	key := path.Join(mon.CephKey, osdAgentName, clusterd.DesiredKey, context.NodeID)
	etcdClient.CreateDir(key)

	err := agent.Initialize(context)
	etcdClient.SetValue(path.Join(mon.CephKey, osdAgentName, clusterd.DesiredKey, context.NodeID, "ready"), "1")
	assert.Nil(t, err)

	// prep the etcd keys as if the leader initiated the orchestration
	etcdClient.SetValue(path.Join(mon.CephKey, "fsid"), "id")
	etcdClient.SetValue(path.Join(mon.CephKey, "name"), clusterName)
	etcdClient.SetValue(path.Join(mon.CephKey, "_secrets", "monitor"), "monsecret")
	etcdClient.SetValue(path.Join(mon.CephKey, "_secrets", "admin"), "adminsecret")

	monKey := path.Join(mon.CephKey, "monitor", clusterd.DesiredKey, context.NodeID)
	etcdClient.SetValue(path.Join(monKey, "id"), "1")
	etcdClient.SetValue(path.Join(monKey, "ipaddress"), "10.6.5.4")
	etcdClient.SetValue(path.Join(monKey, "port"), "8743")
}

func createTestKeyring(t *testing.T, args []string) {
	var configDir string
	if len(args) > 5 && strings.HasPrefix(args[5], "--id") {
		configDir = "/tmp/osd" + args[5][5:]
		err := os.MkdirAll(configDir, 0744)
		assert.Nil(t, err)
		err = ioutil.WriteFile(path.Join(configDir, "keyring"), []byte("mykeyring"), 0644)
		assert.Nil(t, err)
	}
}

func TestDesiredDeviceState(t *testing.T) {
	nodeID := "a"
	etcdClient := util.NewMockEtcdClient()

	// add a device
	err := AddDesiredDevice(etcdClient, nodeID, "myuuid", 23)
	assert.Nil(t, err)
	devices := etcdClient.GetChildDirs("/rook/services/ceph/osd/desired/a/device")
	assert.Equal(t, 1, devices.Count())
	assert.True(t, devices.Contains("myuuid"))

	// remove the device
	err = RemoveDesiredDevice(etcdClient, nodeID, "myuuid")
	assert.Nil(t, err)
	devices = etcdClient.GetChildDirs("/rook/services/ceph/osd/desired/a/device")
	assert.Equal(t, 0, devices.Count())

	// removing a non-existent device is a no-op
	err = RemoveDesiredDevice(etcdClient, nodeID, "foo")
	assert.Nil(t, err)
}

func TestLoadDesiredDevices(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	a := &osdAgent{desiredDevices: []string{}}

	// no devices are desired
	context := &clusterd.Context{EtcdClient: etcdClient, Inventory: createInventory(), NodeID: "a"}
	desired, err := a.loadDesiredDevices(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(desired))

	// two devices are desired and it is a new config
	context.Inventory.Local.Disks = []*inventory.LocalDisk{
		&inventory.LocalDisk{Name: "sda", Size: 1234567890, UUID: "12345"},
		&inventory.LocalDisk{Name: "sdb", Size: 2234567890, UUID: "54321"},
	}
	a.desiredDevices = []string{"sda", "sdb"}
	desired, err = a.loadDesiredDevices(context)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(desired))
	assert.Equal(t, -1, desired["sda"])
	assert.Equal(t, -1, desired["sdb"])

	// two devices are desired and they have previously been configured
	etcdClient.SetValue("/rook/services/ceph/osd/desired/a/device/12345/osd-id", "23")
	etcdClient.SetValue("/rook/services/ceph/osd/desired/a/device/54321/osd-id", "24")
	desired, err = a.loadDesiredDevices(context)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(desired))
	assert.Equal(t, 23, desired["sda"])
	assert.Equal(t, 24, desired["sdb"])

	// no devices are desired and they have previously been configured
	a.desiredDevices = []string{}
	desired, err = a.loadDesiredDevices(context)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(desired))
	assert.Equal(t, 23, desired["sda"])
	assert.Equal(t, 24, desired["sdb"])
}

func TestDesiredDirsState(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()

	// add a dir
	err := AddDesiredDir(etcdClient, "/my/dir", "a")
	assert.Nil(t, err)
	dirs := etcdClient.GetChildDirs("/rook/services/ceph/osd/desired/a/dir")
	assert.Equal(t, 1, dirs.Count())
	assert.True(t, dirs.Contains("my_dir"))
	assert.Equal(t, "/my/dir", etcdClient.GetValue("/rook/services/ceph/osd/desired/a/dir/my_dir/path"))

	loadedDirs, err := loadDesiredDirs(etcdClient, "a")
	assert.Nil(t, err)

	assert.Equal(t, 1, len(loadedDirs))
	assert.Equal(t, unassignedOSDID, loadedDirs["/my/dir"])
}

func createInventory() *inventory.Config {
	return &inventory.Config{Local: &inventory.Hardware{Disks: []*inventory.LocalDisk{}}}
}
