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

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	"github.com/rook/rook/pkg/clusterd"
	cephtest "github.com/rook/rook/pkg/daemon/ceph/test"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStartMDS(t *testing.T) {
	configDir, _ := ioutil.TempDir("", "")
	// Output to check multiple file system creation
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
	fs := cephv1beta1.Filesystem{
		ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
		Spec: cephv1beta1.FilesystemSpec{
			MetadataPool: cephv1beta1.PoolSpec{Replicated: cephv1beta1.ReplicatedSpec{Size: 1}},
			DataPools:    []cephv1beta1.PoolSpec{{Replicated: cephv1beta1.ReplicatedSpec{Size: 1}}},
			MetadataServer: cephv1beta1.MetadataServerSpec{
				ActiveCount: 1,
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU: *resource.NewQuantity(100.0, resource.BinarySI),
					},
					Requests: v1.ResourceList{
						v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
					},
				},
			},
		},
	}

	//defer os.RemoveAll(c.dataDir)

	// start a basic cluster
	err := CreateFilesystem(context, fs, "v0.1", false, []metav1.OwnerReference{})
	assert.Nil(t, err)
	validateStart(t, context, fs)

	// starting again should be a no-op
	err = CreateFilesystem(context, fs, "v0.1", false, []metav1.OwnerReference{})
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
	context = &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: testop.New(3)}

	//Create another filesystem which should fail
	err = CreateFilesystem(context, fs, "v0.1", false, []metav1.OwnerReference{})
	assert.Equal(t, "failed to create file system myfs: Cannot create multiple filesystems. Enable ROOK_ALLOW_MULTIPLE_FILESYSTEMS env variable to create more than one", err.Error())
}

func contains(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}

func validateStart(t *testing.T, context *clusterd.Context, fs cephv1beta1.Filesystem) {

	r, err := context.Clientset.ExtensionsV1beta1().Deployments(fs.Namespace).Get("rook-ceph-mds-myfs", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, AppName+"-myfs", r.Name)
}

func TestPodSpecs(t *testing.T) {
	fs := cephv1beta1.Filesystem{
		ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
		Spec: cephv1beta1.FilesystemSpec{
			MetadataServer: cephv1beta1.MetadataServerSpec{
				ActiveCount: 1,
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU: *resource.NewQuantity(100.0, resource.BinarySI),
					},
					Requests: v1.ResourceList{
						v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
					},
				},
			},
		},
	}
	mdsID := "mds1"

	d := makeDeployment(nil, fs, mdsID, "rook/rook:myversion", false, []metav1.OwnerReference{})
	assert.NotNil(t, d)
	assert.Equal(t, AppName+"-myfs", d.Name)
	assert.Equal(t, v1.RestartPolicyAlways, d.Spec.Template.Spec.RestartPolicy)
	assert.Equal(t, 2, len(d.Spec.Template.Spec.Volumes))
	assert.Equal(t, "rook-data", d.Spec.Template.Spec.Volumes[0].Name)

	assert.Equal(t, AppName+"-myfs", d.ObjectMeta.Name)
	assert.Equal(t, AppName, d.Spec.Template.ObjectMeta.Labels["app"])
	assert.Equal(t, fs.Namespace, d.Spec.Template.ObjectMeta.Labels["rook_cluster"])
	assert.Equal(t, 0, len(d.ObjectMeta.Annotations))

	cont := d.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "rook/rook:myversion", cont.Image)
	assert.Equal(t, 2, len(cont.VolumeMounts))

	assert.Equal(t, 3, len(cont.Args))
	assert.Equal(t, "ceph", cont.Args[0])
	assert.Equal(t, "mds", cont.Args[1])
	assert.Equal(t, "--config-dir=/var/lib/rook", cont.Args[2])

	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())
}

func TestHostNetwork(t *testing.T) {
	fs := cephv1beta1.Filesystem{
		ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
		Spec: cephv1beta1.FilesystemSpec{
			MetadataServer: cephv1beta1.MetadataServerSpec{ActiveCount: 1},
		},
	}
	mdsID := "mds1"

	d := makeDeployment(nil, fs, mdsID, "v0.1", true, []metav1.OwnerReference{})

	assert.Equal(t, true, d.Spec.Template.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, d.Spec.Template.Spec.DNSPolicy)
}

func TestValidateSpec(t *testing.T) {
	context := &clusterd.Context{Executor: &exectest.MockExecutor{}}
	fs := cephv1beta1.Filesystem{}

	// missing name
	assert.NotNil(t, validateFilesystem(context, fs))
	fs.Name = "myfs"

	// missing namespace
	assert.NotNil(t, validateFilesystem(context, fs))
	fs.Namespace = "myns"

	// missing data pools
	assert.NotNil(t, validateFilesystem(context, fs))
	p := cephv1beta1.PoolSpec{Replicated: cephv1beta1.ReplicatedSpec{Size: 1}}
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
