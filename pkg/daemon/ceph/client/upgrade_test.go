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
	assert.Nil(t, err)
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

func TestGetCephDaemonVersionString(t *testing.T) {
	executor := &exectest.MockExecutor{}
	deployment := "rook-ceph-mds-a"
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		assert.Equal(t, "tell", args[0])
		assert.Equal(t, "mds.a", args[1])
		assert.Equal(t, "version", args[2])
		return "", nil
	}
	context := &clusterd.Context{Executor: executor}

	_, err := getCephDaemonVersionString(context, deployment, "rook-ceph")
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

	err := EnableMessenger2(context)
	assert.Nil(t, err)
}

func TestEnableNautilusOSD(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		assert.Equal(t, "osd", args[0])
		assert.Equal(t, "require-osd-release", args[1])
		return "", nil
	}
	context := &clusterd.Context{Executor: executor}

	err := EnableNautilusOSD(context)
	assert.Nil(t, err)
}

func TestOkToStopDaemon(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		switch {
		case args[0] == "create" && args[1] == "mon" && args[2] == "a":
			assert.Equal(t, 2, len(args))
			return "", nil
		}
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}
	context := &clusterd.Context{Executor: executor}

	deployment := "rook-ceph-mon-a"
	err := okToStopDaemon(context, deployment, "rook-ceph")
	assert.Nil(t, err)

	deployment = "rook-ceph-mgr-a"
	err = okToStopDaemon(context, deployment, "rook-ceph")
	assert.Nil(t, err)

	deployment = "rook-ceph-dummy-a"
	err = okToStopDaemon(context, deployment, "rook-ceph")
	assert.Nil(t, err)
}

func TestFindDaemonName(t *testing.T) {
	n, err := findDaemonName("rook-ceph-mon-a")
	assert.Nil(t, err)
	assert.Equal(t, "mon", n)
	n, err = findDaemonName("rook-ceph-osd-0")
	assert.Nil(t, err)
	assert.Equal(t, "osd", n)
	n, err = findDaemonName("rook-ceph-rgw-my-store-a")
	assert.Nil(t, err)
	assert.Equal(t, "rgw", n)
	n, err = findDaemonName("rook-ceph-mds-myfs-a")
	assert.Nil(t, err)
	assert.Equal(t, "mds", n)
	n, err = findDaemonName("rook-ceph-mgr-a")
	assert.Nil(t, err)
	assert.Equal(t, "mgr", n)
	n, err = findDaemonName("rook-ceph-rbd-mirror-a")
	assert.Nil(t, err)
	assert.Equal(t, "rbd-mirror", n)
	_, err = findDaemonName("rook-ceph-unknown-a")
	assert.NotNil(t, err)
}

func TestFindDaemonID(t *testing.T) {
	id := findDaemonID("rook-ceph-mon-a")
	assert.Equal(t, "a", id)
	id = findDaemonID("rook-ceph-osd-0")
	assert.Equal(t, "0", id)
	id = findDaemonID("rook-ceph-rgw-my-super-store-a")
	assert.Equal(t, "a", id)
	id = findDaemonID("rook-ceph-mds-my-wonderful-fs-a")
	assert.Equal(t, "a", id)
	id = findDaemonID("rook-ceph-mgr-a")
	assert.Equal(t, "a", id)
	id = findDaemonID("rook.ceph.mgr.a")
	assert.NotEqual(t, "a", id)
}

func TestOkToContinue(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}

	err := OkToContinue(context, "rook-ceph", "rook-ceph-mon-a") // mon is not checked on ok-to-continue so nil is expected
	assert.Nil(t, err)
}

func TestOkToStop(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	v := cephver.Nautilus

	err := OkToStop(context, "rook-ceph", "rook-ceph-mon-a", "rook-ceph", v)
	assert.Nil(t, err)

	err = OkToStop(context, "rook-ceph", "rook-ceph-mds-a", "rook-ceph", v)
	assert.Nil(t, err)
}

func TestFindFSName(t *testing.T) {
	fsName := findFSName("rook-ceph-mds-myfs-a", "rook-ceph")
	assert.Equal(t, "myfs", fsName)
	fsName = findFSName("rook-ceph-mds-my-super-fs-a", "rook-ceph")
	assert.Equal(t, "my-super-fs", fsName)
}
