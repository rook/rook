/*
Copyright 2024 The Rook Authors. All rights reserved.

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

package cleanup

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

const (
	mockImageLSResponse   = `[{"image":"csi-vol-136268e8-5386-4453-a6bd-9dca381d187d","id":"16e35cfa56a7","size":1073741824,"format":2}]`
	mockSnapshotsResponse = `[{"id":5,"name":"snap1","size":1073741824,"protected":"false","timestamp":"Fri Apr 12 13:39:28 2024"}]`
)

func TestRadosNamespace(t *testing.T) {
	clusterInfo := cephclient.AdminTestClusterInfo("mycluster")
	poolName := "test-pool"
	radosNamespace := "test-namespace"

	t.Run("no images in rados namespace", func(t *testing.T) {
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "ls" && args[1] == "-l" {
				assert.Equal(t, poolName, args[2])
				return "", nil
			}
			return "", errors.New("unknown command")
		}
		context := &clusterd.Context{Executor: executor}
		err := RadosNamespaceCleanup(context, clusterInfo, poolName, radosNamespace)
		assert.NoError(t, err)
	})

	t.Run("images with snapshots available in rados namespace", func(t *testing.T) {
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			// list all subvolumes in subvolumegroup
			if args[0] == "ls" && args[1] == "-l" {
				assert.Equal(t, poolName, args[2])
				return mockImageLSResponse, nil
			}
			if args[0] == "snap" && args[1] == "ls" {
				assert.Equal(t, "test-pool/csi-vol-136268e8-5386-4453-a6bd-9dca381d187d", args[2])
				assert.Equal(t, "--namespace", args[3])
				assert.Equal(t, radosNamespace, args[4])
				return mockSnapshotsResponse, nil
			}
			if args[0] == "snap" && args[1] == "rm" {
				assert.Equal(t, "test-pool/csi-vol-136268e8-5386-4453-a6bd-9dca381d187d@snap1", args[2])
				assert.Equal(t, "--namespace", args[3])
				assert.Equal(t, radosNamespace, args[4])
				return "", nil
			}
			if args[0] == "trash" && args[1] == "mv" {
				assert.Equal(t, "test-pool/csi-vol-136268e8-5386-4453-a6bd-9dca381d187d", args[2])
				assert.Equal(t, "--namespace", args[3])
				assert.Equal(t, radosNamespace, args[4])
				return "", nil
			}
			if args[0] == "rbd" && args[1] == "task" && args[2] == "add" && args[3] == "trash" {
				// pool-name/rados-namespace/image-id
				assert.Equal(t, "test-pool/test-namespace/16e35cfa56a7", args[5])
				return "", nil
			}
			return "", errors.New("unknown command")
		}
		context := &clusterd.Context{Executor: executor}
		err := RadosNamespaceCleanup(context, clusterInfo, poolName, radosNamespace)
		assert.NoError(t, err)
	})
}
