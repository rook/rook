/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package mds

import (
	"context"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetDefaultFlagsMonConfigStore(t *testing.T) {
	clusterInfo := &cephclient.ClusterInfo{
		Context:   context.TODO(),
		Namespace: "ns",
		FSID:      "myfsid",
	}

	executor := &exectest.MockExecutor{}
	ctx := &clusterd.Context{
		Executor: executor,
	}

	t.Run("default factors with memory limit", func(t *testing.T) {
		memoryLimit := resource.MustParse("1Gi")
		fs := cephv1.CephFilesystem{
			ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
			Spec: cephv1.FilesystemSpec{
				MetadataServer: cephv1.MetadataServerSpec{
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceMemory: memoryLimit,
						},
					},
				},
			},
		}

		c := &Cluster{
			clusterInfo: clusterInfo,
			context:     ctx,
			fs:          fs,
		}

		// Mock config set command
		expectedCacheLimit := int(float64(memoryLimit.Value()) * mdsCacheMemoryLimitFactor)
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			if len(args) >= 4 && args[0] == "config" && args[1] == "set" && args[3] == "mds_cache_memory_limit" {
				assert.Equal(t, "mds.myfs-a", args[2])
				assert.Equal(t, "536870912", args[4]) // 1Gi * 0.5
				return "", nil
			}
			if len(args) >= 4 && args[0] == "config" && args[1] == "set" && args[3] == "mds_join_fs" {
				assert.Equal(t, "mds.myfs-a", args[2])
				assert.Equal(t, "myfs", args[4])
				return "", nil
			}
			return "", nil
		}

		err := c.setDefaultFlagsMonConfigStore("myfs-a")
		assert.NoError(t, err)
		assert.Equal(t, 536870912, expectedCacheLimit)
	})

	t.Run("custom limit factor", func(t *testing.T) {
		memoryLimit := resource.MustParse("1Gi")
		customFactor := 0.25
		fs := cephv1.CephFilesystem{
			ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
			Spec: cephv1.FilesystemSpec{
				MetadataServer: cephv1.MetadataServerSpec{
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceMemory: memoryLimit,
						},
					},
					CacheMemoryLimitFactor: &customFactor,
				},
			},
		}

		c := &Cluster{
			clusterInfo: clusterInfo,
			context:     ctx,
			fs:          fs,
		}

		// Mock config set command
		expectedCacheLimit := int(float64(memoryLimit.Value()) * customFactor)
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			if len(args) >= 4 && args[0] == "config" && args[1] == "set" && args[3] == "mds_cache_memory_limit" {
				assert.Equal(t, "mds.myfs-a", args[2])
				assert.Equal(t, "268435456", args[4]) // 1Gi * 0.25
				return "", nil
			}
			if len(args) >= 4 && args[0] == "config" && args[1] == "set" && args[3] == "mds_join_fs" {
				assert.Equal(t, "mds.myfs-a", args[2])
				assert.Equal(t, "myfs", args[4])
				return "", nil
			}
			return "", nil
		}

		err := c.setDefaultFlagsMonConfigStore("myfs-a")
		assert.NoError(t, err)
		assert.Equal(t, 268435456, expectedCacheLimit)
	})

	t.Run("default factors with memory request", func(t *testing.T) {
		memoryRequest := resource.MustParse("512Mi")
		fs := cephv1.CephFilesystem{
			ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
			Spec: cephv1.FilesystemSpec{
				MetadataServer: cephv1.MetadataServerSpec{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceMemory: memoryRequest,
						},
					},
				},
			},
		}

		c := &Cluster{
			clusterInfo: clusterInfo,
			context:     ctx,
			fs:          fs,
		}

		// Mock config set command
		expectedCacheLimit := int(float64(memoryRequest.Value()) * mdsCacheMemoryRequestFactor)
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			if len(args) >= 4 && args[0] == "config" && args[1] == "set" && args[3] == "mds_cache_memory_limit" {
				assert.Equal(t, "mds.myfs-a", args[2])
				assert.Equal(t, "429496729", args[4]) // 512Mi * 0.8
				return "", nil
			}
			if len(args) >= 4 && args[0] == "config" && args[1] == "set" && args[3] == "mds_join_fs" {
				assert.Equal(t, "mds.myfs-a", args[2])
				assert.Equal(t, "myfs", args[4])
				return "", nil
			}
			return "", nil
		}

		err := c.setDefaultFlagsMonConfigStore("myfs-a")
		assert.NoError(t, err)
		assert.Equal(t, 429496729, expectedCacheLimit)
	})

	t.Run("custom request factor", func(t *testing.T) {
		memoryRequest := resource.MustParse("512Mi")
		customFactor := 0.6
		fs := cephv1.CephFilesystem{
			ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
			Spec: cephv1.FilesystemSpec{
				MetadataServer: cephv1.MetadataServerSpec{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceMemory: memoryRequest,
						},
					},
					CacheMemoryRequestFactor: &customFactor,
				},
			},
		}

		c := &Cluster{
			clusterInfo: clusterInfo,
			context:     ctx,
			fs:          fs,
		}

		// Mock config set command
		expectedCacheLimit := int(float64(memoryRequest.Value()) * customFactor)
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			if len(args) >= 4 && args[0] == "config" && args[1] == "set" && args[3] == "mds_cache_memory_limit" {
				assert.Equal(t, "mds.myfs-a", args[2])
				assert.Equal(t, "322122547", args[4]) // 512Mi * 0.6
				return "", nil
			}
			if len(args) >= 4 && args[0] == "config" && args[1] == "set" && args[3] == "mds_join_fs" {
				assert.Equal(t, "mds.myfs-a", args[2])
				assert.Equal(t, "myfs", args[4])
				return "", nil
			}
			return "", nil
		}

		err := c.setDefaultFlagsMonConfigStore("myfs-a")
		assert.NoError(t, err)
		assert.Equal(t, 322122547, expectedCacheLimit)
	})

	t.Run("no memory specified", func(t *testing.T) {
		fs := cephv1.CephFilesystem{
			ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
			Spec: cephv1.FilesystemSpec{
				MetadataServer: cephv1.MetadataServerSpec{
					Resources: v1.ResourceRequirements{},
				},
			},
		}

		c := &Cluster{
			clusterInfo: clusterInfo,
			context:     ctx,
			fs:          fs,
		}

		// Mock config set command - should only set mds_join_fs
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			if len(args) >= 4 && args[0] == "config" && args[1] == "set" && args[3] == "mds_join_fs" {
				assert.Equal(t, "mds.myfs-a", args[2])
				assert.Equal(t, "myfs", args[4])
				return "", nil
			}
			return "", nil
		}

		err := c.setDefaultFlagsMonConfigStore("myfs-a")
		assert.NoError(t, err)
	})
}
