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
	"errors"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	"github.com/rook/rook/pkg/operator/ceph/file/mds"
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
	clusterInfo := &client.ClusterInfo{Namespace: "ns"}
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

func TestCreateFilesystem(t *testing.T) {
	ctx := context.TODO()
	var deploymentsUpdated *[]*apps.Deployment
	mds.UpdateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()

	configDir, _ := ioutil.TempDir("", "")

	// Output to check multiple filesystem creation
	fses := `[{"name":"myfs","metadata_pool":"myfs-metadata","metadata_pool_id":1,"data_pool_ids":[2],"data_pools":["myfs-data0"]}]`

	createdFsResponse := `{"fs_name": "myfs", "metadata_pool": 2, "data_pools":[3]}`

	isBasePoolOperation := func(command string, args []string) bool {
		if reflect.DeepEqual(args[0:7], []string{"osd", "pool", "create", "myfs-metadata", "0", "replicated", "myfs-metadata"}) {
			return true
		} else if reflect.DeepEqual(args[0:5], []string{"osd", "crush", "rule", "create-replicated", "myfs-metadata"}) {
			return true
		} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", "myfs-metadata", "size", "1"}) {
			return true
		} else if reflect.DeepEqual(args[0:7], []string{"osd", "pool", "create", "myfs-data0", "0", "replicated", "myfs-data0"}) {
			return true
		} else if reflect.DeepEqual(args[0:5], []string{"osd", "crush", "rule", "create-replicated", "myfs-data0"}) {
			return true
		} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", "myfs-data0", "size", "1"}) {
			return true
		} else if reflect.DeepEqual(args[0:4], []string{"fs", "add_data_pool", "myfs", "myfs-data0"}) {
			return true
		}
		return false
	}

	firstGet := true
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command string, outFileArg string, args ...string) (string, error) {
			if contains(args, "fs") && contains(args, "get") {
				if firstGet {
					firstGet = false
					return "", errors.New("fs doesn't exist")
				}
				return createdFsResponse, nil
			} else if contains(args, "fs") && contains(args, "ls") {
				return "[]", nil
			} else if contains(args, "osd") && contains(args, "lspools") {
				return "[]", nil
			} else if isBasePoolOperation(command, args) {
				return "", nil
			} else if reflect.DeepEqual(args[0:5], []string{"fs", "new", "myfs", "myfs-metadata", "myfs-data0"}) {
				return "", nil
			} else if contains(args, "auth") && contains(args, "get-or-create-key") {
				return "{\"key\":\"mysecurekey\"}", nil
			} else if contains(args, "config") && contains(args, "mds_cache_memory_limit") {
				return "", nil
			} else if contains(args, "set") && contains(args, "max_mds") {
				return "", nil
			}
			assert.Fail(t, "Unexpected command")
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
	defer os.RemoveAll(configDir)
	clientset := testop.New(t, 1)
	context := &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: clientset}
	fs := cephv1.CephFilesystem{
		ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
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
	clusterInfo := &client.ClusterInfo{FSID: "myfsid"}

	// start a basic cluster
	err := createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, metav1.OwnerReference{}, "/var/lib/rook/", scheme.Scheme)
	assert.Nil(t, err)
	validateStart(ctx, t, context, fs)
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// starting again should be a no-op
	err = createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, metav1.OwnerReference{}, "/var/lib/rook/", scheme.Scheme)
	assert.Nil(t, err)
	validateStart(ctx, t, context, fs)
	assert.ElementsMatch(t, []string{"rook-ceph-mds-myfs-a", "rook-ceph-mds-myfs-b"}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// Increasing the number of data pools should be successful.
	createDataOnePoolCount := 0
	addDataOnePoolCount := 0
	executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command string, outFileArg string, args ...string) (string, error) {
			if contains(args, "fs") && contains(args, "get") {
				return createdFsResponse, nil
			} else if isBasePoolOperation(command, args) {
				return "", nil
			} else if reflect.DeepEqual(args[0:4], []string{"osd", "pool", "create", "myfs-data1"}) {
				createDataOnePoolCount++
				return "", nil
			} else if reflect.DeepEqual(args[0:4], []string{"fs", "add_data_pool", "myfs", "myfs-data1"}) {
				addDataOnePoolCount++
				return "", nil
			} else if contains(args, "set") && contains(args, "max_mds") {
				return "", nil
			} else if contains(args, "auth") && contains(args, "get-or-create-key") {
				return "{\"key\":\"mysecurekey\"}", nil
			} else if reflect.DeepEqual(args[0:5], []string{"osd", "crush", "rule", "create-replicated", "myfs-data1"}) {
				return "", nil
			} else if reflect.DeepEqual(args[0:6], []string{"osd", "pool", "set", "myfs-data1", "size", "1"}) {
				return "", nil
			} else if args[0] == "config" && args[1] == "set" {
				return "", nil
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

	err = createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, metav1.OwnerReference{}, "/var/lib/rook/", scheme.Scheme)
	assert.Nil(t, err)
	validateStart(ctx, t, context, fs)
	assert.ElementsMatch(t, []string{"rook-ceph-mds-myfs-a", "rook-ceph-mds-myfs-b"}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	assert.Equal(t, 1, createDataOnePoolCount)
	assert.Equal(t, 1, addDataOnePoolCount)
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// Test multiple filesystem creation
	executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command string, outFileArg string, args ...string) (string, error) {
			if contains(args, "ls") {
				return fses, nil
			}
			return "{\"key\":\"mysecurekey\"}", errors.New("multiple fs")
		},
	}
	context = &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: clientset}

	//Create another filesystem which should fail
	err = createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, metav1.OwnerReference{}, "/var/lib/rook/", scheme.Scheme)
	assert.Equal(t, "failed to create filesystem \"myfs\": cannot create multiple filesystems. enable ROOK_ALLOW_MULTIPLE_FILESYSTEMS env variable to create more than one", err.Error())
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
	clusterInfo := &client.ClusterInfo{FSID: "myfsid"}

	// start a basic cluster
	err := createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, metav1.OwnerReference{}, "/var/lib/rook/", scheme.Scheme)
	assert.Nil(t, err)
	validateStart(ctx, t, context, fs)

	// starting again should be a no-op
	err = createFilesystem(context, clusterInfo, fs, &cephv1.ClusterSpec{}, metav1.OwnerReference{}, "/var/lib/rook/", scheme.Scheme)
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
	r, err := context.Clientset.AppsV1().Deployments(fs.Namespace).Get(ctx, "rook-ceph-mds-myfs-a", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "rook-ceph-mds-myfs-a", r.Name)

	r, err = context.Clientset.AppsV1().Deployments(fs.Namespace).Get(ctx, "rook-ceph-mds-myfs-b", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "rook-ceph-mds-myfs-b", r.Name)
}
