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
package mon

import (
	"encoding/json"
	"io/ioutil"
	"path"
	"strings"
	"testing"
	"time"

	"os"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephtest "github.com/rook/rook/pkg/daemon/ceph/test"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestStartCluster(namespace string) *clusterd.Context {
	clientset := test.New(3)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				cephtest.CreateConfigDir(path.Join(configDir, namespace))
			}
			return "", nil
		},
	}
	return &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
		ConfigDir: configDir,
	}
}

func newCluster(context *clusterd.Context, namespace string, hostNetwork bool, resources v1.ResourceRequirements) *Cluster {
	return &Cluster{
		HostNetwork:         true,
		context:             context,
		Namespace:           namespace,
		Version:             "myversion",
		Size:                3,
		maxMonID:            -1,
		waitForStart:        false,
		monPodRetryInterval: 10 * time.Millisecond,
		monPodTimeout:       1 * time.Second,
		monTimeoutList:      map[string]time.Time{},
		resources:           resources,
		ownerRef:            metav1.OwnerReference{},
	}
}

func TestStartMonPods(t *testing.T) {
	namespace := "ns"
	context := newTestStartCluster(namespace)
	c := newCluster(context, namespace, false, v1.ResourceRequirements{})

	// start a basic cluster
	err := c.Start()
	assert.Nil(t, err)

	validateStart(t, c)

	// starting again should be a no-op, but still results in an error
	err = c.Start()
	assert.Nil(t, err)

	validateStart(t, c)
}

func TestOperatorRestart(t *testing.T) {
	namespace := "ns"
	context := newTestStartCluster(namespace)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, arg ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				cephtest.CreateConfigDir(path.Join(context.ConfigDir, namespace))
			}
			return "", nil
		},
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			cephtest.CreateConfigDir(path.Join(context.ConfigDir, namespace))
			resp := client.MonStatusResponse{Quorum: []int{0}}
			resp.MonMap.Mons = []client.MonMapEntry{
				{
					Name:    "rook-ceph-mon-0",
					Rank:    0,
					Address: "0.0.0.0",
				},
				{
					Name:    "rook-ceph-mon-1",
					Rank:    0,
					Address: "1.1.1.1",
				},
				{
					Name:    "rook-ceph-mon-2",
					Rank:    0,
					Address: "2.2.2.2",
				},
			}
			serialized, _ := json.Marshal(resp)
			return string(serialized), nil
		},
	}
	context.Executor = executor
	c := newCluster(context, namespace, false, v1.ResourceRequirements{})
	c.clusterInfo = test.CreateConfigDir(3)

	// start a basic cluster
	err := c.Start()
	assert.Nil(t, err)

	validateStart(t, c)

	c = newCluster(context, namespace, false, v1.ResourceRequirements{})

	// starting again should be a no-op, but will not result in an error
	err = c.Start()
	assert.Nil(t, err)

	validateStart(t, c)
}

// safety check that if hostNetwork is used no changes occur on an operator restart
func TestOperatorRestartHostNetwork(t *testing.T) {
	namespace := "ns"
	context := newTestStartCluster(namespace)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, arg ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				cephtest.CreateConfigDir(path.Join(context.ConfigDir, namespace))
			}
			return "", nil
		},
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			cephtest.CreateConfigDir(path.Join(context.ConfigDir, namespace))
			resp := client.MonStatusResponse{Quorum: []int{0}}
			resp.MonMap.Mons = []client.MonMapEntry{
				{
					Name:    "rook-ceph-mon-0",
					Rank:    0,
					Address: "0.0.0.0",
				},
				{
					Name:    "rook-ceph-mon-1",
					Rank:    0,
					Address: "0.0.0.0",
				},
				{
					Name:    "rook-ceph-mon-2",
					Rank:    0,
					Address: "0.0.0.0",
				},
			}
			serialized, _ := json.Marshal(resp)
			return string(serialized), nil
		},
	}
	context.Executor = executor
	c := newCluster(context, namespace, false, v1.ResourceRequirements{})
	c.clusterInfo = test.CreateConfigDir(1)

	// start a basic cluster
	err := c.Start()
	assert.Nil(t, err)

	validateStart(t, c)

	c = newCluster(context, namespace, true, v1.ResourceRequirements{})

	// starting again should be a no-op, but still results in an error
	err = c.Start()
	assert.Nil(t, err)

	validateStart(t, c)
}

func validateStart(t *testing.T, c *Cluster) {
	s, err := c.context.Clientset.CoreV1().Secrets(c.Namespace).Get(appName, metav1.GetOptions{})
	assert.Nil(t, err) // there shouldn't be an error due the secret existing
	assert.Equal(t, 4, len(s.StringData))

	// there is only one pod created. the other two won't be created since the first one doesn't start
	_, err = c.context.Clientset.Extensions().ReplicaSets(c.Namespace).Get("rook-ceph-mon-0", metav1.GetOptions{})
	assert.Nil(t, err)
}

func TestSaveMonEndpoints(t *testing.T) {
	clientset := test.New(1)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: configDir}, "ns", "", "myversion", 3, rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(1)

	// create the initial config map
	err := c.saveMonConfig()
	assert.Nil(t, err)

	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "rook-ceph-mon-1=1.1.1.1:6790", cm.Data[MonEndpointKey])

	// update the config map
	c.clusterInfo.Monitors["rook-ceph-mon-1"].Endpoint = "2.3.4.5:6790"
	c.maxMonID = 2
	err = c.saveMonConfig()
	assert.Nil(t, err)

	cm, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "rook-ceph-mon-1=2.3.4.5:6790", cm.Data[MonEndpointKey])
	assert.Equal(t, "rook-ceph-mon=rook-ceph-mon.ns.svc:6790", cm.Data[EndpointKey])
}

func TestMonInQuourm(t *testing.T) {
	entry := client.MonMapEntry{Name: "foo", Rank: 23}
	quorum := []int{}
	// Nothing in quorum
	assert.False(t, monInQuorum(entry, quorum))

	// One or more members in quorum
	quorum = []int{23}
	assert.True(t, monInQuorum(entry, quorum))
	quorum = []int{5, 6, 7, 23, 8}
	assert.True(t, monInQuorum(entry, quorum))

	// Not in quorum
	entry.Rank = 1
	assert.False(t, monInQuorum(entry, quorum))
}

func TestGetMonID(t *testing.T) {
	// invalid
	id, err := GetMonID("m")
	assert.NotNil(t, err)
	assert.Equal(t, -1, id)
	id, err = GetMonID("mon")
	assert.NotNil(t, err)
	assert.Equal(t, -1, id)
	id, err = GetMonID("rook-ceph-monitor0")
	assert.NotNil(t, err)
	assert.Equal(t, -1, id)

	// valid
	id, err = GetMonID("rook-ceph-mon0")
	assert.Nil(t, err)
	assert.Equal(t, 0, id)
	id, err = GetMonID("rook-ceph-mon123")
	assert.Nil(t, err)
	assert.Equal(t, 123, id)
}
