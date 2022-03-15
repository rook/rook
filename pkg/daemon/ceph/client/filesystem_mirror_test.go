/*
Copyright 2021 The Rook Authors. All rights reserved.

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
	"encoding/base64"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

var (
	// response of "ceph fs snapshot mirror peer_bootstrap create myfs2 client.mirror test"
	// #nosec G101 since this is not leaking any credentials
	fsMirrorToken = `{"token": "eyJmc2lkIjogIjgyYjdlZDkyLTczYjAtNGIyMi1hOGI3LWVkOTQ4M2UyODc1NiIsICJmaWxlc3lzdGVtIjogIm15ZnMyIiwgInVzZXIiOiAiY2xpZW50Lm1pcnJvciIsICJzaXRlX25hbWUiOiAidGVzdCIsICJrZXkiOiAiQVFEVVAxSmdqM3RYQVJBQWs1cEU4cDI1ZUhld2lQK0ZXRm9uOVE9PSIsICJtb25faG9zdCI6ICJbdjI6MTAuOTYuMTQyLjIxMzozMzAwLHYxOjEwLjk2LjE0Mi4yMTM6Njc4OV0sW3YyOjEwLjk2LjIxNy4yMDc6MzMwMCx2MToxMC45Ni4yMTcuMjA3OjY3ODldLFt2MjoxMC45OS4xMC4xNTc6MzMwMCx2MToxMC45OS4xMC4xNTc6Njc4OV0ifQ=="}`

	// response of "ceph fs snapshot mirror daemon status myfs"
	// fsMirrorDaemonStatus    = `{ "daemon_id": "444607", "filesystems": [ { "filesystem_id": "1", "name": "myfs", "directory_count": 0, "peers": [ { "uuid": "4a6983c0-3c9d-40f5-b2a9-2334a4659827", "remote": { "client_name": "client.mirror_remote", "cluster_name": "site-remote", "fs_name": "backup_fs" }, "stats": { "failure_count": 0, "recovery_count": 0 } } ] } ] }`
	fsMirrorDaemonStatus = `[{"daemon_id":25103, "filesystems": [{"filesystem_id": 1, "name": "myfs", "directory_count": 0, "peers": []}]}]`

	// response of "ceph fs snapshot mirror daemon status"
	fsMirrorDaemonStatusNew = `[{"daemon_id":23102, "filesystems": [{"filesystem_id": 2, "name": "myfsNew", "directory_count": 0, "peers": []}]}]`
)

func TestEnableFilesystemSnapshotMirror(t *testing.T) {
	fs := "myfs"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(timeout time.Duration, command string, args ...string) (string, error) {
		if args[0] == "fs" {
			assert.Equal(t, "snapshot", args[1])
			assert.Equal(t, "mirror", args[2])
			assert.Equal(t, "enable", args[3])
			assert.Equal(t, fs, args[4])
			return "", nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	err := EnableFilesystemSnapshotMirror(context, AdminTestClusterInfo("mycluster"), fs)
	assert.NoError(t, err)
}

func TestDisableFilesystemSnapshotMirror(t *testing.T) {
	fs := "myfs"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(timeout time.Duration, command string, args ...string) (string, error) {
		if args[0] == "fs" {
			assert.Equal(t, "snapshot", args[1])
			assert.Equal(t, "mirror", args[2])
			assert.Equal(t, "disable", args[3])
			assert.Equal(t, fs, args[4])
			return "", nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	err := DisableFilesystemSnapshotMirror(context, AdminTestClusterInfo("mycluster"), fs)
	assert.NoError(t, err)
}

func TestImportFilesystemMirrorPeer(t *testing.T) {
	fs := "myfs"
	token := "my-token"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(timeout time.Duration, command string, args ...string) (string, error) {
		if args[0] == "fs" {
			assert.Equal(t, "snapshot", args[1])
			assert.Equal(t, "mirror", args[2])
			assert.Equal(t, "peer_bootstrap", args[3])
			assert.Equal(t, "import", args[4])
			assert.Equal(t, fs, args[5])
			assert.Equal(t, token, args[6])
			return "", nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	err := ImportFSMirrorBootstrapPeer(context, AdminTestClusterInfo("mycluster"), fs, token)
	assert.NoError(t, err)
}

func TestCreateFSMirrorBootstrapPeer(t *testing.T) {
	fs := "myfs"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(timeout time.Duration, command string, args ...string) (string, error) {
		if args[0] == "fs" {
			assert.Equal(t, "snapshot", args[1])
			assert.Equal(t, "mirror", args[2])
			assert.Equal(t, "peer_bootstrap", args[3])
			assert.Equal(t, "create", args[4])
			assert.Equal(t, fs, args[5])
			return fsMirrorToken, nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	token, err := CreateFSMirrorBootstrapPeer(context, AdminTestClusterInfo("mycluster"), fs)
	assert.NoError(t, err)
	_, err = base64.StdEncoding.DecodeString(string(token))
	assert.NoError(t, err)

}

func TestRemoveFilesystemMirrorPeer(t *testing.T) {
	peerUUID := "peer-uuid"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(timeout time.Duration, command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "fs" {
			assert.Equal(t, "snapshot", args[1])
			assert.Equal(t, "mirror", args[2])
			assert.Equal(t, "peer_remove", args[3])
			assert.Equal(t, peerUUID, args[4])
			return "", nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	err := RemoveFilesystemMirrorPeer(context, AdminTestClusterInfo("mycluster"), peerUUID)
	assert.NoError(t, err)
}

func TestFSMirrorDaemonStatus(t *testing.T) {
	fs := "myfs"
	executor := &exectest.MockExecutor{}
	t.Run("snapshot status command with fsName - test for Ceph v16.2.6 and earlier", func(t *testing.T) {
		executor.MockExecuteCommandWithOutput = func(timeout time.Duration, command string, args ...string) (string, error) {
			if args[0] == "fs" {
				assert.Equal(t, "snapshot", args[1])
				assert.Equal(t, "mirror", args[2])
				assert.Equal(t, "daemon", args[3])
				assert.Equal(t, "status", args[4])
				assert.Equal(t, fs, args[5]) // fs-name needed for Ceph v16.2.6 and earlier
				return fsMirrorDaemonStatus, nil
			}
			return "", errors.New("unknown command")
		}
		context := &clusterd.Context{Executor: executor}
		clusterInfo := AdminTestClusterInfo("mycluster")
		clusterInfo.CephVersion = cephver.CephVersion{Major: 16, Minor: 2, Extra: 6}
		s, err := GetFSMirrorDaemonStatus(context, clusterInfo, fs)
		assert.NoError(t, err)
		assert.Equal(t, 25103, s[0].DaemonID)
		assert.Equal(t, "myfs", s[0].Filesystems[0].Name)
	})
	t.Run("snapshot status command without fsName - test for Ceph v16.2.7 and above", func(t *testing.T) {
		executor.MockExecuteCommandWithOutput = func(timeout time.Duration, command string, args ...string) (string, error) {
			if args[0] == "fs" {
				assert.Equal(t, "snapshot", args[1])
				assert.Equal(t, "mirror", args[2])
				assert.Equal(t, "daemon", args[3])
				assert.Equal(t, "status", args[4])
				assert.NotEqual(t, fs, args[5])
				return fsMirrorDaemonStatusNew, nil
			}
			return "", errors.New("unknown command")
		}
		context := &clusterd.Context{Executor: executor}
		clusterInfo := AdminTestClusterInfo("mycluster")
		clusterInfo.CephVersion = cephver.CephVersion{Major: 16, Minor: 2, Extra: 7}
		s, err := GetFSMirrorDaemonStatus(context, clusterInfo, fs)
		assert.NoError(t, err)
		assert.Equal(t, 23102, s[0].DaemonID)
		assert.Equal(t, "myfsNew", s[0].Filesystems[0].Name)
	})
}
