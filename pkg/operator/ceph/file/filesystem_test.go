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
	"io/ioutil"
	"os"
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
	"github.com/rook/rook/pkg/operator/k8sutil"
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
	fs.Spec.DataPools = append(fs.Spec.DataPools, p)

	// missing metadata pool
	assert.NotNil(t, validateFilesystem(context, clusterInfo, clusterSpec, fs))
	fs.Spec.MetadataPool = p

	// missing mds count
	assert.NotNil(t, validateFilesystem(context, clusterInfo, clusterSpec, fs))
	fs.Spec.MetadataServer.ActiveCount = 1

	// valid!
	assert.Nil(t, validateFilesystem(context, clusterInfo, clusterSpec, fs))
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

func fsExecutor(t *testing.T, fsName, configDir string, multiFS bool) *exectest.MockExecutor {
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
			MockExecuteCommandWithOutputFile: func(command string, outFileArg string, args ...string) (string, error) {
				if contains(args, "fs") && contains(args, "get") {
					if firstGet {
						firstGet = false
						return "", errors.New("fs doesn't exist")
					}
					return string(createdFsResponse), nil
				} else if contains(args, "fs") && contains(args, "ls") {
					return `[{"name":"myfs","metadata_pool":"myfs-metadata","metadata_pool_id":4,"data_pool_ids":[5],"data_pools":["myfs-data0"]},{"name":"myfs2","metadata_pool":"myfs2-metadata","metadata_pool_id":6,"data_pool_ids":[7],"data_pools":["myfs2-data0"]},{"name":"leseb","metadata_pool":"cephfs.leseb.meta","metadata_pool_id":8,"data_pool_ids":[9],"data_pools":["cephfs.leseb.data"]}]`, nil
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
				} else if contains(args, "versions") {
					versionStr, _ := json.Marshal(
						map[string]map[string]int{
							"mds": {
								"ceph version 16.0.0-4-g2f728b9 (2f728b952cf293dd7f809ad8a0f5b5d040c43010) pacific (stable)": 2,
							},
						})
					return string(versionStr), nil
				}
				assert.Fail(t, fmt.Sprintf("Unexpected command %q %q", command, args))
				return "", nil
			},
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if strings.Contains(command, "ceph-authtool") {
					err := clienttest.CreateConfigDir(path.Join(configDir, "ns"))
					assert.Nil(t, err)
				}

				return "", nil
			},
		}
	}

	return &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command string, outFileArg string, args ...string) (string, error) {
			if contains(args, "fs") && contains(args, "get") {
				if firstGet {
					firstGet = false
					return "", errors.New("fs doesn't exist")
				}
				return string(createdFsResponse), nil
			} else if contains(args, "fs") && contains(args, "ls") {
				return "[]", nil
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
			} else if contains(args, "config") && contains(args, "get") {
				return "{}", nil
			} else if contains(args, "versions") {
				versionStr, _ := json.Marshal(
					map[string]map[string]int{
						"mds": {
							"ceph version 16.0.0-4-g2f728b9 (2f728b952cf293dd7f809ad8a0f5b5d040c43010) pacific (stable)": 2,
						},
					})
				return string(versionStr), nil
			}
			assert.Fail(t, fmt.Sprintf("Unexpected command %q %q", command, args))
			return "", nil
		},
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				err := clienttest.CreateConfigDir(path.Join(configDir, "ns"))
				assert.Nil(t, err)
			}

			return "", nil
		},
	}
}

func fsTest(fsName string) cephv1.CephFilesystem {
	return cephv1.CephFilesystem{
		ObjectMeta: metav1.ObjectMeta{Name: fsName, Namespace: "ns"},
		Spec: cephv1.FilesystemSpec{
			MetadataPool: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false}},
			DataPools:    []cephv1.PoolSpec{{Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false}}},
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
	configDir, _ := ioutil.TempDir("", "")

	fsName := "myfs"
	executor := fsExecutor(t, fsName, configDir, false)
	defer os.RemoveAll(configDir)
	clientset := testop.New(t, 1)
	context := &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: clientset}
	fs := fsTest(fsName)
	clusterInfo := &cephclient.ClusterInfo{FSID: "myfsid", CephVersion: version.Octopus}

	// start a basic cluster
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	err := createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, ownerInfo, "/var/lib/rook/")
	assert.Nil(t, err)
	validateStart(ctx, t, context, fs)
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// starting again should be a no-op
	err = createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, ownerInfo, "/var/lib/rook/")
	assert.Nil(t, err)
	validateStart(ctx, t, context, fs)
	assert.ElementsMatch(t, []string{fmt.Sprintf("rook-ceph-mds-%s-a", fsName), fmt.Sprintf("rook-ceph-mds-%s-b", fsName)}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// Increasing the number of data pools should be successful.
	createDataOnePoolCount := 0
	addDataOnePoolCount := 0
	createdFsResponse := fmt.Sprintf(`{"fs_name": "%s", "metadata_pool": 2, "data_pools":[3]}`, fsName)
	executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command string, outFileArg string, args ...string) (string, error) {
			if contains(args, "fs") && contains(args, "get") {
				return createdFsResponse, nil
			} else if isBasePoolOperation(fsName, command, args) {
				return "", nil
			} else if reflect.DeepEqual(args[0:4], []string{"osd", "pool", "create", fsName + "-data1"}) {
				createDataOnePoolCount++
				return "", nil
			} else if reflect.DeepEqual(args[0:4], []string{"fs", "add_data_pool", fsName, fsName + "-data1"}) {
				addDataOnePoolCount++
				return "", nil
			} else if contains(args, "set") && contains(args, "max_mds") {
				return "", nil
			} else if contains(args, "auth") && contains(args, "get-or-create-key") {
				return "{\"key\":\"mysecurekey\"}", nil
			} else if reflect.DeepEqual(args[0:5], []string{"osd", "crush", "rule", "create-replicated", fsName + "-data1"}) {
				return "", nil
			} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", fsName + "-data1", "size", "1"}) {
				return "", nil
			} else if args[0] == "config" && args[1] == "set" {
				return "", nil
			} else if contains(args, "versions") {
				versionStr, _ := json.Marshal(
					map[string]map[string]int{
						"mds": {
							"ceph version 16.0.0-4-g2f728b9 (2f728b952cf293dd7f809ad8a0f5b5d040c43010) pacific (stable)": 2,
						},
					})
				return string(versionStr), nil
			}
			assert.Fail(t, fmt.Sprintf("Unexpected command: %v", args))
			return "", nil
		},
	}
	context = &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: clientset}
	fs.Spec.DataPools = append(fs.Spec.DataPools, cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false}})

	err = createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, ownerInfo, "/var/lib/rook/")
	assert.Nil(t, err)
	validateStart(ctx, t, context, fs)
	assert.ElementsMatch(t, []string{fmt.Sprintf("rook-ceph-mds-%s-a", fsName), fmt.Sprintf("rook-ceph-mds-%s-b", fsName)}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	assert.Equal(t, 1, createDataOnePoolCount)
	assert.Equal(t, 1, addDataOnePoolCount)
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// Test multiple filesystem creation
	// Output to check multiple filesystem creation
	fses := `[{"name":"myfs","metadata_pool":"myfs-metadata","metadata_pool_id":4,"data_pool_ids":[5],"data_pools":["myfs-data0"]},{"name":"myfs2","metadata_pool":"myfs2-metadata","metadata_pool_id":6,"data_pool_ids":[7],"data_pools":["myfs2-data0"]},{"name":"leseb","metadata_pool":"cephfs.leseb.meta","metadata_pool_id":8,"data_pool_ids":[9],"data_pools":["cephfs.leseb.data"]}]`
	executorMultiFS := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command string, outFileArg string, args ...string) (string, error) {
			if contains(args, "ls") {
				return fses, nil
			} else if contains(args, "versions") {
				versionStr, _ := json.Marshal(
					map[string]map[string]int{
						"mds": {
							"ceph version 16.0.0-4-g2f728b9 (2f728b952cf293dd7f809ad8a0f5b5d040c43010) pacific (stable)": 2,
						},
					})
				return string(versionStr), nil
			}
			return "{\"key\":\"mysecurekey\"}", errors.New("multiple fs")
		},
	}
	context = &clusterd.Context{
		Executor:  executorMultiFS,
		ConfigDir: configDir,
		Clientset: clientset,
	}

	// Create another filesystem which should fail
	err = createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, &k8sutil.OwnerInfo{}, "/var/lib/rook/")
	assert.Error(t, err)
	assert.Equal(t, fmt.Sprintf("failed to create filesystem %q: multiple filesystems are only supported as of ceph pacific", fsName), err.Error())

	// It works since the op env var is specified even if we don't run on pacific
	os.Setenv(cephclient.MultiFsEnv, "true")
	fsName = "myfs2"
	fs = fsTest(fsName)
	executor = fsExecutor(t, fsName, configDir, true)
	clusterInfo.CephVersion = version.Pacific
	context = &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: clientset,
	}
	err = createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, ownerInfo, "/var/lib/rook/")
	assert.NoError(t, err)
	os.Unsetenv(cephclient.MultiFsEnv)

	// It works since the Ceph version is Pacific
	fsName = "myfs3"
	fs = fsTest(fsName)
	executor = fsExecutor(t, fsName, configDir, true)
	clusterInfo.CephVersion = version.Pacific
	context = &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: clientset,
	}
	err = createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, ownerInfo, "/var/lib/rook/")
	assert.NoError(t, err)
}

func TestUpgradeFilesystem(t *testing.T) {
	ctx := context.TODO()
	var deploymentsUpdated *[]*apps.Deployment
	mds.UpdateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()
	configDir, _ := ioutil.TempDir("", "")

	fsName := "myfs"
	executor := fsExecutor(t, fsName, configDir, false)
	defer os.RemoveAll(configDir)
	clientset := testop.New(t, 1)
	context := &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: clientset}
	fs := fsTest(fsName)
	clusterInfo := &cephclient.ClusterInfo{FSID: "myfsid", CephVersion: version.Octopus}

	// start a basic cluster for upgrade
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	err := createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, ownerInfo, "/var/lib/rook/")
	assert.NoError(t, err)
	validateStart(ctx, t, context, fs)
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// do upgrade
	clusterInfo.CephVersion = version.Quincy
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
	firstGet := false
	executor.MockExecuteCommandWithOutputFile = func(command string, outFileArg string, args ...string) (string, error) {
		if contains(args, "fs") && contains(args, "get") {
			if firstGet {
				firstGet = false
				return "", errors.New("fs doesn't exist")
			}
			return string(createdFsResponse), nil
		} else if contains(args, "fs") && contains(args, "ls") {
			return "[]", nil
		} else if contains(args, "osd") && contains(args, "lspools") {
			return "[]", nil
		} else if contains(args, "mds") && contains(args, "fail") {
			return "", errors.New("fail mds failed")
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
		} else if contains(args, "config") && contains(args, "get") {
			return "{}", nil
		} else if contains(args, "versions") {
			versionStr, _ := json.Marshal(
				map[string]map[string]int{
					"mds": {
						"ceph version 16.0.0-4-g2f728b9 (2f728b952cf293dd7f809ad8a0f5b5d040c43010) pacific (stable)": 2,
					},
				})
			return string(versionStr), nil
		}
		assert.Fail(t, fmt.Sprintf("Unexpected command %q %q", command, args))
		return "", nil
	}
	// do upgrade
	clusterInfo.CephVersion = version.Quincy
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
	configDir, _ := ioutil.TempDir("", "")
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command string, outFileArg string, args ...string) (string, error) {
			return "{\"key\":\"mysecurekey\"}", nil
		},
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				err := clienttest.CreateConfigDir(path.Join(configDir, "ns"))
				assert.Nil(t, err)
			}

			return "", nil
		},
	}
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: clientset}
	fs := cephv1.CephFilesystem{
		ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
		Spec: cephv1.FilesystemSpec{
			MetadataServer: cephv1.MetadataServerSpec{
				ActiveCount: 1,
			},
		},
	}
	clusterInfo := &cephclient.ClusterInfo{FSID: "myfsid"}

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
