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
	"errors"
	"testing"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

const (
	mockSubVolumeListResp = `[{"name":"csi-vol-9d942d45-6f48-4d9e-a707-edfcd602bd89"}]`
	mockGetOmapValResp    = `Writing to /dev/stdout
pvc-e2907819-b005-4582-97ec-8fa1b277f46fsh`
	mockSubvolumeSnapshotsResp    = `[{"name":"snap0"}]`
	mockSubvolumeSnapshotInfoResp = `{"created_at":"2024-04-0808:46:36.267888","data_pool":"myfs-replicated","has_pending_clones":"yes","pending_clones":[{"name":"clone3"}]}`
)

func TestSubVolumeGroupCleanup(t *testing.T) {
	clusterInfo := cephclient.AdminTestClusterInfo("mycluster")
	fsName := "myfs"
	subVolumeGroupName := "csi"
	poolName := "myfs-metadata"
	csiNamespace := "csi"

	t.Run("no subvolumes in subvolumegroup", func(t *testing.T) {
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommandWithTimeout = func(timeout time.Duration, command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "fs" && args[1] == "subvolume" && args[2] == "ls" {
				assert.Equal(t, fsName, args[3])
				assert.Equal(t, subVolumeGroupName, args[4])
				return "[]", nil
			}
			return "", errors.New("unknown command")
		}
		context := &clusterd.Context{Executor: executor}
		err := SubVolumeGroupCleanup(context, clusterInfo, fsName, subVolumeGroupName, poolName, csiNamespace)
		assert.NoError(t, err)
	})

	t.Run("subvolumes with snapshots and pending clones", func(t *testing.T) {
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommandWithTimeout = func(timeout time.Duration, command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			// list all subvolumes in subvolumegroup
			if args[0] == "fs" && args[1] == "subvolume" && args[2] == "ls" {
				assert.Equal(t, fsName, args[3])
				assert.Equal(t, subVolumeGroupName, args[4])
				return mockSubVolumeListResp, nil
			}
			if args[0] == "getomapval" {
				assert.Equal(t, "csi.volume.9d942d45-6f48-4d9e-a707-edfcd602bd89", args[1])
				assert.Equal(t, "csi.volname", args[2])
				assert.Equal(t, poolName, args[4])
				assert.Equal(t, csiNamespace, args[6])
				return mockGetOmapValResp, nil
			}
			// delete OMAP value
			if args[0] == "rm" && args[1] == "csi.volume.9d942d45-6f48-4d9e-a707-edfcd602bd89" {
				assert.Equal(t, "-p", args[2])
				assert.Equal(t, poolName, args[3])
				assert.Equal(t, "--namespace", args[4])
				assert.Equal(t, csiNamespace, args[5])
				return "", nil
			}
			// delete OMAP key
			if args[0] == "rmomapkey" && args[1] == "csi.volumes.default" {
				assert.Equal(t, "ceph.volume.pvc-e2907819-b005-4582-97ec-8fa1b277f46fsh", args[2])
				assert.Equal(t, "-p", args[3])
				assert.Equal(t, poolName, args[4])
				assert.Equal(t, "--namespace", args[5])
				assert.Equal(t, csiNamespace, args[6])
				return "", nil
			}
			// list all snapshots in a subvolume
			if args[0] == "fs" && args[1] == "subvolume" && args[2] == "snapshot" && args[3] == "ls" {
				assert.Equal(t, fsName, args[4])
				assert.Equal(t, "csi-vol-9d942d45-6f48-4d9e-a707-edfcd602bd89", args[5])
				assert.Equal(t, "--group_name", args[6])
				assert.Equal(t, subVolumeGroupName, args[7])
				return mockSubvolumeSnapshotsResp, nil
			}
			// list all pending clones in a subvolume snapshot
			if args[0] == "fs" && args[1] == "subvolume" && args[2] == "snapshot" && args[3] == "info" {
				assert.Equal(t, fsName, args[4])
				assert.Equal(t, "csi-vol-9d942d45-6f48-4d9e-a707-edfcd602bd89", args[5])
				assert.Equal(t, "snap0", args[6])
				assert.Equal(t, "--group_name", args[7])
				assert.Equal(t, subVolumeGroupName, args[8])
				return mockSubvolumeSnapshotInfoResp, nil
			}
			// cancel pending clones
			if args[0] == "fs" && args[1] == "clone" && args[2] == "cancel" {
				assert.Equal(t, fsName, args[3])
				assert.Equal(t, "clone3", args[4])
				assert.Equal(t, "--group_name", args[5])
				assert.Equal(t, subVolumeGroupName, args[6])
				return "", nil
			}
			// delete snapshots
			if args[0] == "fs" && args[1] == "subvolume" && args[2] == "snapshot" && args[3] == "rm" {
				assert.Equal(t, fsName, args[4])
				assert.Equal(t, "csi-vol-9d942d45-6f48-4d9e-a707-edfcd602bd89", args[5])
				assert.Equal(t, "snap0", args[6])
				assert.Equal(t, "--group_name", args[7])
				assert.Equal(t, subVolumeGroupName, args[8])
				return "", nil
			}
			// delete subvolume
			if args[0] == "fs" && args[1] == "subvolume" && args[2] == "rm" {
				assert.Equal(t, fsName, args[3])
				assert.Equal(t, "csi-vol-9d942d45-6f48-4d9e-a707-edfcd602bd89", args[4])
				assert.Equal(t, subVolumeGroupName, args[5])
				assert.Equal(t, "--force", args[6])
				return "", nil
			}
			return "", errors.New("unknown command")
		}
		context := &clusterd.Context{Executor: executor}
		err := SubVolumeGroupCleanup(context, clusterInfo, fsName, subVolumeGroupName, poolName, csiNamespace)
		assert.NoError(t, err)
	})
}

func TestGetOmapValue(t *testing.T) {
	result := getOMAPValue("csi-vol-3a41b367-9566-4dbb-8884-39e1fa306ea7")
	assert.Equal(t, "csi.volume.3a41b367-9566-4dbb-8884-39e1fa306ea7", result)

	result = getOMAPValue("invalidSubVolume")
	assert.Equal(t, "", result)
}
