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
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"testing"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fake peer bootstrap token, shaped to be recognizable in a log but useless as a
// credential
const examplePeerToken = "EXAMPLEPEERTOKEN0000000000000000000000000001"

var (
	// response of "ceph fs snapshot mirror peer_bootstrap create myfs2 client.mirror test"
	//nolint:gosec // since this is not leaking any credentials
	fsMirrorToken = `{"token": "eyJmc2lkIjogIjgyYjdlZDkyLTczYjAtNGIyMi1hOGI3LWVkOTQ4M2UyODc1NiIsICJmaWxlc3lzdGVtIjogIm15ZnMyIiwgInVzZXIiOiAiY2xpZW50Lm1pcnJvciIsICJzaXRlX25hbWUiOiAidGVzdCIsICJrZXkiOiAiQVFEVVAxSmdqM3RYQVJBQWs1cEU4cDI1ZUhld2lQK0ZXRm9uOVE9PSIsICJtb25faG9zdCI6ICJbdjI6MTAuOTYuMTQyLjIxMzozMzAwLHYxOjEwLjk2LjE0Mi4yMTM6Njc4OV0sW3YyOjEwLjk2LjIxNy4yMDc6MzMwMCx2MToxMC45Ni4yMTcuMjA3OjY3ODldLFt2MjoxMC45OS4xMC4xNTc6MzMwMCx2MToxMC45OS4xMC4xNTc6Njc4OV0ifQ=="}`

	// response of "ceph fs snapshot mirror daemon status"
	fsMirrorDaemonStatusNew = `[{"daemon_id":23102, "filesystems": [{"filesystem_id": 2, "name": "myfsNew", "directory_count": 0, "peers": []}]}]`
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

	err := EnableFilesystemSnapshotMirror(context, AdminTestClusterInfo("mycluster"), fs)
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

	err := DisableFilesystemSnapshotMirror(context, AdminTestClusterInfo("mycluster"), fs)
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

	err := ImportFSMirrorBootstrapPeer(context, AdminTestClusterInfo("mycluster"), fs, token)
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

	token, err := CreateFSMirrorBootstrapPeer(context, AdminTestClusterInfo("mycluster"), fs)
	assert.NoError(t, err)
	_, err = base64.StdEncoding.DecodeString(string(token))
	assert.NoError(t, err)
}

// mirrorPeerBootstrapExecutor returns an executor whose `fs snapshot mirror
// peer_bootstrap create` reply is the given response.
func mirrorPeerBootstrapExecutor(response string) *exectest.MockExecutor {
	return &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "fs" {
				return response, nil
			}
			return "", errors.New("unknown command")
		},
	}
}

func TestCreateFSMirrorBootstrapPeerDoesNotLeakToken(t *testing.T) {
	// the response being unmarshalled is the peer bootstrap token, which belongs
	// only in a Secret. The error reaches the operator log, and through reporting
	// the CR status and a Kubernetes event, none of which need access to that Secret.
	tests := []struct {
		name     string
		response string
		// the field a type error names; empty when the error is a syntax error,
		// which reports only its own message. Qualified with the struct name: the
		// wrapper text already contains "token", so a bare field name asserts nothing
		namesField string
	}{
		{
			// Token is a string in BootstrapPeerToken, so an array is the shape a
			// schema change across Ceph versions would take
			name:       "type error",
			response:   `{"token": ["` + examplePeerToken + `"]}`,
			namesField: "BootstrapPeerToken.token",
		},
		{
			name:     "truncated response",
			response: `{"token": "` + examplePeerToken + `"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			context := &clusterd.Context{Executor: mirrorPeerBootstrapExecutor(tc.response)}

			_, err := CreateFSMirrorBootstrapPeer(context, AdminTestClusterInfo("mycluster"), "myfs")

			require.Error(t, err)
			assert.NotContains(t, err.Error(), examplePeerToken)
			// the response size is the only context a syntax error leaves
			assert.Contains(t, err.Error(), fmt.Sprintf("(%d bytes)", len(tc.response)))
			if tc.namesField != "" {
				assert.Contains(t, err.Error(), tc.namesField)
			}
		})
	}
}

// captureLogsAtLevel redirects the capnslog sink into a buffer at the given level
// for the duration of the test, restoring the package default afterwards so later
// tests still log to stderr and never at TRACE.
func captureLogsAtLevel(t *testing.T, level capnslog.LogLevel) *bytes.Buffer {
	t.Helper()

	logBuf := bytes.NewBuffer([]byte{})
	capnslog.SetFormatter(capnslog.NewLogFormatter(logBuf, "", 0))
	capnslog.SetGlobalLogLevel(level)
	t.Cleanup(func() {
		capnslog.SetFormatter(capnslog.NewDefaultFormatter(os.Stderr))
		capnslog.SetGlobalLogLevel(capnslog.INFO)
	})

	return logBuf
}

func TestCreateFSMirrorBootstrapPeerLogsRawResponseOnlyUnderTrace(t *testing.T) {
	// the raw response is the only way to tell a truncated document from a schema
	// change, but it is the peer bootstrap token itself, so it is logged only at the
	// TRACE level that ROOK_LOG_LEVEL=TRACE_INSECURE unlocks.
	badResponse := `{"token": "` + examplePeerToken + `"`

	for _, tc := range []struct {
		level  capnslog.LogLevel
		logged bool
	}{
		{level: capnslog.DEBUG, logged: false},
		{level: capnslog.TRACE, logged: true},
	} {
		t.Run(tc.level.String(), func(t *testing.T) {
			logBuf := captureLogsAtLevel(t, tc.level)
			context := &clusterd.Context{Executor: mirrorPeerBootstrapExecutor(badResponse)}

			_, err := CreateFSMirrorBootstrapPeer(context, AdminTestClusterInfo("mycluster"), "myfs")

			require.Error(t, err)
			assert.NotContains(t, err.Error(), examplePeerToken)
			if tc.logged {
				assert.Contains(t, logBuf.String(), examplePeerToken)
			} else {
				assert.NotContains(t, logBuf.String(), examplePeerToken)
			}
		})
	}
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

	err := RemoveFilesystemMirrorPeer(context, AdminTestClusterInfo("mycluster"), peerUUID)
	assert.NoError(t, err)
}

func TestFSMirrorDaemonStatus(t *testing.T) {
	fs := "myfs"
	executor := &exectest.MockExecutor{}
	t.Run("snapshot status command", func(t *testing.T) {
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
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
		s, err := GetFSMirrorDaemonStatus(context, clusterInfo, fs)
		assert.NoError(t, err)
		assert.Equal(t, 23102, s[0].DaemonID)
		assert.Equal(t, "myfsNew", s[0].Filesystems[0].Name)
	})
}
