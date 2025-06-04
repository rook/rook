/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package file

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"reflect"
	"strings"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	"github.com/rook/rook/pkg/operator/ceph/file/mds"
	"github.com/rook/rook/pkg/operator/ceph/version"
	testopk8s "github.com/rook/rook/pkg/operator/k8sutil/test"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateSpec(t *testing.T) {
	context := &clusterd.Context{Executor: &exectest.MockExecutor{}}
	clusterInfo := &cephclient.ClusterInfo{Namespace: "ns"}
	fs := &cephv1.CephFilesystem{}
	clusterSpec := &cephv1.ClusterSpec{}

	// missing name
	assert.NotNil(t, validateFilesystem(context, clusterInfo, clusterSpec, fs))
	fs.Name = "myfs"

	// missing namespace
	assert.NotNil(t, validateFilesystem(context, clusterInfo, clusterSpec, fs))
	fs.Namespace = "myns"

	// missing data pools
	assert.NotNil(t, validateFilesystem(context, clusterInfo, clusterSpec, fs))
	p := cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false}}
	fs.Spec.DataPools = append(fs.Spec.DataPools, cephv1.NamedPoolSpec{PoolSpec: p})

	// missing metadata pool
	assert.NotNil(t, validateFilesystem(context, clusterInfo, clusterSpec, fs))
	fs.Spec.MetadataPool.PoolSpec = p

	// missing mds count
	assert.NotNil(t, validateFilesystem(context, clusterInfo, clusterSpec, fs))
	fs.Spec.MetadataServer.ActiveCount = 1

	// valid!
	assert.Nil(t, validateFilesystem(context, clusterInfo, clusterSpec, fs))
}

func TestHasDuplicatePoolNames(t *testing.T) {
	// PoolSpec with no duplicates
	fs := &cephv1.CephFilesystem{
		Spec: cephv1.FilesystemSpec{
			DataPools: []cephv1.NamedPoolSpec{
				{Name: "pool1"},
				{Name: "pool2"},
			},
		},
	}

	result := hasDuplicatePoolNames(fs.Spec.DataPools)
	assert.False(t, result)

	// add duplicate pool name in the spec.
	fs.Spec.DataPools = append(fs.Spec.DataPools, cephv1.NamedPoolSpec{Name: "pool1"})
	result = hasDuplicatePoolNames(fs.Spec.DataPools)
	assert.True(t, result)
}

func TestGenerateDataPoolNames(t *testing.T) {
	fs := &Filesystem{Name: "fake", Namespace: "fake"}
	fsSpec := cephv1.FilesystemSpec{
		DataPools: []cephv1.NamedPoolSpec{
			{
				PoolSpec: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false}},
			},
			{
				Name:     "somename",
				PoolSpec: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false}},
			},
		},
	}

	expectedNames := []string{"fake-data0", "fake-somename"}
	names := generateDataPoolNames(fs, fsSpec)
	assert.Equal(t, expectedNames, names)
}

func TestPreservePoolNames(t *testing.T) {
	fs := &Filesystem{Name: "fake", Namespace: "fake"}
	fsSpec := cephv1.FilesystemSpec{
		DataPools: []cephv1.NamedPoolSpec{
			{
				PoolSpec: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false}},
			},
			{
				Name:     "somename",
				PoolSpec: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false}},
			},
		},
		PreservePoolNames: true,
	}

	expectedNames := []string{"fake-data0", "somename"}
	names := generateDataPoolNames(fs, fsSpec)
	assert.Equal(t, expectedNames, names)
}

func isBasePoolOperation(fsName, command string, args []string) bool {
	if reflect.DeepEqual(args[0:7], []string{"osd", "pool", "create", fsName + "-metadata", "0", "replicated", fsName + "-metadata"}) {
		return true
	} else if reflect.DeepEqual(args[0:5], []string{"osd", "crush", "rule", "create-replicated", fsName + "-metadata"}) {
		return true
	} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", fsName + "-metadata", "size", "1"}) {
		return true
	} else if reflect.DeepEqual(args[0:7], []string{"osd", "pool", "create", fsName + "-data0", "0", "replicated", fsName + "-data0"}) {
		return true
	} else if reflect.DeepEqual(args[0:5], []string{"osd", "crush", "rule", "create-replicated", fsName + "-data0"}) {
		return true
	} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", fsName + "-data0", "size", "1"}) {
		return true
	} else if reflect.DeepEqual(args[0:4], []string{"fs", "add_data_pool", fsName, fsName + "-data0"}) {
		return true
	}
	return false
}

func fsExecutor(t *testing.T, fsName, configDir string, multiFS bool, createDataPoolCount, addDataPoolCount *int) *exectest.MockExecutor {
	mdsmap := cephclient.CephFilesystemDetails{
		ID: 0,
		MDSMap: cephclient.MDSMap{
			FilesystemName: fsName,
			MetadataPool:   2,
			MaxMDS:         1,
			Up: map[string]int{
				"mds_0": 123,
			},
			DataPools: []int{3},
			Info: map[string]cephclient.MDSInfo{
				"gid_123": {
					GID:   123,
					State: "up:active",
					Name:  fmt.Sprintf("%s-%s", fsName, "a"),
				},
				"gid_124": {
					GID:   124,
					State: "up:standby-replay",
					Name:  fmt.Sprintf("%s-%s", fsName, "b"),
				},
			},
		},
	}
	createdFsResponse, _ := json.Marshal(mdsmap)
	firstGet := true

	if multiFS {
		return &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if contains(args, "fs") && contains(args, "get") {
					if firstGet {
						firstGet = false
						return "", errors.New("fs doesn't exist")
					}
					return string(createdFsResponse), nil
				} else if contains(args, "fs") && contains(args, "ls") {
					return `[{"name":"myfs","metadata_pool":"myfs-metadata","metadata_pool_id":4,"data_pool_ids":[5],"data_pools":["myfs-data0"]},{"name":"myfs2","metadata_pool":"myfs2-metadata","metadata_pool_id":6,"data_pool_ids":[7],"data_pools":["myfs2-data0"]},{"name":"leseb","metadata_pool":"cephfs.leseb.meta","metadata_pool_id":8,"data_pool_ids":[9],"data_pools":["cephfs.leseb.data"]}]`, nil
				} else if contains(args, "fs") && contains(args, "dump") {
					return `{"standbys":[], "filesystems":[]}`, nil
				} else if reflect.DeepEqual(args[0:5], []string{"fs", "subvolumegroup", "create", fsName, defaultCSISubvolumeGroup}) {
					return "", nil
				} else if contains(args, "osd") && contains(args, "lspools") {
					return "[]", nil
				} else if contains(args, "mds") && contains(args, "fail") {
					return "", nil
				} else if isBasePoolOperation(fsName, command, args) {
					return "", nil
				} else if reflect.DeepEqual(args[0:5], []string{"fs", "new", fsName, fsName + "-metadata", fsName + "-data0"}) {
					return "", nil
				} else if contains(args, "auth") && contains(args, "get-or-create-key") {
					return "{\"key\":\"mysecurekey\"}", nil
				} else if contains(args, "auth") && contains(args, "del") {
					return "", nil
				} else if contains(args, "config") && contains(args, "get") {
					return "{}", nil
				} else if contains(args, "config") && contains(args, "mds_cache_memory_limit") {
					return "", nil
				} else if contains(args, "set") && contains(args, "max_mds") {
					return "", nil
				} else if contains(args, "set") && contains(args, "allow_standby_replay") {
					return "", nil
				} else if contains(args, "config") && contains(args, "mds_join_fs") {
					return "", nil
				} else if contains(args, "flag") && contains(args, "enable_multiple") {
					return "", nil
				} else if reflect.DeepEqual(args[0:5], []string{"osd", "crush", "rule", "create-replicated", fsName + "-data1"}) {
					return "", nil
				} else if reflect.DeepEqual(args[0:4], []string{"osd", "pool", "create", fsName + "-data1"}) {
					*createDataPoolCount++
					return "", nil
				} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", fsName + "-data1", "size", "1"}) {
					return "", nil
				} else if reflect.DeepEqual(args[0:4], []string{"fs", "add_data_pool", fsName, fsName + "-data1"}) {
					*addDataPoolCount++
					return "", nil
				} else if reflect.DeepEqual(args[0:5], []string{"osd", "crush", "rule", "create-replicated", fsName + "-named-pool"}) {
					return "", nil
				} else if reflect.DeepEqual(args[0:4], []string{"osd", "pool", "create", fsName + "-named-pool"}) {
					*createDataPoolCount++
					return "", nil
				} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", fsName + "-named-pool", "size", "1"}) {
					return "", nil
				} else if reflect.DeepEqual(args[0:4], []string{"fs", "add_data_pool", fsName, fsName + "-named-pool"}) {
					*addDataPoolCount++
					return "", nil
				} else if reflect.DeepEqual(args[0:3], []string{"osd", "pool", "get"}) {
					return "", errors.New("test pool does not exist yet")
				} else if contains(args, "versions") {
					versionStr, _ := json.Marshal(
						map[string]map[string]int{
							"mds": {
								"ceph version 19.0.0-0-g2f728b9 (2f728b952cf293dd7f809ad8a0f5b5d040c43010) squid (stable)": 2,
							},
						})
					return string(versionStr), nil
				} else if strings.Contains(command, "ceph-authtool") {
					err := clienttest.CreateConfigDir(path.Join(configDir, "ns"))
					assert.Nil(t, err)
				}

				assert.Fail(t, fmt.Sprintf("Unexpected command %q %q", command, args))
				return "", nil
			},
		}
	}

	return &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if contains(args, "fs") && contains(args, "get") {
				if firstGet {
					firstGet = false
					return "", errors.New("fs doesn't exist")
				}
				return string(createdFsResponse), nil
			} else if contains(args, "fs") && contains(args, "ls") {
				return "[]", nil
			} else if contains(args, "fs") && contains(args, "dump") {
				return `{"standbys":[], "filesystems":[]}`, nil
			} else if reflect.DeepEqual(args[0:5], []string{"fs", "subvolumegroup", "create", fsName, defaultCSISubvolumeGroup}) {
				return "", nil
			} else if reflect.DeepEqual(args[0:5], []string{"fs", "subvolumegroup", "info", fsName, defaultCSISubvolumeGroup}) {
				return "", nil
			} else if contains(args, "osd") && contains(args, "lspools") {
				return "[]", nil
			} else if contains(args, "mds") && contains(args, "fail") {
				return "", nil
			} else if isBasePoolOperation(fsName, command, args) {
				return "", nil
			} else if reflect.DeepEqual(args[0:5], []string{"fs", "new", fsName, fsName + "-metadata", fsName + "-data0"}) {
				return "", nil
			} else if contains(args, "auth") && contains(args, "get-or-create-key") {
				return "{\"key\":\"mysecurekey\"}", nil
			} else if contains(args, "auth") && contains(args, "del") {
				return "", nil
			} else if contains(args, "config") && contains(args, "mds_cache_memory_limit") {
				return "", nil
			} else if contains(args, "set") && contains(args, "max_mds") {
				return "", nil
			} else if contains(args, "set") && contains(args, "allow_standby_replay") {
				return "", nil
			} else if contains(args, "config") && contains(args, "mds_join_fs") {
				return "", nil
			} else if contains(args, "flag") && contains(args, "enable_multiple") {
				return "", nil
			} else if contains(args, "config") && contains(args, "get") {
				return "{}", nil
			} else if reflect.DeepEqual(args[0:5], []string{"osd", "crush", "rule", "create-replicated", fsName + "-data1"}) {
				return "", nil
			} else if reflect.DeepEqual(args[0:3], []string{"osd", "pool", "application"}) {
				return "", nil
			} else if reflect.DeepEqual(args[0:4], []string{"osd", "pool", "create", fsName + "-data1"}) {
				*createDataPoolCount++
				return "", nil
			} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", fsName + "-data1", "size", "1"}) {
				return "", nil
			} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", fsName + "-metadata", "target_size_ratio", "0"}) {
				return "", nil
			} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", fsName + "-data0", "target_size_ratio", "0"}) {
				return "", nil
			} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", fsName + "-data1", "target_size_ratio", "0"}) {
				return "", nil
			} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", fsName + "-named-pool", "target_size_ratio", "0"}) {
				return "", nil
			} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", fsName + "-metadata", "compression_mode", ""}) {
				return "", nil
			} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", fsName + "-data0", "compression_mode", ""}) {
				return "", nil
			} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", fsName + "-data1", "compression_mode", ""}) {
				return "", nil
			} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", fsName + "-named-pool", "compression_mode", ""}) {
				return "", nil
			} else if reflect.DeepEqual(args[0:4], []string{"fs", "add_data_pool", fsName, fsName + "-data1"}) {
				*addDataPoolCount++
				return "", nil
			} else if reflect.DeepEqual(args[0:5], []string{"osd", "crush", "rule", "create-replicated", fsName + "-named-pool"}) {
				return "", nil
			} else if reflect.DeepEqual(args[0:4], []string{"osd", "pool", "create", fsName + "-named-pool"}) {
				*createDataPoolCount++
				return "", nil
			} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", fsName + "-named-pool", "size", "1"}) {
				return "", nil
			} else if reflect.DeepEqual(args[0:3], []string{"osd", "pool", "get"}) {
				return "", errors.New("test pool does not exist yet")
			} else if reflect.DeepEqual(args[0:4], []string{"fs", "add_data_pool", fsName, fsName + "-named-pool"}) {
				*addDataPoolCount++
				return "", nil
			} else if contains(args, "versions") {
				versionStr, _ := json.Marshal(
					map[string]map[string]int{
						"mds": {
							"ceph version 19.2.0-0-g2f728b9 (2f728b952cf293dd7f809ad8a0f5b5d040c43010) squid (stable)": 2,
						},
					})
				return string(versionStr), nil
			} else if strings.Contains(command, "ceph-authtool") {
				err := clienttest.CreateConfigDir(path.Join(configDir, "ns"))
				assert.Nil(t, err)
			}
			assert.Fail(t, fmt.Sprintf("Unexpected command %q %q", command, args))
			return "", nil
		},
	}
}

func fsTest(fsName string) cephv1.CephFilesystem {
	return cephv1.CephFilesystem{
		ObjectMeta: metav1.ObjectMeta{Name: fsName, Namespace: "ns"},
		Spec: cephv1.FilesystemSpec{
			MetadataPool: cephv1.NamedPoolSpec{
				PoolSpec: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false}},
			},
			DataPools: []cephv1.NamedPoolSpec{
				{
					PoolSpec: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false}},
				},
			},
			MetadataServer: cephv1.MetadataServerSpec{
				ActiveCount: 1,
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceMemory: *resource.NewQuantity(4294967296, resource.BinarySI),
					},
					Requests: v1.ResourceList{
						v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
					},
				},
			},
		},
	}
}

func TestCreateFilesystem(t *testing.T) {
	ctx := context.TODO()
	var deploymentsUpdated *[]*apps.Deployment
	mds.UpdateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()
	configDir := t.TempDir()
	fsName := "myfs"
	addDataPoolCount := 0
	createDataPoolCount := 0
	executor := fsExecutor(t, fsName, configDir, false, &createDataPoolCount, &addDataPoolCount)
	clientset := testop.New(t, 1)
	context := &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: clientset,
	}
	fs := fsTest(fsName)
	clusterInfo := &cephclient.ClusterInfo{FSID: "myfsid", CephVersion: version.Squid, Context: ctx}
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()

	t.Run("start basic filesystem", func(t *testing.T) {
		// start a basic cluster
		err := createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, ownerInfo, "/var/lib/rook/")
		assert.Nil(t, err)
		validateStart(ctx, t, context, fs)
		assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
		testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)
	})

	t.Run("start again should no-op", func(t *testing.T) {
		err := createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, ownerInfo, "/var/lib/rook/")
		assert.Nil(t, err)
		validateStart(ctx, t, context, fs)
		assert.ElementsMatch(t, []string{fmt.Sprintf("rook-ceph-mds-%s-a", fsName), fmt.Sprintf("rook-ceph-mds-%s-b", fsName)}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
		testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)
	})

	t.Run("increasing the number of data pools should be successful.", func(t *testing.T) {
		context = &clusterd.Context{
			Executor:  executor,
			ConfigDir: configDir,
			Clientset: clientset,
		}
		// add not named pool, with default naming
		fs.Spec.DataPools = append(fs.Spec.DataPools, cephv1.NamedPoolSpec{
			PoolSpec: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false}},
		})
		// add named pool
		fs.Spec.DataPools = append(fs.Spec.DataPools, cephv1.NamedPoolSpec{
			Name:     "named-pool",
			PoolSpec: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false}},
		})
		err := createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, ownerInfo, "/var/lib/rook/")
		assert.Nil(t, err)
		validateStart(ctx, t, context, fs)
		assert.ElementsMatch(t, []string{fmt.Sprintf("rook-ceph-mds-%s-a", fsName), fmt.Sprintf("rook-ceph-mds-%s-b", fsName)}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
		assert.Equal(t, 2, createDataPoolCount)
		assert.Equal(t, 2, addDataPoolCount)
		testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)
	})

	t.Run("multi filesystem creation should succeed", func(t *testing.T) {
		clusterInfo.CephVersion = version.Squid
		err := createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, ownerInfo, "/var/lib/rook/")
		assert.NoError(t, err)
	})
}

func TestUpgradeFilesystem(t *testing.T) {
	ctx := context.TODO()
	var deploymentsUpdated *[]*apps.Deployment
	mds.UpdateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()
	configDir := t.TempDir()

	fsName := "myfs"
	addDataPoolCount := 0
	createDataPoolCount := 0
	executor := fsExecutor(t, fsName, configDir, false, &createDataPoolCount, &addDataPoolCount)
	clientset := testop.New(t, 1)
	context := &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: clientset,
	}
	fs := fsTest(fsName)
	clusterInfo := &cephclient.ClusterInfo{FSID: "myfsid", CephVersion: version.Squid, Context: ctx}

	// start a basic cluster for upgrade
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	err := createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, ownerInfo, "/var/lib/rook/")
	assert.NoError(t, err)
	validateStart(ctx, t, context, fs)
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// do upgrade
	clusterInfo.CephVersion = version.Squid
	context = &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: clientset,
	}
	err = createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, ownerInfo, "/var/lib/rook/")
	assert.NoError(t, err)

	// test fail standby daemon failed
	mdsmap := cephclient.CephFilesystemDetails{
		ID: 0,
		MDSMap: cephclient.MDSMap{
			FilesystemName: fsName,
			MetadataPool:   2,
			MaxMDS:         1,
			Up: map[string]int{
				"mds_0": 123,
			},
			DataPools: []int{3},
			Info: map[string]cephclient.MDSInfo{
				"gid_123": {
					GID:   123,
					State: "up:active",
					Name:  fmt.Sprintf("%s-%s", fsName, "a"),
				},
				"gid_124": {
					GID:   124,
					State: "up:standby-replay",
					Name:  fmt.Sprintf("%s-%s", fsName, "b"),
				},
			},
		},
	}
	createdFsResponse, _ := json.Marshal(mdsmap)

	// actual version
	clusterInfo.CephVersion = version.Squid
	// mocked version to cause an error different from the actual version
	mockedVersionStr, _ := json.Marshal(
		map[string]map[string]int{
			"mds": {
				"ceph version 18.2.0-0-g2f728b9 (2f728b952cf293dd7f809ad8a0f5b5d040c43010) reef (stable)": 2,
			},
		})
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if contains(args, "fs") {
			if contains(args, "get") {
				return string(createdFsResponse), nil
			} else if contains(args, "ls") {
				return "[]", nil
			} else if contains(args, "dump") {
				return `{"standbys":[], "filesystems":[]}`, nil
			} else if contains(args, "subvolumegroup") {
				return "[]", nil
			}
		}
		if contains(args, "osd") {
			if contains(args, "lspools") {
				return "[]", nil
			}
			if contains(args, "pool") && contains(args, "application") {
				if contains(args, "get") {
					return `{"":{}}`, nil
				}
				return "[]", nil
			}
			if reflect.DeepEqual(args[1:3], []string{"pool", "get"}) {
				return "", errors.New("test pool does not exist yet")
			}
		}
		if contains(args, "mds") && contains(args, "fail") {
			return "", errors.New("fail mds failed")
		}
		if isBasePoolOperation(fsName, command, args) {
			return "", nil
		}
		if reflect.DeepEqual(args[0:5], []string{"fs", "new", fsName, fsName + "-metadata", fsName + "-data0"}) {
			return "", nil
		}
		if contains(args, "auth") {
			if contains(args, "get-or-create-key") {
				return "{\"key\":\"mysecurekey\"}", nil
			} else if contains(args, "auth") && contains(args, "del") {
				return "", nil
			}
		}
		if contains(args, "config") {
			if contains(args, "mds_cache_memory_limit") {
				return "", nil
			} else if contains(args, "mds_join_fs") {
				return "", nil
			} else if contains(args, "get") {
				return "{}", nil
			}
		}
		if contains(args, "set") {
			if contains(args, "max_mds") {
				return "", nil
			} else if contains(args, "allow_standby_replay") {
				return "", nil
			}
		}
		if contains(args, "versions") {
			return string(mockedVersionStr), nil
		}
		assert.Fail(t, fmt.Sprintf("Unexpected command %q %q", command, args))
		return "", nil
	}
	// do upgrade
	context = &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: clientset,
	}
	err = createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, ownerInfo, "/var/lib/rook/")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fail mds failed")
}

func TestCreateNopoolFilesystem(t *testing.T) {
	ctx := context.TODO()
	clientset := testop.New(t, 3)
	configDir := t.TempDir()
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				err := clienttest.CreateConfigDir(path.Join(configDir, "ns"))
				assert.Nil(t, err)
			} else {
				return "{\"key\":\"mysecurekey\"}", nil
			}
			return "", errors.New("unknown command error")
		},
	}
	context := &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: clientset,
	}
	fs := cephv1.CephFilesystem{
		ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
		Spec: cephv1.FilesystemSpec{
			MetadataServer: cephv1.MetadataServerSpec{
				ActiveCount: 1,
			},
		},
	}
	clusterInfo := &cephclient.ClusterInfo{FSID: "myfsid", Context: ctx}

	// start a basic cluster
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	err := createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, ownerInfo, "/var/lib/rook/")
	assert.Nil(t, err)
	validateStart(ctx, t, context, fs)

	// starting again should be a no-op
	err = createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, ownerInfo, "/var/lib/rook/")
	assert.Nil(t, err)
	validateStart(ctx, t, context, fs)
}

func contains(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}

func validateStart(ctx context.Context, t *testing.T, context *clusterd.Context, fs cephv1.CephFilesystem) {
	r, err := context.Clientset.AppsV1().Deployments(fs.Namespace).Get(ctx, fmt.Sprintf("rook-ceph-mds-%s-a", fs.Name), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, fmt.Sprintf("rook-ceph-mds-%s-a", fs.Name), r.Name)

	r, err = context.Clientset.AppsV1().Deployments(fs.Namespace).Get(ctx, fmt.Sprintf("rook-ceph-mds-%s-b", fs.Name), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, fmt.Sprintf("rook-ceph-mds-%s-b", fs.Name), r.Name)
}
