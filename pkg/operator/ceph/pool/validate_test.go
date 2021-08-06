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

	// not specifying some replication or EC settings is fine
	p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	err := ValidatePool(context, clusterInfo, clusterSpec, &p)
	assert.Nil(t, err)

	// must specify name
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Namespace: clusterInfo.Namespace}}
	err = ValidatePool(context, clusterInfo, clusterSpec, &p)
	assert.NotNil(t, err)

	// must specify namespace
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool"}}
	err = ValidatePool(context, clusterInfo, clusterSpec, &p)
	assert.NotNil(t, err)

	// must not specify both replication and EC settings
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = false
	p.Spec.ErasureCoded.CodingChunks = 2
	p.Spec.ErasureCoded.DataChunks = 3
	err = ValidatePool(context, clusterInfo, clusterSpec, &p)
	assert.NotNil(t, err)

	// succeed with replication settings
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = false
	err = ValidatePool(context, clusterInfo, clusterSpec, &p)
	assert.Nil(t, err)

	// size is 1 and RequireSafeReplicaSize is true
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = true
	err = ValidatePool(context, clusterInfo, clusterSpec, &p)
	assert.Error(t, err)

	// succeed with ec settings
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.ErasureCoded.CodingChunks = 1
	p.Spec.ErasureCoded.DataChunks = 2
	err = ValidatePool(context, clusterInfo, clusterSpec, &p)
	assert.Nil(t, err)

	// Tests with various compression modes
	// succeed with compression mode "none"
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = false
	p.Spec.CompressionMode = "none"
	err = ValidatePool(context, clusterInfo, clusterSpec, &p)
	assert.Nil(t, err)

	// succeed with compression mode "aggressive"
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = false
	p.Spec.CompressionMode = "aggressive"
	err = ValidatePool(context, clusterInfo, clusterSpec, &p)
	assert.Nil(t, err)

	// fail with compression mode "unsupported"
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = false
	p.Spec.CompressionMode = "unsupported"
	err = ValidatePool(context, clusterInfo, clusterSpec, &p)
	assert.Error(t, err)

	// fail since replica size is lower than ReplicasPerFailureDomain
	p.Spec.Replicated.ReplicasPerFailureDomain = 2
	err = ValidatePool(context, clusterInfo, clusterSpec, &p)
	assert.Error(t, err)

	// fail since replica size is equal than ReplicasPerFailureDomain
	p.Spec.Replicated.Size = 2
	p.Spec.Replicated.ReplicasPerFailureDomain = 2
	err = ValidatePool(context, clusterInfo, clusterSpec, &p)
	assert.Error(t, err)

	// fail since ReplicasPerFailureDomain is not a power of 2
	p.Spec.Replicated.Size = 4
	p.Spec.Replicated.ReplicasPerFailureDomain = 3
	err = ValidatePool(context, clusterInfo, clusterSpec, &p)
	assert.Error(t, err)

	// fail since ReplicasPerFailureDomain is not a power of 2
	p.Spec.Replicated.Size = 4
	p.Spec.Replicated.ReplicasPerFailureDomain = 5
	err = ValidatePool(context, clusterInfo, clusterSpec, &p)
	assert.Error(t, err)

	// Failure the sub domain does not exist
	p.Spec.Replicated.SubFailureDomain = "dummy"
	err = ValidatePool(context, clusterInfo, clusterSpec, &p)
	assert.Error(t, err)

	// succeed with ec pool and valid compression mode
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.ErasureCoded.CodingChunks = 1
	p.Spec.ErasureCoded.DataChunks = 2
	p.Spec.CompressionMode = "passive"
	err = ValidatePool(context, clusterInfo, clusterSpec, &p)
	assert.Nil(t, err)

	// Add mirror test mode
	{
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.Mirroring.Enabled = true
		p.Spec.Mirroring.Mode = "foo"
		err = ValidatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
		assert.EqualError(t, err, "unrecognized mirroring mode \"foo\". only 'image and 'pool' are supported")

		// Success mode is known
		p.Spec.Mirroring.Mode = "pool"
		err = ValidatePool(context, clusterInfo, clusterSpec, &p)
		assert.NoError(t, err)

		// Error no interval specified
		p.Spec.Mirroring.SnapshotSchedules = []cephv1.SnapshotScheduleSpec{{StartTime: "14:00:00-05:00"}}
		err = ValidatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
		assert.EqualError(t, err, "schedule interval cannot be empty if start time is specified")

		// Success we have an interval
		p.Spec.Mirroring.SnapshotSchedules = []cephv1.SnapshotScheduleSpec{{Interval: "24h"}}
		err = ValidatePool(context, clusterInfo, clusterSpec, &p)
		assert.NoError(t, err)
	}

	// Failure and subfailure domains
	{
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
		p.Spec.FailureDomain = "host"
		p.Spec.Replicated.SubFailureDomain = "host"
		err = ValidatePool(context, clusterInfo, clusterSpec, &p)
		assert.Error(t, err)
		assert.EqualError(t, err, "failure and subfailure domain cannot be identical")
	}

}

func TestValidateCrushProperties(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	clusterInfo := cephclient.AdminClusterInfo("mycluster")
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
		Spec: cephv1.PoolSpec{
			Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false},
		},
	}
	clusterSpec := &cephv1.ClusterSpec{}

	err := ValidatePool(context, clusterInfo, clusterSpec, p)
	assert.Nil(t, err)

	// fail with a failure domain that doesn't exist
	p.Spec.FailureDomain = "doesntexist"
	err = ValidatePool(context, clusterInfo, clusterSpec, p)
	assert.NotNil(t, err)

	// fail with a crush root that doesn't exist
	p.Spec.FailureDomain = "osd"
	p.Spec.CrushRoot = "bad"
	err = ValidatePool(context, clusterInfo, clusterSpec, p)
	assert.NotNil(t, err)

	// fail with a crush root that does exist
	p.Spec.CrushRoot = "good"
	err = ValidatePool(context, clusterInfo, clusterSpec, p)
	assert.Nil(t, err)

	// Success replica size is 4 and replicasPerFailureDomain is 2
	p.Spec.Replicated.Size = 4
	p.Spec.Replicated.ReplicasPerFailureDomain = 2
	err = ValidatePool(context, clusterInfo, clusterSpec, p)
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
			clusterInfo := cephclient.AdminClusterInfo("mycluster")
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

			p := &cephv1.CephBlockPool{
				ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace},
				Spec: cephv1.PoolSpec{
					Replicated: cephv1.ReplicatedSpec{
						HybridStorage: tc.hybridStorageSpec,
					},
				},
			}

			err := validateDeviceClasses(context, clusterInfo, &p.Spec)
			if tc.isValidSpec {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
