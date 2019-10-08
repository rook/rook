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
	"errors"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	cephtest "github.com/rook/rook/pkg/daemon/ceph/test"
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
	fs := cephv1.CephFilesystem{}

	// missing name
	assert.NotNil(t, validateFilesystem(context, fs))
	fs.Name = "myfs"

	// missing namespace
	assert.NotNil(t, validateFilesystem(context, fs))
	fs.Namespace = "myns"

	// missing data pools
	assert.NotNil(t, validateFilesystem(context, fs))
	p := cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1}}
	fs.Spec.DataPools = append(fs.Spec.DataPools, p)

	// missing metadata pool
	assert.NotNil(t, validateFilesystem(context, fs))
	fs.Spec.MetadataPool = p

	// missing mds count
	assert.NotNil(t, validateFilesystem(context, fs))
	fs.Spec.MetadataServer.ActiveCount = 1

	// valid!
	assert.Nil(t, validateFilesystem(context, fs))
}

func TestCreateFilesystem(t *testing.T) {
	var deploymentsUpdated *[]*apps.Deployment
	mds.UpdateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()

	configDir, _ := ioutil.TempDir("", "")
	// Output to check multiple filesystem creation
	fses := `[{"name":"myfs","metadata_pool":"myfs-metadata","metadata_pool_id":1,"data_pool_ids":[2],"data_pools":["myfs-data0"]}]`

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return "{\"key\":\"mysecurekey\"}", nil
		},
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				cephtest.CreateConfigDir(path.Join(configDir, "ns"))
			}

			return "", nil
		},
	}
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: testop.New(3)}
	fs := cephv1.CephFilesystem{
		ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
		Spec: cephv1.FilesystemSpec{
			MetadataPool: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1}},
			DataPools:    []cephv1.PoolSpec{{Replicated: cephv1.ReplicatedSpec{Size: 1}}},
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
	clusterInfo := &cephconfig.ClusterInfo{FSID: "myfsid"}

	// start a basic cluster
	err := createFilesystem(clusterInfo, context, fs, "v0.1", &cephv1.ClusterSpec{}, metav1.OwnerReference{}, "/var/lib/rook/", false)
	assert.Nil(t, err)
	validateStart(t, context, fs)
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// starting again should be a no-op
	err = createFilesystem(clusterInfo, context, fs, "v0.1", &cephv1.ClusterSpec{}, metav1.OwnerReference{}, "/var/lib/rook/", false)
	assert.Nil(t, err)
	validateStart(t, context, fs)
	assert.ElementsMatch(t, []string{"rook-ceph-mds-myfs-a", "rook-ceph-mds-myfs-b"}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// Test multiple filesystem creation
	executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			if contains(args, "ls") {
				return fses, nil
			}
			return "{\"key\":\"mysecurekey\"}", errors.New("multiple fs")
		},
	}
	context = &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: testop.New(3)}

	//Create another filesystem which should fail
	err = createFilesystem(clusterInfo, context, fs, "v0.1", &cephv1.ClusterSpec{}, metav1.OwnerReference{}, "/var/lib/rook/", false)
	assert.Equal(t, "failed to create filesystem myfs: cannot create multiple filesystems. enable ROOK_ALLOW_MULTIPLE_FILESYSTEMS env variable to create more than one", err.Error())
}

func TestCreateNopoolFilesystem(t *testing.T) {
	configDir, _ := ioutil.TempDir("", "")
	// Output to check multiple filesystem creation
	fses := `[{"name":"myfs"}]`

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return "{\"key\":\"mysecurekey\"}", nil
		},
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				cephtest.CreateConfigDir(path.Join(configDir, "ns"))
			}

			return "", nil
		},
	}
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: testop.New(3)}
	fs := cephv1.CephFilesystem{
		ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
		Spec: cephv1.FilesystemSpec{
			MetadataServer: cephv1.MetadataServerSpec{
				ActiveCount: 1,
			},
		},
	}
	clusterInfo := &cephconfig.ClusterInfo{FSID: "myfsid"}

	// start a basic cluster
	err := createFilesystem(clusterInfo, context, fs, "v0.1", &cephv1.ClusterSpec{}, metav1.OwnerReference{}, "/var/lib/rook/", false)
	assert.Nil(t, err)
	validateStart(t, context, fs)

	// starting again should be a no-op
	err = createFilesystem(clusterInfo, context, fs, "v0.1", &cephv1.ClusterSpec{}, metav1.OwnerReference{}, "/var/lib/rook/", false)
	assert.Nil(t, err)
	validateStart(t, context, fs)

	// Test multiple filesystem creation
	executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			if contains(args, "ls") {
				return fses, nil
			}
			return "{\"key\":\"mysecurekey\"}", errors.New("multiple fs")
		},
	}
}
func contains(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}

func validateStart(t *testing.T, context *clusterd.Context, fs cephv1.CephFilesystem) {
	r, err := context.Clientset.AppsV1().Deployments(fs.Namespace).Get("rook-ceph-mds-myfs-a", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "rook-ceph-mds-myfs-a", r.Name)

	r, err = context.Clientset.AppsV1().Deployments(fs.Namespace).Get("rook-ceph-mds-myfs-b", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "rook-ceph-mds-myfs-b", r.Name)
}
