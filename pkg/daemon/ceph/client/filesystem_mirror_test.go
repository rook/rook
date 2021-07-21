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

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

var (
	// response of "ceph fs snapshot mirror peer_bootstrap create myfs2 client.mirror test"
	// #nosec G101 since this is not leaking any credentials
	fsMirrorToken = `{"token": "eyJmc2lkIjogIjgyYjdlZDkyLTczYjAtNGIyMi1hOGI3LWVkOTQ4M2UyODc1NiIsICJmaWxlc3lzdGVtIjogIm15ZnMyIiwgInVzZXIiOiAiY2xpZW50Lm1pcnJvciIsICJzaXRlX25hbWUiOiAidGVzdCIsICJrZXkiOiAiQVFEVVAxSmdqM3RYQVJBQWs1cEU4cDI1ZUhld2lQK0ZXRm9uOVE9PSIsICJtb25faG9zdCI6ICJbdjI6MTAuOTYuMTQyLjIxMzozMzAwLHYxOjEwLjk2LjE0Mi4yMTM6Njc4OV0sW3YyOjEwLjk2LjIxNy4yMDc6MzMwMCx2MToxMC45Ni4yMTcuMjA3OjY3ODldLFt2MjoxMC45OS4xMC4xNTc6MzMwMCx2MToxMC45OS4xMC4xNTc6Njc4OV0ifQ=="}`

	// response of "ceph fs snapshot mirror daemon status myfs"
	// fsMirrorDaemonStatus    = `{ "daemon_id": "444607", "filesystems": [ { "filesystem_id": "1", "name": "myfs", "directory_count": 0, "peers": [ { "uuid": "4a6983c0-3c9d-40f5-b2a9-2334a4659827", "remote": { "client_name": "client.mirror_remote", "cluster_name": "site-remote", "fs_name": "backup_fs" }, "stats": { "failure_count": 0, "recovery_count": 0 } } ] } ] }`
	fsMirrorDaemonStatusNew = `[{"daemon_id":25103, "filesystems": [{"filesystem_id": 1, "name": "myfs", "directory_count": 0, "peers": []}]}]`
)

func TestEnableFilesystemSnapshotMirror(t *testing.T) {
	fs := "myfs"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
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

	err := EnableFilesystemSnapshotMirror(context, AdminClusterInfo("mycluster"), fs)
	assert.NoError(t, err)
}

func TestDisableFilesystemSnapshotMirror(t *testing.T) {
	fs := "myfs"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
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

	err := DisableFilesystemSnapshotMirror(context, AdminClusterInfo("mycluster"), fs)
	assert.NoError(t, err)
}

func TestImportFilesystemMirrorPeer(t *testing.T) {
	fs := "myfs"
	token := "my-token"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
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

	err := ImportFSMirrorBootstrapPeer(context, AdminClusterInfo("mycluster"), fs, token)
	assert.NoError(t, err)
}

func TestCreateFSMirrorBootstrapPeer(t *testing.T) {
	fs := "myfs"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
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

	token, err := CreateFSMirrorBootstrapPeer(context, AdminClusterInfo("mycluster"), fs)
	assert.NoError(t, err)
	_, err = base64.StdEncoding.DecodeString(string(token))
	assert.NoError(t, err)

}

func TestRemoveFilesystemMirrorPeer(t *testing.T) {
	peerUUID := "peer-uuid"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
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

	err := RemoveFilesystemMirrorPeer(context, AdminClusterInfo("mycluster"), peerUUID)
	assert.NoError(t, err)
}

func TestFSMirrorDaemonStatus(t *testing.T) {
	fs := "myfs"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if args[0] == "fs" {
			assert.Equal(t, "snapshot", args[1])
			assert.Equal(t, "mirror", args[2])
			assert.Equal(t, "daemon", args[3])
			assert.Equal(t, "status", args[4])
			assert.Equal(t, fs, args[5])
			return fsMirrorDaemonStatusNew, nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	s, err := GetFSMirrorDaemonStatus(context, AdminClusterInfo("mycluster"), fs)
	assert.NoError(t, err)
	assert.Equal(t, "myfs", s[0].Filesystems[0].Name)
}
