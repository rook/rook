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
	"fmt"
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
	clientset := test.New(1)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		Clientset: clientset,
		ConfigDir: configDir,
		Executor:  executor,
	}
	c := New(context, "ns", "", "myversion", 3, rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(1)
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	c.mapping.MonsToNodes[0] = "node0"
	c.mapping.Addresses[0] = "0.0.0.0"
	c.maxMonID = 10

	err := c.checkHealth()
	assert.Nil(t, err)

	err = c.failoverMon("mon1")
	assert.Nil(t, err)

	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "rook-ceph-mon11=:6790", cm.Data[EndpointDataKey])
}

func TestCheckHealthNotFound(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return clienttest.MonInQuorumResponse(), nil
		},
	}
	clientset := test.New(1)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		Clientset: clientset,
		ConfigDir: configDir,
		Executor:  executor,
	}
	c := New(context, "ns", "", "myversion", 2, rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(2)
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	c.mapping.MonsToNodes[0] = "node0"
	c.mapping.Addresses[0] = "0.0.0.0"
	c.mapping.Addresses[0] = "0.0.0.0"
	c.mapping.MonsToNodes[2] = "node0"
	c.maxMonID = 10

	c.saveMonConfig()

	// Check if the two mons are found in the configmap
	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	if cm.Data[EndpointDataKey] == "mon1=1.2.3.1:6790,mon2=1.2.3.2:6790" {
		assert.Equal(t, "mon1=1.2.3.1:6790,mon2=1.2.3.2:6790", cm.Data[EndpointDataKey])
	} else {
		assert.Equal(t, "mon2=1.2.3.2:6790,mon1=1.2.3.1:6790", cm.Data[EndpointDataKey])
	}

	// Because mon2 isn't in the MonInQuorumResponse() this will create a mon11
	err = c.checkHealth()
	assert.Nil(t, err)

	// recheck that the "not found" mon has been replaced with a new one
	cm, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	if cm.Data[EndpointDataKey] == "mon1=1.2.3.1:6790,rook-ceph-mon11=:6790" {
		assert.Equal(t, "mon1=1.2.3.1:6790,rook-ceph-mon11=:6790", cm.Data[EndpointDataKey])
	} else {
		assert.Equal(t, "rook-ceph-mon11=:6790,mon1=1.2.3.1:6790", cm.Data[EndpointDataKey])
	}
}

func TestCheckMonsValid(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return clienttest.MonInQuorumResponse(), nil
		},
	}
	clientset := test.New(1)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		Clientset: clientset,
		ConfigDir: configDir,
		Executor:  executor,
	}
	c := New(context, "ns", "", "myversion", 3, rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(1)
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	// add two mons to the mapping on node0
	c.mapping.MonsToNodes[1] = "node0"
	c.mapping.NodesToMons["node0"] = 1
	c.mapping.MonsToNodes[2] = "node1"
	c.mapping.NodesToMons["node1"] = 2

	// add three nodes
	for i := 0; i < 3; i++ {
		n := &v1.Node{
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type: v1.NodeReady,
					},
				},
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeExternalIP,
						Address: "0.0.0.0",
					},
				},
			},
		}
		n.Name = fmt.Sprintf("node%d", i)
		clientset.CoreV1().Nodes().Create(n)
	}

	_, err := c.checkMonsOnValidNodes()
	assert.Nil(t, err)
	assert.Equal(t, "node0", c.mapping.MonsToNodes[1])
	assert.Equal(t, "node1", c.mapping.MonsToNodes[2])

	// set node1 unschedulable and check that mon2 gets failovered to be mon3 to node2
	node0, err := c.context.Clientset.CoreV1().Nodes().Get("node0", metav1.GetOptions{})
	assert.Nil(t, err)
	node0.Spec.Unschedulable = true
	_, err = c.context.Clientset.CoreV1().Nodes().Update(node0)
	assert.Nil(t, err)

	// add the pods so the getNodesInUse() works correctly
	for i := 1; i <= 2; i++ {
		po := c.makeMonPod(&monConfig{Name: fmt.Sprintf("mon%d", i)}, fmt.Sprintf("node%d", i-1))
		_, err = clientset.CoreV1().Pods(c.Namespace).Create(po)
		assert.Nil(t, err)
	}

	_, err = c.checkMonsOnValidNodes()
	assert.Nil(t, err)

	assert.Len(t, c.mapping.MonsToNodes, 2)
	assert.Nil(t, c.mapping.MonsToNodes[1])
	// the new mon should always be on the empty node2
	// the failovered mon's name is "rook-ceph-mon0"
	assert.Equal(t, "node2", c.mapping.MonsToNodes[0])
	assert.Equal(t, "node1", c.mapping.MonsToNodes[2])
}
