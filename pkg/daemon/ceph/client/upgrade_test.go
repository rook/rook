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
	"encoding/json"
	"testing"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client/fake"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestGetCephMonVersionString(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		assert.Equal(t, "version", args[0])
		return "", nil
	}
	context := &clusterd.Context{Executor: executor}

	_, err := getCephMonVersionString(context, AdminTestClusterInfo("mycluster"))
	assert.NoError(t, err)
}

func TestGetCephMonVersionsString(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		assert.Equal(t, "versions", args[0])
		return "", nil
	}
	context := &clusterd.Context{Executor: executor}

	_, err := getAllCephDaemonVersionsString(context, AdminTestClusterInfo("mycluster"))
	assert.Nil(t, err)
}

func TestEnableReleaseOSDFunctionality(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		assert.Equal(t, "osd", args[0])
		assert.Equal(t, "require-osd-release", args[1])
		return "", nil
	}
	context := &clusterd.Context{Executor: executor}

	err := EnableReleaseOSDFunctionality(context, AdminTestClusterInfo("mycluster"), "squid")
	assert.NoError(t, err)
}

func TestOkToStopDaemon(t *testing.T) {
	// First test
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		switch {
		case args[0] == "mon" && args[1] == "ok-to-stop" && args[2] == "a":
			return "", nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	context := &clusterd.Context{Executor: executor}

	deployment := "rook-ceph-mon-a"
	err := okToStopDaemon(context, AdminTestClusterInfo("mycluster"), deployment, "mon", "a")
	assert.NoError(t, err)

	// Second test
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		assert.Equal(t, "mgr", args[0])
		assert.Equal(t, "ok-to-stop", args[1])
		assert.Equal(t, "a", args[2])
		return "", nil
	}
	context = &clusterd.Context{Executor: executor}

	deployment = "rook-ceph-mgr-a"
	err = okToStopDaemon(context, AdminTestClusterInfo("mycluster"), deployment, "mgr", "a")
	assert.NoError(t, err)

	// Third test
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		assert.Equal(t, "dummy", args[0])
		assert.Equal(t, "ok-to-stop", args[1])
		assert.Equal(t, "a", args[2])
		return "", nil
	}
	context = &clusterd.Context{Executor: executor}

	deployment = "rook-ceph-dummy-a"
	err = okToStopDaemon(context, AdminTestClusterInfo("mycluster"), deployment, "dummy", "a")
	assert.NoError(t, err)
}

func TestOkToContinue(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}

	err := OkToContinue(context, AdminTestClusterInfo("mycluster"), "rook-ceph-mon-a", "mon", "a") // mon is not checked on ok-to-continue so nil is expected
	assert.NoError(t, err)
}

func TestFindFSName(t *testing.T) {
	fsName := findFSName("rook-ceph-mds-myfs-a")
	assert.Equal(t, "myfs-a", fsName)
	fsName = findFSName("rook-ceph-mds-my-super-fs-a")
	assert.Equal(t, "my-super-fs-a", fsName)
}

func TestDaemonMapEntry(t *testing.T) {
	dummyVersionsRaw := []byte(`
	{
		"mon": {
			"ceph version 18.2.5 (cbff874f9007f1869bfd3821b7e33b2a6ffd4988) reef (stable)": 1,
			"ceph version 19.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) squid (stable)": 2
		}
	}`)

	var dummyVersions cephv1.CephDaemonsVersions
	err := json.Unmarshal([]byte(dummyVersionsRaw), &dummyVersions)
	assert.NoError(t, err)

	m, err := daemonMapEntry(&dummyVersions, "mon")
	assert.NoError(t, err)
	assert.Equal(t, dummyVersions.Mon, m)

	_, err = daemonMapEntry(&dummyVersions, "dummy")
	assert.Error(t, err)
}

func TestBuildHostListFromTree(t *testing.T) {
	dummyOsdTreeRaw := []byte(`
	{
		"nodes": [
		  {
			"id": -3,
			"name": "r1",
			"type": "rack",
			"type_id": 3,
			"children": [
			  -4
			]
		  },
		  {
			"id": -4,
			"name": "ceph-nano-oooooo",
			"type": "host",
			"type_id": 1,
			"pool_weights": {},
			"children": [
			  0
			]
		  },
		  {
			"id": 0,
			"name": "osd.0",
			"type": "osd",
			"type_id": 0,
			"crush_weight": 0.009796,
			"depth": 2,
			"pool_weights": {},
			"exists": 1,
			"status": "up",
			"reweight": 1,
			"primary_affinity": 1
		  },
		  {
			"id": -1,
			"name": "default",
			"type": "root",
			"type_id": 10,
			"children": [
			  -2
			]
		  },
		  {
			"id": -2,
			"name": "ceph-nano-nau-faa32aebf00b",
			"type": "host",
			"type_id": 1,
			"pool_weights": {},
			"children": []
		  }
		],
		"stray": [
		  {
			"id": 1,
			"name": "osd.1",
			"type": "osd",
			"type_id": 0,
			"crush_weight": 0,
			"depth": 0,
			"exists": 1,
			"status": "down",
			"reweight": 0,
			"primary_affinity": 1
		  }
		]
	  }`)

	var dummyTree OsdTree
	err := json.Unmarshal([]byte(dummyOsdTreeRaw), &dummyTree)
	assert.NoError(t, err)

	osdHosts, err := buildHostListFromTree(dummyTree)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(osdHosts.Nodes))

	dummyEmptyOsdTreeRaw := []byte(`{}`)
	var dummyEmptyTree OsdTree
	err = json.Unmarshal([]byte(dummyEmptyOsdTreeRaw), &dummyEmptyTree)
	assert.NoError(t, err)

	_, err = buildHostListFromTree(dummyEmptyTree)
	assert.Error(t, err)

	dummyEmptyNodeOsdTreeRaw := []byte(`{"nodes": []}`)
	var dummyEmptyNodeTree OsdTree
	err = json.Unmarshal([]byte(dummyEmptyNodeOsdTreeRaw), &dummyEmptyNodeTree)
	assert.NoError(t, err)

	_, err = buildHostListFromTree(dummyEmptyNodeTree)
	assert.NoError(t, err)
}

func TestGetRetryConfig(t *testing.T) {
	testcases := []struct {
		label           string
		clusterInfo     *ClusterInfo
		daemonType      string
		expectedRetries int
		expectedDelay   time.Duration
	}{
		{
			label:           "case 1: mon daemon",
			clusterInfo:     &ClusterInfo{},
			daemonType:      "mon",
			expectedRetries: 10,
			expectedDelay:   60 * time.Second,
		},
		{
			label:           "case 2: osd daemon with 5 minutes delay",
			clusterInfo:     &ClusterInfo{OsdUpgradeTimeout: 5 * time.Minute},
			daemonType:      "osd",
			expectedRetries: 30,
			expectedDelay:   10 * time.Second,
		},
		{
			label:           "case 3: osd daemon with 10 minutes delay",
			clusterInfo:     &ClusterInfo{OsdUpgradeTimeout: 10 * time.Minute},
			daemonType:      "osd",
			expectedRetries: 60,
			expectedDelay:   10 * time.Second,
		},
		{
			label:           "case 4: mds daemon",
			clusterInfo:     &ClusterInfo{},
			daemonType:      "mds",
			expectedRetries: 10,
			expectedDelay:   15 * time.Second,
		},
	}

	for _, tc := range testcases {
		actualRetries, actualDelay := getRetryConfig(tc.clusterInfo, tc.daemonType)

		assert.Equal(t, tc.expectedRetries, actualRetries, "[%s] failed to get correct retry count", tc.label)
		assert.Equalf(t, tc.expectedDelay, actualDelay, "[%s] failed to get correct delays between retries", tc.label)
	}
}

func TestOSDUpdateShouldCheckOkToStop(t *testing.T) {
	clusterInfo := AdminTestClusterInfo("mycluster")
	lsOutput := ""
	treeOutput := ""
	context := &clusterd.Context{
		Executor: &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				t.Logf("command: %s %v", command, args)
				if command != "ceph" || args[0] != "osd" {
					panic("not a 'ceph osd' call")
				}
				if args[1] == "tree" {
					if treeOutput == "" {
						return "", errors.Errorf("induced error")
					}
					return treeOutput, nil
				}
				if args[1] == "ls" {
					if lsOutput == "" {
						return "", errors.Errorf("induced error")
					}
					return lsOutput, nil
				}
				panic("do not understand command")
			},
		},
	}

	t.Run("3 nodes with 1 OSD each", func(t *testing.T) {
		lsOutput = fake.OsdLsOutput(3)
		treeOutput = fake.OsdTreeOutput(3, 1)
		assert.True(t, OSDUpdateShouldCheckOkToStop(context, clusterInfo))
	})

	t.Run("1 node with 3 OSDs", func(t *testing.T) {
		lsOutput = fake.OsdLsOutput(3)
		treeOutput = fake.OsdTreeOutput(1, 3)
		assert.True(t, OSDUpdateShouldCheckOkToStop(context, clusterInfo))
	})

	t.Run("2 nodes with 1 OSD each", func(t *testing.T) {
		lsOutput = fake.OsdLsOutput(2)
		treeOutput = fake.OsdTreeOutput(2, 1)
		assert.False(t, OSDUpdateShouldCheckOkToStop(context, clusterInfo))
	})

	t.Run("3 nodes with 3 OSDs each", func(t *testing.T) {
		lsOutput = fake.OsdLsOutput(9)
		treeOutput = fake.OsdTreeOutput(3, 3)
		assert.True(t, OSDUpdateShouldCheckOkToStop(context, clusterInfo))
	})

	// degraded case but good to test just in case
	t.Run("0 nodes", func(t *testing.T) {
		lsOutput = fake.OsdLsOutput(0)
		treeOutput = fake.OsdTreeOutput(0, 0)
		assert.False(t, OSDUpdateShouldCheckOkToStop(context, clusterInfo))
	})

	// degraded case, OSDs are failing to start so they haven't registered in the CRUSH map yet
	t.Run("0 nodes with down OSDs", func(t *testing.T) {
		lsOutput = fake.OsdLsOutput(3)
		treeOutput = fake.OsdTreeOutput(0, 1)
		assert.True(t, OSDUpdateShouldCheckOkToStop(context, clusterInfo))
	})
}
