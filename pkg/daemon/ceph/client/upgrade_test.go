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

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestGetCephMonVersionString(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		assert.Equal(t, "version", args[0])
		return "", nil
	}
	context := &clusterd.Context{Executor: executor}

	_, err := getCephMonVersionString(context, "rook-ceph")
	assert.NoError(t, err)
}

func TestGetCephMonVersionsString(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		assert.Equal(t, "versions", args[0])
		return "", nil
	}
	context := &clusterd.Context{Executor: executor}

	_, err := getAllCephDaemonVersionsString(context, "rook-ceph")
	assert.Nil(t, err)
}

func TestEnableMessenger2(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		assert.Equal(t, "mon", args[0])
		assert.Equal(t, "enable-msgr2", args[1])
		return "", nil
	}
	context := &clusterd.Context{Executor: executor}

	err := EnableMessenger2(context, "rook-ceph")
	assert.NoError(t, err)
}

func TestEnableNautilusOSD(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		assert.Equal(t, "osd", args[0])
		assert.Equal(t, "require-osd-release", args[1])
		return "", nil
	}
	context := &clusterd.Context{Executor: executor}

	err := EnableNautilusOSD(context, "rook-ceph")
	assert.NoError(t, err)
}

func TestOkToStopDaemon(t *testing.T) {
	// First test
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		switch {
		case args[0] == "mon" && args[1] == "ok-to-stop" && args[2] == "a":
			return "", nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	context := &clusterd.Context{Executor: executor}

	deployment := "rook-ceph-mon-a"
	err := okToStopDaemon(context, deployment, "rook-ceph", "mon", "a")
	assert.NoError(t, err)

	// Second test
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		assert.Equal(t, "mgr", args[0])
		assert.Equal(t, "ok-to-stop", args[1])
		assert.Equal(t, "a", args[2])
		return "", nil
	}
	context = &clusterd.Context{Executor: executor}

	deployment = "rook-ceph-mgr-a"
	err = okToStopDaemon(context, deployment, "rook-ceph", "mgr", "a")
	assert.NoError(t, err)

	// Third test
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		assert.Equal(t, "dummy", args[0])
		assert.Equal(t, "ok-to-stop", args[1])
		assert.Equal(t, "a", args[2])
		return "", nil
	}
	context = &clusterd.Context{Executor: executor}

	deployment = "rook-ceph-dummy-a"
	err = okToStopDaemon(context, deployment, "rook-ceph", "dummy", "a")
	assert.NoError(t, err)
}

func TestOkToContinue(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}

	err := OkToContinue(context, "rook-ceph", "rook-ceph-mon-a", "mon", "a") // mon is not checked on ok-to-continue so nil is expected
	assert.NoError(t, err)
}

func TestOkToStop(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	v := cephver.Nautilus

	err := OkToStop(context, "rook-ceph", "rook-ceph-mon-a", "mon", "a", v)
	assert.NoError(t, err)

	err = OkToStop(context, "rook-ceph", "rook-ceph-mds-a", "mds", "a", v)
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
			"ceph version 13.2.5 (cbff874f9007f1869bfd3821b7e33b2a6ffd4988) mimic (stable)": 1,
			"ceph version 14.2.0 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) nautilus (stable)": 2
		}
	}`)

	var dummyVersions CephDaemonsVersions
	err := json.Unmarshal([]byte(dummyVersionsRaw), &dummyVersions)
	assert.NoError(t, err)

	m, err := daemonMapEntry(&dummyVersions, "mon")
	assert.NoError(t, err)
	assert.Equal(t, dummyVersions.Mon, m)

	m, err = daemonMapEntry(&dummyVersions, "dummy")
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
