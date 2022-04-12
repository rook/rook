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

// Package pool to manage a rook pool.
package pool

import (
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidatePool(t *testing.T) {
	context := &clusterd.Context{Executor: &exectest.MockExecutor{}}
	clusterInfo := &cephclient.ClusterInfo{Namespace: "myns"}
	clusterSpec := &cephv1.ClusterSpec{}

	t.Run("not specifying replication or EC settings is invalid", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "either of erasurecoded or replicated fields should be set")
	})

	t.Run("must specify name", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Namespace: clusterInfo.Namespace}}
		p.Spec.Replicated.Size = 3
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
	})

	t.Run("must specify namespace", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool"}}
		p.Spec.Replicated.Size = 3
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
	})

	t.Run("must not specify both replication and EC settings", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.Replicated.Size = 1
		p.Spec.Replicated.RequireSafeReplicaSize = false
		p.Spec.ErasureCoded.CodingChunks = 2
		p.Spec.ErasureCoded.DataChunks = 3
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
	})

	t.Run("succeed with replication settings", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.Replicated.Size = 1
		p.Spec.Replicated.RequireSafeReplicaSize = false
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.NoError(t, err)
	})

	t.Run("size is 1 and RequireSafeReplicaSize is true", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.Replicated.Size = 1
		p.Spec.Replicated.RequireSafeReplicaSize = true
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
	})

	t.Run("succeed with ec settings", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.ErasureCoded.CodingChunks = 1
		p.Spec.ErasureCoded.DataChunks = 2
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.NoError(t, err)
	})

	t.Run("fail Parameters['compression_mode'] is unknown", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.Replicated.Size = 1
		p.Spec.Replicated.RequireSafeReplicaSize = false
		p.Spec.Parameters = map[string]string{"compression_mode": "foo"}
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
		assert.EqualError(t, err, "failed to validate pool spec unknown compression mode \"foo\"")
		assert.Equal(t, "foo", p.Spec.Parameters["compression_mode"])
	})

	t.Run("success Parameters['compression_mode'] is known", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.Replicated.Size = 1
		p.Spec.Replicated.RequireSafeReplicaSize = false
		p.Spec.Parameters = map[string]string{"compression_mode": "aggressive"}
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.NoError(t, err)
	})

	t.Run("fail since replica size is lower than ReplicasPerFailureDomain", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.Replicated.Size = 1
		p.Spec.Replicated.ReplicasPerFailureDomain = 2
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
	})

	t.Run("fail since replica size is equal than ReplicasPerFailureDomain", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.Replicated.Size = 2
		p.Spec.Replicated.ReplicasPerFailureDomain = 2
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
	})

	t.Run("fail since ReplicasPerFailureDomain is not a power of 2", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.Replicated.Size = 4
		p.Spec.Replicated.ReplicasPerFailureDomain = 3
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
	})

	t.Run("fail since ReplicasPerFailureDomain is not a power of 2", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.Replicated.Size = 4
		p.Spec.Replicated.ReplicasPerFailureDomain = 5
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
	})

	t.Run("failure the sub domain does not exist", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.Replicated.SubFailureDomain = "dummy"
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
	})

	t.Run("succeed with ec pool and valid compression mode", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.ErasureCoded.CodingChunks = 1
		p.Spec.ErasureCoded.DataChunks = 2
		p.Spec.CompressionMode = "passive"
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.NoError(t, err)
	})

	t.Run("fail unrecognized mirroring mode", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.Replicated.Size = 3
		p.Spec.Mirroring.Enabled = true
		p.Spec.Mirroring.Mode = "foo"
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
		assert.EqualError(t, err, "unrecognized mirroring mode \"foo\". only 'image and 'pool' are supported")
	})

	t.Run("success known mirroring mode", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.Replicated.Size = 3
		p.Spec.Mirroring.Enabled = true
		p.Spec.Mirroring.Mode = "pool"
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.NoError(t, err)
	})

	t.Run("fail mirroring mode no interval specified", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.Replicated.Size = 3
		p.Spec.Mirroring.Enabled = true
		p.Spec.Mirroring.Mode = "pool"
		p.Spec.Mirroring.SnapshotSchedules = []cephv1.SnapshotScheduleSpec{{StartTime: "14:00:00-05:00"}}
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
		assert.EqualError(t, err, "schedule interval cannot be empty if start time is specified")
	})

	t.Run("fail mirroring mode we have a snap interval", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.Replicated.Size = 3
		p.Spec.Mirroring.Enabled = true
		p.Spec.Mirroring.Mode = "pool"
		p.Spec.Mirroring.SnapshotSchedules = []cephv1.SnapshotScheduleSpec{{Interval: "24h"}}
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.NoError(t, err)
	})

	t.Run("failure and subfailure domains", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.Replicated.Size = 3
		p.Spec.FailureDomain = "host"
		p.Spec.Replicated.SubFailureDomain = "host"
		err := validatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
		assert.EqualError(t, err, "failure and subfailure domain cannot be identical")
	})
}

func TestValidateCrushProperties(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	clusterInfo := cephclient.AdminTestClusterInfo("mycluster")
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "crush" && args[2] == "dump" {
			return `{"types":[{"type_id": 0,"name": "osd"}],"buckets":[{"id": -1,"name":"default"},{"id": -2,"name":"good"}, {"id": -3,"name":"host"}]}`, nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	// succeed with a failure domain that exists
	p := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace},
		Spec: cephv1.NamedBlockPoolSpec{
			PoolSpec: cephv1.PoolSpec{
				Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false},
			},
		},
	}
	clusterSpec := &cephv1.ClusterSpec{}

	err := validatePool(context, clusterInfo, clusterSpec, p)
	assert.Nil(t, err)

	// fail with a failure domain that doesn't exist
	p.Spec.FailureDomain = "doesntexist"
	err = validatePool(context, clusterInfo, clusterSpec, p)
	assert.NotNil(t, err)

	// fail with a crush root that doesn't exist
	p.Spec.FailureDomain = "osd"
	p.Spec.CrushRoot = "bad"
	err = validatePool(context, clusterInfo, clusterSpec, p)
	assert.NotNil(t, err)

	// fail with a crush root that does exist
	p.Spec.CrushRoot = "good"
	err = validatePool(context, clusterInfo, clusterSpec, p)
	assert.Nil(t, err)

	// Success replica size is 4 and replicasPerFailureDomain is 2
	p.Spec.Replicated.Size = 4
	p.Spec.Replicated.ReplicasPerFailureDomain = 2
	err = validatePool(context, clusterInfo, clusterSpec, p)
	assert.NoError(t, err)
}

func TestValidateDeviceClasses(t *testing.T) {
	testcases := []struct {
		name                       string
		primaryDeviceClassOutput   string
		secondaryDeviceClassOutput string
		hybridStorageSpec          *cephv1.HybridStorageSpec
		isValidSpec                bool
	}{
		{
			name:                       "valid hybridStorageSpec",
			primaryDeviceClassOutput:   "[0, 1, 2]",
			secondaryDeviceClassOutput: "[3, 4, 5]",
			hybridStorageSpec: &cephv1.HybridStorageSpec{
				PrimaryDeviceClass:   "ssd",
				SecondaryDeviceClass: "hdd",
			},
			isValidSpec: true,
		},
		{
			name:                       "invalid hybridStorageSpec.PrimaryDeviceClass",
			primaryDeviceClassOutput:   "[]",
			secondaryDeviceClassOutput: "[3, 4, 5]",
			hybridStorageSpec: &cephv1.HybridStorageSpec{
				PrimaryDeviceClass:   "ssd",
				SecondaryDeviceClass: "hdd",
			},
			isValidSpec: false,
		},
		{
			name:                       "invalid hybridStorageSpec.SecondaryDeviceClass",
			primaryDeviceClassOutput:   "[0, 1, 2]",
			secondaryDeviceClassOutput: "[]",
			hybridStorageSpec: &cephv1.HybridStorageSpec{
				PrimaryDeviceClass:   "ssd",
				SecondaryDeviceClass: "hdd",
			},
			isValidSpec: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			clusterInfo := cephclient.AdminTestClusterInfo("mycluster")
			executor := &exectest.MockExecutor{}
			context := &clusterd.Context{Executor: executor}
			executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
				logger.Infof("ExecuteCommandWithOutputFile: %s %v", command, args)
				if args[1] == "crush" && args[2] == "class" && args[3] == "ls-osd" && args[4] == "ssd" {
					// Mock executor for `ceph osd crush class ls-osd ssd`
					return tc.primaryDeviceClassOutput, nil
				} else if args[1] == "crush" && args[2] == "class" && args[3] == "ls-osd" && args[4] == "hdd" {
					// Mock executor for `ceph osd crush class ls-osd hdd`
					return tc.secondaryDeviceClassOutput, nil
				}
				return "", nil
			}

			p := &cephv1.PoolSpec{
				Replicated: cephv1.ReplicatedSpec{
					HybridStorage: tc.hybridStorageSpec,
				},
			}

			err := validateDeviceClasses(context, clusterInfo, p)
			if tc.isValidSpec {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
