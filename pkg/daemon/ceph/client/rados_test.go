/*
Copyright 2022 The Rook Authors. All rights reserved.

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
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRadosNamespaceHasObjects(t *testing.T) {
	RunAllCephCommandsInToolboxPod = ""

	capturedShellCmd := func(t *testing.T, pool, namespace string) string {
		var shellCmd string
		me := &test.MockExecutor{
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, arg ...string) (string, error) {
				require.Equal(t, "sh", command)
				require.Len(t, arg, 2)
				require.Equal(t, "-c", arg[0])
				shellCmd = arg[1]
				return "", nil
			},
		}
		ctx := &clusterd.Context{Executor: me, ConfigDir: "/var/lib/rook"}
		_, err := RadosNamespaceHasObjects(ctx, AdminTestClusterInfo("rook-ceph"), pool, namespace)
		require.NoError(t, err)
		return shellCmd
	}

	t.Run("ordinary names stay unquoted", func(t *testing.T) {
		assert.Equal(t,
			"rados --pool mypool --namespace myns ls "+
				"--cluster=rook-ceph "+
				"--conf=/var/lib/rook/rook-ceph/rook-ceph.config "+
				"--name=client.admin "+
				"--keyring=/var/lib/rook/rook-ceph/client.admin.keyring "+
				"2>/dev/null | head -c 1",
			capturedShellCmd(t, "mypool", "myns"))
	})

	t.Run("shell metacharacters cannot break out of an argument", func(t *testing.T) {
		assert.Contains(t, capturedShellCmd(t, "mypool", "myns; rm -rf /"),
			"--namespace 'myns; rm -rf /' ls")
	})
}

func TestRadosRemoveObject(t *testing.T) {
	newTest := func(mockExec *test.MockExecutor) (*clusterd.Context, *ClusterInfo) {
		ctx := &clusterd.Context{
			Executor: mockExec,
		}
		info := &ClusterInfo{
			Context: context.Background(),
		}
		return ctx, info
	}

	t.Run("timeout stat-ing", func(t *testing.T) {
		me := &test.MockExecutor{
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, arg ...string) (string, error) {
				return "", test.FakeTimeoutError("")
			},
		}
		c, i := newTest(me)
		err := RadosRemoveObject(c, i, "mypool", "myns", "someobj")
		assert.Error(t, err)
		assert.True(t, exec.IsTimeout(err))
	})

	t.Run("stat err means the object is already gone", func(t *testing.T) {
		me := &test.MockExecutor{
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, arg ...string) (string, error) {
				subCommand := arg[4]
				assert.Equal(t, subCommand, "stat")
				return "", errors.New("induced stat error")
			},
		}
		c, i := newTest(me)
		err := RadosRemoveObject(c, i, "mypool", "myns", "someobj")
		assert.NoError(t, err)
	})

	t.Run("rm err is an error", func(t *testing.T) {
		me := &test.MockExecutor{
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, arg ...string) (string, error) {
				assert.Equal(t, arg[0], "--pool")
				assert.Equal(t, arg[1], "mypool")
				assert.Equal(t, arg[2], "--namespace")
				assert.Equal(t, arg[3], "myns")
				subCommand := arg[4]
				assert.Equal(t, arg[5], "someobj")

				switch subCommand {
				case "stat":
					return "", nil // successful stat means obj exists
				case "rm":
					return "", errors.New("induced rm error")
				default:
					panic(fmt.Sprintf("unhandled subcommand %q", subCommand))
				}
			},
		}
		c, i := newTest(me)
		err := RadosRemoveObject(c, i, "mypool", "myns", "someobj")
		assert.Error(t, err)
	})

	t.Run("rm successful", func(t *testing.T) {
		me := &test.MockExecutor{
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, arg ...string) (string, error) {
				assert.Equal(t, arg[0], "--pool")
				assert.Equal(t, arg[1], "mypool")
				assert.Equal(t, arg[2], "--namespace")
				assert.Equal(t, arg[3], "myns")
				subCommand := arg[4]
				assert.Equal(t, arg[5], "someobj")

				if subCommand == "stat" || subCommand == "rm" {
					return "", nil
				} else {
					panic(fmt.Sprintf("unhandled subcommand %q", subCommand))
				}
			},
		}
		c, i := newTest(me)
		err := RadosRemoveObject(c, i, "mypool", "myns", "someobj")
		assert.NoError(t, err)
	})
}
