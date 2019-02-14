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
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
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
	context := &clusterd.Context{Clientset: clientset, Executor: executor, ConfigDir: configDir}
	store := simpleStore()
	version := "v1.1.0"

	// start a basic cluster
	c := &config{context, store, version, cephv1.CephVersionSpec{}, false, []metav1.OwnerReference{}}
	err := c.createStore()
	assert.Nil(t, err)

	validateStart(t, c, clientset, false)

	// starting again should update the pods with the new settings
	c.store.Spec.Gateway.AllNodes = true
	err = c.updateStore()
	assert.Nil(t, err)

	validateStart(t, c, clientset, true)
}

func validateStart(t *testing.T, c *config, clientset *fake.Clientset, allNodes bool) {
	if !allNodes {
		r, err := clientset.Apps().Deployments(c.store.Namespace).Get(c.instanceName(), metav1.GetOptions{})
		assert.Nil(t, err)
		assert.Equal(t, c.instanceName(), r.Name)

		_, err = clientset.Apps().DaemonSets(c.store.Namespace).Get(c.instanceName(), metav1.GetOptions{})
		assert.True(t, errors.IsNotFound(err))
	} else {
		r, err := clientset.Apps().DaemonSets(c.store.Namespace).Get(c.instanceName(), metav1.GetOptions{})
		assert.Nil(t, err)
		assert.Equal(t, c.instanceName(), r.Name)

		_, err = clientset.Apps().Deployments(c.store.Namespace).Get(c.instanceName(), metav1.GetOptions{})
		assert.True(t, errors.IsNotFound(err))
	}

	s, err := clientset.CoreV1().Services(c.store.Namespace).Get(c.instanceName(), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, c.instanceName(), s.Name)

	secret, err := clientset.CoreV1().Secrets(c.store.Namespace).Get(c.instanceName(), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, c.instanceName(), secret.Name)
	assert.Equal(t, 1, len(secret.StringData))
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

	// create the pools
	c := &config{context, store, "1.2.3.4", cephv1.CephVersionSpec{}, false, []metav1.OwnerReference{}}
	err := c.createStore()
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
