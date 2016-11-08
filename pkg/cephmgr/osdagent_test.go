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

	testceph "github.com/rook/rook/pkg/cephmgr/client/test"
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
		log.Printf("START %d for %s. %s %+v", startCount, name, command, args)
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
		log.Printf("RUN %d for %s. %s %+v", execCount, name, command, args)
		parts := strings.Split(name, " ")
		nameSuffix := parts[len(parts)-1]
		switch {
		case execCount%3 == 0:
			assert.Equal(t, "sgdisk", command)
			assert.Equal(t, "--zap-all", args[0])
			assert.Equal(t, "/dev/"+nameSuffix, args[1])
		case execCount%3 == 1:
			assert.Equal(t, "/dev/"+nameSuffix, args[10])
		case execCount%3 == 2:
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
		log.Printf("OUTPUT %d for %s. %s %+v", outputExecCount, name, command, args)
		outputExecCount++
		return "", nil
	}

	context := &clusterd.Context{
		EtcdClient: etcdClient,
		Executor:   executor,
		NodeID:     nodeID,
		ConfigDir:  "/tmp",
		ProcMan:    proc.New(executor),
	}

	// prep the etcd keys that would have been discovered by inventory
	disksKey := path.Join(inventory.GetNodeConfigKey(context.NodeID), "disks")
	etcdClient.SetValue(path.Join(disksKey, "sdx", "uuid"), "12345")
	etcdClient.SetValue(path.Join(disksKey, "sdy", "uuid"), "54321")
	etcdClient.SetValue(path.Join(disksKey, "sdx", "size"), "1234567890")
	etcdClient.SetValue(path.Join(disksKey, "sdy", "size"), "2234567890")

	// Set one device as already having an assigned osd id. The other device will go through id selection,
	// which is mocked in the createTestAgent method to return an id of 3.
	etcdClient.SetValue(fmt.Sprintf("/rook/services/ceph/osd/desired/%s/device/sdx/osd-id", nodeID), "23")

	// prep the OSD agent and related orcehstration data
	prepAgentOrchestrationData(t, agent, etcdClient, context, clusterName)

	err := agent.ConfigureLocalService(context)
	assert.Nil(t, err)

	// wait for the async osds to complete
	<-agent.osdsCompleted

	assert.Equal(t, 0, agent.configCounter)
	assert.Equal(t, 6, execCount)
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
	nodeConfigKey := path.Join(inventory.NodesConfigKey, nodeID)
	etcdClient.CreateDir(nodeConfigKey)
	inventory.TestSetDiskInfo(etcdClient, nodeConfigKey, "sda", "ff6d4869-29ee-4bfd-bf21-dfd597bd222e",
		100, true, false, "btrfs", "/mnt/xyz", inventory.Disk, "", false)
	inventory.TestSetDiskInfo(etcdClient, nodeConfigKey, "sdb", "ff6d4869-29ee-4bfd-bf21-dfd597bd222e",
		50, false, false, "ext4", "/mnt/zyx", inventory.Disk, "", false)
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

	context := &clusterd.Context{EtcdClient: etcdClient, NodeID: nodeID, Executor: executor, ProcMan: proc.New(executor)}
	etcdClient.SetValue("/rook/services/ceph/osd/desired/a/device/sda/osd-id", "23")

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
	agent := newOSDAgent(factory, devices, forceFormat, location)
	agent.cluster = &ClusterInfo{Name: "myclust"}
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
