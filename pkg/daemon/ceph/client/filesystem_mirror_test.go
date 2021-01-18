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
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
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

func TestAddFilesystemMirrorPeer(t *testing.T) {
	fs := "myfs"
	peer := "my-peer"
	remoteFS := "remoteFS"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if args[0] == "fs" {
			assert.Equal(t, "mirror", args[1])
			assert.Equal(t, "peer_add", args[2])
			assert.Equal(t, fs, args[3])
			assert.Equal(t, peer, args[4])
			assert.Equal(t, remoteFS, args[5])
			return "", nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	err := AddFilesystemMirrorPeer(context, AdminClusterInfo("mycluster"), fs, peer, remoteFS)
	assert.NoError(t, err)
}

func TestRemoveFilesystemMirrorPeer(t *testing.T) {
	peerUUID := "peer-uuid"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if args[0] == "fs" {
			assert.Equal(t, "mirror", args[1])
			assert.Equal(t, "peer_remove", args[2])
			assert.Equal(t, peerUUID, args[3])
			return "", nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	err := RemoveFilesystemMirrorPeer(context, AdminClusterInfo("mycluster"), peerUUID)
	assert.NoError(t, err)
}
