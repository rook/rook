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
	"testing"

	"os"

	"github.com/rook/rook/pkg/cephmgr/client"
	testclient "github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/operator/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

func TestStartMonPods(t *testing.T) {
	clientset := test.New(3)
	factory := &testclient.MockConnectionFactory{Fsid: "fsid", SecretKey: "mysecret"}
	c := New(clientset, factory, "myname", "ns", "", "myversion")
	c.maxRetries = 1
	c.retryDelay = 0

	// start a basic cluster
	// an error is expected since mocking always creates pods that are not running
	info, err := c.Start()
	assert.NotNil(t, err)
	assert.Nil(t, info)

	validateStart(t, c)

	// starting again should be a no-op, but still results in an error
	info, err = c.Start()
	assert.NotNil(t, err)
	assert.Nil(t, info)

	validateStart(t, c)
}

func validateStart(t *testing.T, c *Cluster) {
	s, err := c.clientset.CoreV1().Secrets(c.Namespace).Get("rook-admin", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(s.StringData))

	s, err = c.clientset.CoreV1().Secrets(c.Namespace).Get("mon", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 4, len(s.StringData))

	// there is only one pod created. the other two won't be created since the first one doesn't start
	p, err := c.clientset.CoreV1().Pods(c.Namespace).Get("mon0", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "mon0", p.Name)

	pods, err := c.clientset.CoreV1().Pods(c.Namespace).List(metav1.ListOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(pods.Items))

	// no pods are running or pending
	running, pending, err := c.pollPods()
	assert.Nil(t, err)
	assert.Equal(t, 0, len(running))
	assert.Equal(t, 0, len(pending))
}

func TestSaveMonEndpoints(t *testing.T) {
	clientset := test.New(1)
	c := New(clientset, nil, "myname", "ns", "", "myversion")
	c.clusterInfo = test.CreateClusterInfo(1)

	// create the initial config map
	err := c.saveMonConfig()
	assert.Nil(t, err)

	cm, err := c.clientset.CoreV1().ConfigMaps(c.Namespace).Get("mon-config", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "mon1=1.2.3.1:6790", cm.Data["endpoints"])

	// update the config map
	c.clusterInfo.Monitors["mon1"].Endpoint = "2.3.4.5:6790"
	err = c.saveMonConfig()
	assert.Nil(t, err)

	cm, err = c.clientset.CoreV1().ConfigMaps(c.Namespace).Get("mon-config", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "mon1=2.3.4.5:6790", cm.Data["endpoints"])
}

func TestCheckHealth(t *testing.T) {
	clientset := test.New(1)
	factory := &testclient.MockConnectionFactory{Fsid: "fsid", SecretKey: "mysecret"}
	c := New(clientset, factory, "myname", "ns", "", "myversion")
	c.retryDelay = 1
	c.maxRetries = 1
	c.clusterInfo = test.CreateClusterInfo(1)
	c.configDir = "/tmp/healthtest"
	c.waitForStart = false
	defer os.RemoveAll(c.configDir)

	err := c.CheckHealth()
	assert.Nil(t, err)

	c.maxMonID = 10
	conn, err := factory.NewConnWithClusterAndUser(c.Namespace, "admin")
	defer conn.Shutdown()
	err = c.failoverMon(conn, "mon1")
	assert.Nil(t, err)

	cm, err := c.clientset.CoreV1().ConfigMaps(c.Namespace).Get("mon-config", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "mon11=:6790", cm.Data["endpoints"])
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

func TestMonID(t *testing.T) {
	// invalid
	id, err := getMonID("m")
	assert.NotNil(t, err)
	assert.Equal(t, -1, id)
	id, err = getMonID("mon")
	assert.NotNil(t, err)
	assert.Equal(t, -1, id)
	id, err = getMonID("monitor0")
	assert.NotNil(t, err)
	assert.Equal(t, -1, id)

	// valid
	id, err = getMonID("mon0")
	assert.Nil(t, err)
	assert.Equal(t, 0, id)
	id, err = getMonID("mon123")
	assert.Nil(t, err)
	assert.Equal(t, 123, id)
}
