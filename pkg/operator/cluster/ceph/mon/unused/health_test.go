/*
Copyright 2017 The Rook Authors. All rights reserved.

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
package mon

import (
	"io/ioutil"
	"testing"

	"os"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckHealth(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return clienttest.MonInQuorumResponse(), nil
		},
	}
	clientset := test.New(3)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		Clientset: clientset,
		ConfigDir: configDir,
		Executor:  executor,
	}
	c := New(context, "ns", "", "myversion", 3, rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(3)
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	err := c.checkHealth()
	assert.Nil(t, err)

	c.maxMonID = 10
	err = c.failoverMon("rook-ceph-mon1")
	assert.Nil(t, err)

	_, ok := c.clusterInfo.Monitors["rook-ceph-mon1"]
	assert.False(t, ok)
	assert.NotNil(t, c.clusterInfo.Monitors["rook-ceph-mon2"])
	assert.NotNil(t, c.clusterInfo.Monitors["rook-ceph-mon3"])
	assert.NotNil(t, c.clusterInfo.Monitors["rook-ceph-mon11"])
}

// Simulate the behavior for a three nodes env when one mon fails (not in quorum)
func TestCheckHealthNotInSourceOfTruth(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return clienttest.MonInQuorumResponse(), nil
		},
	}
	clientset := test.New(3)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		Clientset: clientset,
		ConfigDir: configDir,
		Executor:  executor,
	}
	c := New(context, "ns", "", "myversion", 2, rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(1)
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	c.maxMonID = 10

	c.saveMonConfig()

	// Check if the two mons are found in the configmap
	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)

	assert.Equal(t, "rook-ceph-mon1=1.1.1.1:6790", cm.Data[MonEndpointKey])

	// Because rook-ceph-mon2 isn't in the MonInQuorumResponse() but in the
	// clusterinfo this will create a rook-ceph-mon2
	err = c.checkHealth()
	assert.Nil(t, err)

	// recheck that the "not found" mon has been replaced with a new one
	cm, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	if cm.Data[MonEndpointKey] == "rook-ceph-mon1=:6790,rook-ceph-mon11=:6790" {
		assert.Equal(t, "rook-ceph-mon1=:6790,rook-ceph-mon11=:6790", cm.Data[MonEndpointKey])
	} else {
		assert.Equal(t, "rook-ceph-mon11=:6790,rook-ceph-mon1=:6790", cm.Data[MonEndpointKey])
	}
}
