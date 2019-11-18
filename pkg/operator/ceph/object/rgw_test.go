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

package object

import (
	"io/ioutil"
	"os"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestStartRGW(t *testing.T) {
	clientset := testop.New(3)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return `{"key":"mysecurekey"}`, nil
		},
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			return `{"id":"test-id"}`, nil
		},
	}

	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	info := testop.CreateConfigDir(1)
	context := &clusterd.Context{Clientset: clientset, Executor: executor, ConfigDir: configDir}
	store := simpleStore()
	store.Spec.Gateway.Instances = 1
	version := "v1.1.0"
	data := cephconfig.NewStatelessDaemonDataPathMap(cephconfig.RgwType, "my-fs", "rook-ceph", "/var/lib/rook/")

	// start a basic cluster
	c := &clusterConfig{info, context, store, version, &cephv1.ClusterSpec{}, metav1.OwnerReference{}, data, false, false}
	err := c.startRGWPods()
	assert.Nil(t, err)

	validateStart(t, c, clientset)
}

func validateStart(t *testing.T, c *clusterConfig, clientset *fake.Clientset) {
	rgwName := c.instanceName() + "-a"
	r, err := clientset.AppsV1().Deployments(c.store.Namespace).Get(rgwName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, rgwName, r.Name)
}

func TestCreateObjectStore(t *testing.T) {
	commandWithOutputFunc := func(debug bool, actionName, command string, args ...string) (string, error) {
		return `{"realms": []}`, nil
	}
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithCombinedOutput: commandWithOutputFunc,
		MockExecuteCommandWithOutput:         commandWithOutputFunc,
		MockExecuteCommandWithOutputFile: func(debug bool, actionName, command, outfile string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if command == "ceph" {
				if args[1] == "erasure-code-profile" {
					return `{"k":"2","m":"1","plugin":"jerasure","technique":"reed_sol_van"}`, nil
				}
				if args[0] == "auth" && args[1] == "get-or-create-key" {
					return `{"key":"mykey"}`, nil
				}
			}
			return "", nil
		},
	}

	store := simpleStore()
	clientset := testop.New(3)
	context := &clusterd.Context{Executor: executor, Clientset: clientset}
	info := testop.CreateConfigDir(1)
	data := cephconfig.NewStatelessDaemonDataPathMap(cephconfig.RgwType, "my-fs", "rook-ceph", "/var/lib/rook/")

	// create the pools
	c := &clusterConfig{info, context, store, "1.2.3.4", &cephv1.ClusterSpec{}, metav1.OwnerReference{}, data, false, false}
	err := c.createOrUpdate()
	assert.Nil(t, err)
}

func simpleStore() cephv1.CephObjectStore {
	return cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "mycluster"},
		Spec: cephv1.ObjectStoreSpec{
			MetadataPool: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1}},
			DataPool:     cephv1.PoolSpec{ErasureCoded: cephv1.ErasureCodedSpec{CodingChunks: 1, DataChunks: 2}},
			Gateway:      cephv1.GatewaySpec{Port: 123},
		},
	}
}
