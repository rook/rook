/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

var (
	bootstrapPeerToken            = `eyJmc2lkIjoiYzZiMDg3ZjItNzgyOS00ZGJiLWJjZmMtNTNkYzM0ZTBiMzVkIiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFBV1lsWmZVQ1Q2RGhBQVBtVnAwbGtubDA5YVZWS3lyRVV1NEE9PSIsIm1vbl9ob3N0IjoiW3YyOjE5Mi4xNjguMTExLjEwOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTA6Njc4OV0sW3YyOjE5Mi4xNjguMTExLjEyOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTI6Njc4OV0sW3YyOjE5Mi4xNjguMTExLjExOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTE6Njc4OV0ifQ==` //nolint:gosec // This is just a var name, not a real token
	mirrorStatus                  = `{"summary":{"health":"WARNING","daemon_health":"OK","image_health":"WARNING","states":{"starting_replay":1,"replaying":1}}}`
	mirrorStatusVerbose           = `{"summary":{"health":"WARNING","daemon_health":"OK","image_health":"WARNING","states":{"starting_replay":1,"replaying":1}}, "images":[{"name":"test","global_id":"ebad309d-4d8f-4c6f-afe0-46e02ace26ac","state":"up+stopped","description":"local image is primary","daemon_service":{"service_id":"4339","instance_id":"4644","daemon_id":"a","hostname":"fv-az1195-222"},"last_update":"2024-03-18 04:16:54","peer_sites":[{"site_name":"4c3e1cb8-8129-43ab-8d75-abc20154fd4a","mirror_uuids":"a71727e9-63a4-4386-a44b-4cf48dc77fa8","state":"up+replaying","description":"replaying, {\"bytes_per_second\":0.0,\"bytes_per_snapshot\":0.0,\"last_snapshot_bytes\":0,\"last_snapshot_sync_seconds\":0,\"remote_snapshot_timestamp\":1710734899,\"replay_state\":\"idle\"}","last_update":"2024-03-18 04:16:36"}]}]}`
	mirrorInfo                    = `{"mode":"image","site_name":"39074576-5884-4ef3-8a4d-8a0c5ed33031","peers":[{"uuid":"4a6983c0-3c9d-40f5-b2a9-2334a4659827","direction":"rx-tx","site_name":"ocs","mirror_uuid":"","client_name":"client.rbd-mirror-peer"}]}`
	snapshotScheduleStatus        = `[{"schedule_time": "14:00:00-05:00", "image": "foo"}, {"schedule_time": "08:00:00-05:00", "image": "bar"}]`
	snapshotScheduleList          = `[{"interval":"3d","start_time":""},{"interval":"1d","start_time":"14:00:00-05:00"}]`
	snapshotScheduleListRecursive = `[{"pool":"replicapool","namespace":"-","image":"-","items":[{"interval":"1d","start_time":"14:00:00-05:00"}]},{"pool":"replicapool","namespace":"","image":"snapeuh","items":[{"interval":"1d","start_time":"14:00:00-05:00"},{"interval":"4h","start_time":"14:00:00-05:00"},{"interval":"4h","start_time":"04:00:00-05:00"}]}]`
)

func TestCreateRBDMirrorBootstrapPeer(t *testing.T) {
	pool := "pool-test"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if args[0] == "mirror" {
			assert.Equal(t, "pool", args[1])
			assert.Equal(t, "peer", args[2])
			assert.Equal(t, "bootstrap", args[3])
			assert.Equal(t, "create", args[4])
			assert.Equal(t, pool, args[5])
			return bootstrapPeerToken, nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}
	c := AdminTestClusterInfo("mycluster")
	c.FSID = "4fe04ebb-ec0c-46c2-ac55-9eb52ebbfb82"

	token, err := CreateRBDMirrorBootstrapPeer(context, c, pool)
	assert.NoError(t, err)
	assert.Equal(t, bootstrapPeerToken, string(token))
}

func TestEnablePoolMirroring(t *testing.T) {
	pool := cephv1.NamedPoolSpec{
		Name: "pool-test",
		PoolSpec: cephv1.PoolSpec{
			Mirroring: cephv1.MirroringSpec{Mode: "image"},
		},
	}
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if args[0] == "mirror" {
			assert.Equal(t, "pool", args[1])
			assert.Equal(t, "enable", args[2])
			assert.Equal(t, pool.Name, args[3])
			assert.Equal(t, pool.Mirroring.Mode, args[4])
			return "", nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	err := enablePoolMirroring(context, AdminTestClusterInfo("mycluster"), pool)
	assert.NoError(t, err)
}

func TestGetPoolMirroringStatus(t *testing.T) {
	pool := "pool-test"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if args[0] == "mirror" {
			assert.Equal(t, "pool", args[1])
			assert.Equal(t, "status", args[2])
			assert.Equal(t, pool, args[3])
			return mirrorStatus, nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	poolMirrorStatus, err := GetPoolMirroringStatus(context, AdminTestClusterInfo("mycluster"), pool)
	assert.NoError(t, err)
	assert.Equal(t, "WARNING", poolMirrorStatus.Summary.Health)
	assert.Equal(t, "OK", poolMirrorStatus.Summary.DaemonHealth)
}

func TestGetMirroredPoolImages(t *testing.T) {
	pool := "pool-test"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if args[0] == "mirror" {
			assert.Equal(t, "pool", args[1])
			assert.Equal(t, "status", args[2])
			assert.Equal(t, "--verbose", args[3])
			assert.Equal(t, pool, args[4])
			return mirrorStatusVerbose, nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	mirroredImages, err := GetMirroredPoolImages(context, AdminTestClusterInfo("mycluster"), pool)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(*mirroredImages.Images))
}

func TestImportRBDMirrorBootstrapPeer(t *testing.T) {
	pool := "pool-test"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if args[0] == "mirror" {
			assert.Equal(t, "pool", args[1])
			assert.Equal(t, "peer", args[2])
			assert.Equal(t, "bootstrap", args[3])
			assert.Equal(t, "import", args[4])
			assert.Equal(t, pool, args[5])
			assert.Equal(t, 11, len(args))
			return mirrorStatus, nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	err := ImportRBDMirrorBootstrapPeer(context, AdminTestClusterInfo("mycluster"), pool, "", []byte(bootstrapPeerToken))
	assert.NoError(t, err)

	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if args[0] == "mirror" {
			assert.Equal(t, "pool", args[1])
			assert.Equal(t, "peer", args[2])
			assert.Equal(t, "bootstrap", args[3])
			assert.Equal(t, "import", args[4])
			assert.Equal(t, pool, args[5])
			assert.Equal(t, "--direction", args[7])
			assert.Equal(t, "rx-tx", args[8])
			assert.Equal(t, 13, len(args))
			return mirrorStatus, nil
		}
		return "", errors.New("unknown command")
	}
	context = &clusterd.Context{Executor: executor}
	err = ImportRBDMirrorBootstrapPeer(context, AdminTestClusterInfo("mycluster"), pool, "rx-tx", []byte(bootstrapPeerToken))
	assert.NoError(t, err)
}

func TestGetPoolMirroringInfo(t *testing.T) {
	pool := "pool-test"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if args[0] == "mirror" {
			assert.Equal(t, "pool", args[1])
			assert.Equal(t, "info", args[2])
			assert.Equal(t, pool, args[3])
			return mirrorInfo, nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	poolMirrorInfo, err := GetPoolMirroringInfo(context, AdminTestClusterInfo("mycluster"), pool)
	assert.NoError(t, err)
	assert.Equal(t, "image", poolMirrorInfo.Mode)
	assert.Equal(t, 1, len(poolMirrorInfo.Peers))
}

func TestEnableSnapshotSchedule(t *testing.T) {
	pool := "pool-test"
	interval := "24h"

	// Schedule with Interval
	{
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			logger.Infof("Command: %v %v", command, args)
			if args[0] == "mirror" {
				assert.Equal(t, "snapshot", args[1])
				assert.Equal(t, "schedule", args[2])
				assert.Equal(t, "add", args[3])
				assert.Equal(t, "--pool", args[4])
				assert.Equal(t, pool, args[5])
				assert.Equal(t, interval, args[6])
				assert.Equal(t, len(args), 11)
				return "success", nil
			}
			return "", errors.New("unknown command")
		}
		context := &clusterd.Context{Executor: executor}
		poolSpec := &cephv1.PoolSpec{Mirroring: cephv1.MirroringSpec{SnapshotSchedules: []cephv1.SnapshotScheduleSpec{{Interval: interval}}}}

		err := enableSnapshotSchedule(context, AdminTestClusterInfo("mycluster"), poolSpec.Mirroring.SnapshotSchedules[0], pool)
		assert.NoError(t, err)
	}

	// Schedule with Interval and start time
	{
		startTime := "14:00:00-05:00"
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			logger.Infof("Command: %v %v", command, args)
			if args[0] == "mirror" {
				assert.Equal(t, "snapshot", args[1])
				assert.Equal(t, "schedule", args[2])
				assert.Equal(t, "add", args[3])
				assert.Equal(t, "--pool", args[4])
				assert.Equal(t, pool, args[5])
				assert.Equal(t, interval, args[6])
				assert.Equal(t, startTime, args[7])
				assert.Equal(t, len(args), 12)
				return "success", nil
			}
			return "", errors.New("unknown command")
		}
		context := &clusterd.Context{Executor: executor}
		poolSpec := &cephv1.PoolSpec{Mirroring: cephv1.MirroringSpec{SnapshotSchedules: []cephv1.SnapshotScheduleSpec{{Interval: interval, StartTime: startTime}}}}

		err := enableSnapshotSchedule(context, AdminTestClusterInfo("mycluster"), poolSpec.Mirroring.SnapshotSchedules[0], pool)
		assert.NoError(t, err)
	}
}

func TestListSnapshotSchedules(t *testing.T) {
	pool := "pool-test"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %v %v", command, args)
		if args[0] == "mirror" {
			assert.Equal(t, "snapshot", args[1])
			assert.Equal(t, "schedule", args[2])
			assert.Equal(t, "ls", args[3])
			assert.Equal(t, "--pool", args[4])
			assert.Equal(t, pool, args[5])
			return snapshotScheduleStatus, nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	snapshotScheduleStatus, err := listSnapshotSchedules(context, AdminTestClusterInfo("mycluster"), pool)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(snapshotScheduleStatus))
}

func TestListSnapshotSchedulesRecursively(t *testing.T) {
	pool := "pool-test"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %v %v", command, args)
		if args[0] == "mirror" {
			assert.Equal(t, "snapshot", args[1])
			assert.Equal(t, "schedule", args[2])
			assert.Equal(t, "ls", args[3])
			assert.Equal(t, "--pool", args[4])
			assert.Equal(t, pool, args[5])
			assert.Equal(t, "--recursive", args[6])
			return snapshotScheduleListRecursive, nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	snapshotScheduleStatus, err := ListSnapshotSchedulesRecursively(context, AdminTestClusterInfo("mycluster"), pool)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(snapshotScheduleStatus))
}

func TestRemoveSnapshotSchedule(t *testing.T) {
	pool := "pool-test"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %v %v", command, args)
		if args[0] == "mirror" {
			assert.Equal(t, "snapshot", args[1])
			assert.Equal(t, "schedule", args[2])
			assert.Equal(t, "remove", args[3])
			assert.Equal(t, "--pool", args[4])
			assert.Equal(t, pool, args[5])
			return "", nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	snapScheduleResponse := cephv1.SnapshotSchedule{StartTime: "14:00:00-05:00", Interval: "1d"}
	err := removeSnapshotSchedule(context, AdminTestClusterInfo("mycluster"), snapScheduleResponse, pool)
	assert.NoError(t, err)
}

func TestRemoveSnapshotSchedules(t *testing.T) {
	interval := "24h"
	startTime := "14:00:00-05:00"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %v %v", command, args)
		if args[0] == "mirror" {
			switch args[3] {
			case "ls":
				return snapshotScheduleList, nil
			case "remove":
				return "success", nil
			}
		}
		return "", errors.New("unknown command")
	}

	context := &clusterd.Context{Executor: executor}
	pool := cephv1.NamedPoolSpec{
		Name: "pool-test",
		PoolSpec: cephv1.PoolSpec{
			Mirroring: cephv1.MirroringSpec{
				SnapshotSchedules: []cephv1.SnapshotScheduleSpec{
					{Interval: interval, StartTime: startTime},
				},
			},
		},
	}
	err := removeSnapshotSchedules(context, AdminTestClusterInfo("mycluster"), pool.Name)
	assert.NoError(t, err)
}

func TestDisableMirroring(t *testing.T) {
	pool := "pool-test"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if args[0] == "mirror" {
			assert.Equal(t, "pool", args[1])
			assert.Equal(t, "disable", args[2])
			assert.Equal(t, pool, args[3])
			return "", nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	err := DisablePoolMirroring(context, AdminTestClusterInfo("mycluster"), pool)
	assert.NoError(t, err)
}

func TestRemoveClusterPeer(t *testing.T) {
	pool := "pool-test"
	peerUUID := "39ae33fb-1dd6-4f9b-8ed7-0e4517068900"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if args[0] == "mirror" {
			assert.Equal(t, "pool", args[1])
			assert.Equal(t, "peer", args[2])
			assert.Equal(t, "remove", args[3])
			assert.Equal(t, pool, args[4])
			assert.Equal(t, peerUUID, args[5])
			return "", nil
		}
		return "", errors.New("unknown command")
	}
	context := &clusterd.Context{Executor: executor}

	err := RemoveClusterPeer(context, AdminTestClusterInfo("mycluster"), pool, peerUUID)
	assert.NoError(t, err)
}
