/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package client

import (
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client/fake"
	"github.com/rook/rook/pkg/operator/ceph/version"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

var (
	fakeOsdTree = `{
		"nodes": [
		  {
			"id": -3,
			"name": "minikube",
			"type": "host",
			"type_id": 1,
			"pool_weights": {},
			"children": [
			  2,
			  1,
			  0
			]
		  },
		  {
			"id": -2,
			"name": "minikube-2",
			"type": "host",
			"type_id": 1,
			"pool_weights": {},
			"children": [
			  3,
			  4,
			  5
			]
		  }
		]
	  }`

	fakeOSdList = `[0,1,2]`
)

func TestHostTree(t *testing.T) {
	executor := &exectest.MockExecutor{}
	emptyTreeResult := false
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		switch {
		case args[0] == "osd" && args[1] == "tree":
			if emptyTreeResult {
				return `not a json`, nil
			}
			return fakeOsdTree, nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	tree, err := HostTree(&clusterd.Context{Executor: executor}, AdminTestClusterInfo("mycluster"))
	assert.NoError(t, err)
	assert.Equal(t, 2, len(tree.Nodes))
	assert.Equal(t, "minikube", tree.Nodes[0].Name)
	assert.Equal(t, 3, len(tree.Nodes[0].Children))

	emptyTreeResult = true
	tree, err = HostTree(&clusterd.Context{Executor: executor}, AdminTestClusterInfo("mycluster"))
	assert.Error(t, err)
	assert.Equal(t, 0, len(tree.Nodes))
}

func TestGetDestroyedIDs(t *testing.T) {
	treeWithDestroyed := `{
		"nodes": [
			{"id": -1, "name": "default", "type": "root", "type_id": 10, "children": [-3]},
			{"id": -3, "name": "node1", "type": "host", "type_id": 1, "children": [0, 1, 2]},
			{"id": 0, "name": "osd.0", "type": "osd", "type_id": 0, "exists": 1, "status": "up"},
			{"id": 1, "name": "osd.1", "type": "osd", "type_id": 0, "exists": 1, "status": "destroyed"},
			{"id": 2, "name": "osd.2", "type": "osd", "type_id": 0, "exists": 1, "status": "down"}
		],
		"stray": []
	}`

	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if args[0] == "osd" && args[1] == "tree" {
			return treeWithDestroyed, nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	tree, err := HostTree(&clusterd.Context{Executor: executor}, AdminTestClusterInfo("mycluster"))
	assert.NoError(t, err)
	assert.Equal(t, []int{1}, tree.GetDestroyedIDs())

	// no destroyed slots
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if args[0] == "osd" && args[1] == "tree" {
			return fakeOsdTree, nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	tree, err = HostTree(&clusterd.Context{Executor: executor}, AdminTestClusterInfo("mycluster"))
	assert.NoError(t, err)
	assert.Empty(t, tree.GetDestroyedIDs())
}

func TestOSDLifecycleHelpers(t *testing.T) {
	var gotArgs []string
	failNext := false
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			gotArgs = args
			if failNext {
				return "", errors.New("boom")
			}
			return "", nil
		},
	}
	ctx := &clusterd.Context{Executor: executor}
	info := AdminTestClusterInfo("mycluster")
	// the ceph command appends global flags (--cluster, --conf, ...); assert only the leading args.
	hasPrefix := func(prefix ...string) bool {
		if len(gotArgs) < len(prefix) {
			return false
		}
		for i, p := range prefix {
			if gotArgs[i] != p {
				return false
			}
		}
		return true
	}

	assert.NoError(t, OSDOut(ctx, info, 5))
	assert.True(t, hasPrefix("osd", "out", "5"), "got %v", gotArgs)

	assert.NoError(t, OSDIn(ctx, info, 5))
	assert.True(t, hasPrefix("osd", "in", "5"), "got %v", gotArgs)

	assert.NoError(t, OSDDown(ctx, info, 5))
	assert.True(t, hasPrefix("osd", "down", "5"), "got %v", gotArgs)

	assert.NoError(t, OSDDestroy(ctx, info, 5))
	assert.True(t, hasPrefix("osd", "destroy", "osd.5", "--yes-i-really-mean-it"), "got %v", gotArgs)

	// error propagation
	failNext = true
	assert.Error(t, OSDOut(ctx, info, 5))
	assert.Error(t, OSDIn(ctx, info, 5))
	assert.Error(t, OSDDown(ctx, info, 5))
	assert.Error(t, OSDDestroy(ctx, info, 5))
}

func TestOsdListNum(t *testing.T) {
	executor := &exectest.MockExecutor{}
	emptyOsdListNumResult := false
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		switch {
		case args[0] == "osd" && args[1] == "ls":
			if emptyOsdListNumResult {
				return `not a json`, nil
			}
			return fakeOSdList, nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	list, err := OsdListNum(&clusterd.Context{Executor: executor}, AdminTestClusterInfo("mycluster"))
	assert.NoError(t, err)
	assert.Equal(t, 3, len(list))

	emptyOsdListNumResult = true
	list, err = OsdListNum(&clusterd.Context{Executor: executor}, AdminTestClusterInfo("mycluster"))
	assert.Error(t, err)
	assert.Equal(t, 0, len(list))
}

func TestOSDDeviceClasses(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		switch {
		case args[0] == "osd" && args[1] == "crush" && args[2] == "get-device-class" && len(args) > 3:
			return fake.OSDDeviceClassOutput(args[3]), nil
		default:
			return fake.OSDDeviceClassOutput(""), nil
		}
	}

	context := &clusterd.Context{Executor: executor}
	clusterInfo := AdminTestClusterInfo("mycluster")

	t.Run("device classes returned", func(t *testing.T) {
		deviceClasses, err := OSDDeviceClasses(context, clusterInfo, []string{"0"})
		assert.NoError(t, err)
		assert.Equal(t, deviceClasses[0].DeviceClass, "hdd")
	})

	t.Run("error happened when no id provided", func(t *testing.T) {
		_, err := OSDDeviceClasses(context, clusterInfo, []string{})
		assert.Error(t, err)
	})
}

func TestConvertKibibytesToTebibytes(t *testing.T) {
	kib := "1024"
	terabyte, err := convertKibibytesToTebibytes(kib)
	assert.NoError(t, err)
	assert.Equal(t, float64(9.5367431640625e-07), terabyte)

	kib = "1073741824"
	terabyte, err = convertKibibytesToTebibytes(kib)
	assert.NoError(t, err)
	assert.Equal(t, float64(1), terabyte)
}

func TestOSDOkToStop(t *testing.T) {
	returnString := ""
	returnOkResult := true
	seenArgs := []string{}

	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		switch {
		case args[0] == "osd" && args[1] == "ok-to-stop":
			seenArgs = args
			if returnOkResult {
				return returnString, nil
			}
			return returnString, errors.Errorf("Error EBUSY: unsafe to stop osd(s) at this time (50 PGs are or would become offline)")
		}
		panic(fmt.Sprintf("unexpected ceph command %q", args))
	}

	context := &clusterd.Context{Executor: executor}
	clusterInfo := AdminTestClusterInfo("mycluster")

	doSetup := func() {
		seenArgs = []string{}
	}

	t.Run("output ok to stop", func(t *testing.T) {
		doSetup()
		clusterInfo.CephVersion = version.Squid
		returnString = fake.OsdOkToStopOutput(1, []int{1, 2})
		returnOkResult = true
		osds, err := OSDOkToStop(context, clusterInfo, 1, 2)
		assert.NoError(t, err)
		assert.ElementsMatch(t, osds, []int{1, 2})
		assert.Equal(t, "1", seenArgs[2])
		assert.Equal(t, "--max=2", seenArgs[3])
	})

	t.Run("output not ok to stop", func(t *testing.T) {
		doSetup()
		clusterInfo.CephVersion = version.Squid
		returnString = fake.OsdOkToStopOutput(3, []int{})
		returnOkResult = false
		_, err := OSDOkToStop(context, clusterInfo, 3, 5)
		assert.Error(t, err)
		assert.Equal(t, "3", seenArgs[2])
		assert.Equal(t, "--max=5", seenArgs[3])
	})

	t.Run("handle maxReturned=0", func(t *testing.T) {
		doSetup()
		clusterInfo.CephVersion = version.Squid
		returnString = fake.OsdOkToStopOutput(4, []int{4, 8})
		returnOkResult = true
		osds, err := OSDOkToStop(context, clusterInfo, 4, 0)
		assert.NoError(t, err)
		assert.ElementsMatch(t, osds, []int{4, 8})
		assert.Equal(t, "4", seenArgs[2])
		// should just pass through as --max=0; don't do any special processing
		assert.Equal(t, "--max=0", seenArgs[3])
	})
}
