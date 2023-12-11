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
	"context"
	"fmt"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// generate a standard mon config from a mon id w/ default port and IP 2.4.6.{1,2,3,...}
// support mon ID as new ["a", "b", etc.] form or as legacy ["mon0", "mon1", etc.] form
func testGenMonConfig(monID string) *monConfig {
	var moniker string
	var index int
	var err error
	if strings.HasPrefix(monID, "mon") { // is legacy mon name
		moniker = monID                                                 // keep legacy "mon#" name
		index, err = strconv.Atoi(strings.Replace(monID, "mon", "", 1)) // get # off end of mon#
	} else {
		moniker = "mon-" + monID
		index, err = k8sutil.NameToIndex(monID)
	}
	if err != nil {
		panic(err)
	}
	return &monConfig{
		ResourceName: "rook-ceph-" + moniker, // rook-ceph-mon-A or rook-ceph-mon#
		DaemonName:   monID,                  // A or mon#
		Port:         DefaultMsgr1Port,
		PublicIP:     fmt.Sprintf("2.4.6.%d", index+1),
		// dataDirHostPath assumed to be /var/lib/rook
		DataPathMap: config.NewStatefulDaemonDataPathMap(
			"/var/lib/rook", dataDirRelativeHostPath(monID), config.MonType, monID, "rook-ceph"),
	}
}

func newTestStartCluster(t *testing.T, namespace string) (*clusterd.Context, error) {
	monResponse := func() (string, error) {
		return clienttest.MonInQuorumResponseMany(3), nil
	}
	return newTestStartClusterWithQuorumResponse(t, namespace, monResponse)
}

func newTestStartClusterWithQuorumResponse(t *testing.T, namespace string, monResponse func() (string, error)) (*clusterd.Context, error) {
	clientset := test.New(t, 3)
	configDir := t.TempDir()
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				err := clienttest.CreateConfigDir(path.Join(configDir, namespace))
				return "", errors.Wrap(err, "failed testing of start cluster without quorum response")
			} else {
				return monResponse()
			}
		},
	}
	return &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
		ConfigDir: configDir,
	}, nil
}

func newCluster(context *clusterd.Context, namespace string, allowMultiplePerNode bool, resources v1.ResourceRequirements) *Cluster {
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	return &Cluster{
		ClusterInfo: nil,
		context:     context,
		Namespace:   namespace,
		rookImage:   "myversion",
		spec: cephv1.ClusterSpec{
			Mon: cephv1.MonSpec{
				Count:                3,
				AllowMultiplePerNode: allowMultiplePerNode,
			},
			Resources: map[string]v1.ResourceRequirements{"mon": resources},
		},
		maxMonID:       -1,
		waitForStart:   false,
		monTimeoutList: map[string]time.Time{},
		mapping: &opcontroller.Mapping{
			Schedule: map[string]*opcontroller.MonScheduleInfo{},
		},
		ownerInfo: ownerInfo,
	}
}

// setCommonMonProperties is a convenience helper for setting common test properties
func setCommonMonProperties(c *Cluster, currentMons int, mon cephv1.MonSpec, rookImage string) {
	c.ClusterInfo = clienttest.CreateTestClusterInfo(currentMons)
	c.spec.Mon.Count = mon.Count
	c.spec.Mon.AllowMultiplePerNode = mon.AllowMultiplePerNode
	c.rookImage = rookImage
}

func TestResourceName(t *testing.T) {
	assert.Equal(t, "rook-ceph-mon-a", resourceName("rook-ceph-mon-a"))
	assert.Equal(t, "rook-ceph-mon123", resourceName("rook-ceph-mon123"))
	assert.Equal(t, "rook-ceph-mon-b", resourceName("b"))
}

func TestStartMonDeployment(t *testing.T) {
	namespace := "ns"
	context, err := newTestStartCluster(t, namespace)
	assert.NoError(t, err)
	c := newCluster(context, namespace, true, v1.ResourceRequirements{})
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)

	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: EndpointConfigMapName},
		Data:       map[string]string{"maxMonId": "1"},
	}
	_, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(c.ClusterInfo.Context, cm, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Start mon a on a specific node since there is no volumeClaimTemplate
	m := &monConfig{ResourceName: "rook-ceph-mon-a", DaemonName: "a", Port: 3300, PublicIP: "1.2.3.4", DataPathMap: &config.DataPathMap{}}
	schedule := &opcontroller.MonScheduleInfo{Hostname: "host-a", Zone: "zonea"}
	err = c.startMon(m, schedule)
	assert.NoError(t, err)
	deployment, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Get(c.ClusterInfo.Context, m.ResourceName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, schedule.Hostname, deployment.Spec.Template.Spec.NodeSelector["kubernetes.io/hostname"])

	// Start mon b on any node in a zone since there is a volumeClaimTemplate
	m = &monConfig{ResourceName: "rook-ceph-mon-b", DaemonName: "b", Port: 3300, PublicIP: "1.2.3.5", DataPathMap: &config.DataPathMap{}}
	schedule = &opcontroller.MonScheduleInfo{Hostname: "host-b", Zone: "zoneb"}
	c.spec.Mon.VolumeClaimTemplate = &v1.PersistentVolumeClaim{}
	err = c.startMon(m, schedule)
	assert.NoError(t, err)
	deployment, err = c.context.Clientset.AppsV1().Deployments(c.Namespace).Get(c.ClusterInfo.Context, m.ResourceName, metav1.GetOptions{})
	assert.NoError(t, err)
	// no node selector when there is a volumeClaimTemplate and the mon is assigned to a zone
	assert.Equal(t, 0, len(deployment.Spec.Template.Spec.NodeSelector))
}

func TestStartMonPods(t *testing.T) {
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")
	namespace := "ns"
	context, err := newTestStartCluster(t, namespace)
	assert.NoError(t, err)
	c := newCluster(context, namespace, true, v1.ResourceRequirements{})
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)
	c.spec.Annotations = cephv1.AnnotationsSpec{
		cephv1.KeyClusterMetadata: cephv1.Annotations{
			"key": "value",
		},
	}

	// start a basic cluster
	_, err = c.Start(c.ClusterInfo, c.rookImage, cephver.Quincy, c.spec)
	assert.NoError(t, err)

	// test annotations
	secret, err := c.context.Clientset.CoreV1().Secrets(c.Namespace).Get(c.ClusterInfo.Context, AppName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"key": "value"}, secret.Annotations)

	validateStart(t, c)

	// starting again should be a no-op
	_, err = c.Start(c.ClusterInfo, c.rookImage, cephver.Quincy, c.spec)
	assert.NoError(t, err)

	validateStart(t, c)
}

func TestOperatorRestart(t *testing.T) {
	namespace := "ns"
	context, err := newTestStartCluster(t, namespace)
	assert.NoError(t, err)
	c := newCluster(context, namespace, true, v1.ResourceRequirements{})
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)

	// start a basic cluster
	info, err := c.Start(c.ClusterInfo, c.rookImage, cephver.Quincy, c.spec)
	assert.NoError(t, err)
	assert.NoError(t, info.IsInitialized())

	validateStart(t, c)

	c = newCluster(context, namespace, true, v1.ResourceRequirements{})
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)

	// starting again should be a no-op, but will not result in an error
	info, err = c.Start(c.ClusterInfo, c.rookImage, cephver.Quincy, c.spec)
	assert.NoError(t, err)
	assert.NoError(t, info.IsInitialized())

	validateStart(t, c)
}

// safety check that if hostNetwork is used no changes occur on an operator restart
func TestOperatorRestartHostNetwork(t *testing.T) {
	namespace := "ns"
	context, err := newTestStartCluster(t, namespace)
	assert.NoError(t, err)

	// cluster without host networking
	c := newCluster(context, namespace, false, v1.ResourceRequirements{})
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)

	// start a basic cluster
	info, err := c.Start(c.ClusterInfo, c.rookImage, cephver.Quincy, c.spec)
	assert.NoError(t, err)
	assert.NoError(t, info.IsInitialized())

	validateStart(t, c)

	// cluster with host networking
	c = newCluster(context, namespace, false, v1.ResourceRequirements{})
	c.spec.Network.HostNetwork = true
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)

	// starting again should be a no-op, but still results in an error
	info, err = c.Start(c.ClusterInfo, c.rookImage, cephver.Quincy, c.spec)
	assert.NoError(t, err)
	assert.NoError(t, info.IsInitialized(), info)

	validateStart(t, c)
}

func validateStart(t *testing.T, c *Cluster) {
	s, err := c.context.Clientset.CoreV1().Secrets(c.Namespace).Get(c.ClusterInfo.Context, AppName, metav1.GetOptions{})
	assert.NoError(t, err) // there shouldn't be an error due the secret existing
	assert.Equal(t, 4, len(s.Data))

	// there is only one pod created. the other two won't be created since the first one doesn't start
	_, err = c.context.Clientset.AppsV1().Deployments(c.Namespace).Get(c.ClusterInfo.Context, "rook-ceph-mon-a", metav1.GetOptions{})
	assert.NoError(t, err)
}

func TestPersistMons(t *testing.T) {
	clientset := test.New(t, 1)
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	c := New(context.TODO(), &clusterd.Context{Clientset: clientset}, "ns", cephv1.ClusterSpec{Annotations: cephv1.AnnotationsSpec{cephv1.KeyClusterMetadata: cephv1.Annotations{"key": "value"}}}, ownerInfo)
	setCommonMonProperties(c, 1, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

	// Persist mon a
	err := c.persistExpectedMonDaemons()
	assert.NoError(t, err)

	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(context.TODO(), EndpointConfigMapName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, "a=1.2.3.1:3300", cm.Data[EndpointDataKey])
	assert.Equal(t, map[string]string{"key": "value"}, cm.Annotations)

	// Persist mon b, and remove mon a for simply testing the configmap is updated
	c.ClusterInfo.Monitors["b"] = &cephclient.MonInfo{Name: "b", Endpoint: "4.5.6.7:3300"}
	delete(c.ClusterInfo.Monitors, "a")
	err = c.persistExpectedMonDaemons()
	assert.NoError(t, err)

	cm, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(context.TODO(), EndpointConfigMapName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, "b=4.5.6.7:3300", cm.Data[EndpointDataKey])
}

func TestSaveMonEndpoints(t *testing.T) {
	ctx := context.TODO()
	clientset := test.New(t, 1)
	configDir := t.TempDir()
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	c := New(ctx, &clusterd.Context{Clientset: clientset, ConfigDir: configDir}, "ns", cephv1.ClusterSpec{}, ownerInfo)
	setCommonMonProperties(c, 1, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

	// create the initial config map
	err := c.saveMonConfig()
	assert.NoError(t, err)

	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(ctx, EndpointConfigMapName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, "a=1.2.3.1:3300", cm.Data[EndpointDataKey])
	assert.Equal(t, `{"node":{}}`, cm.Data[opcontroller.MappingKey])
	assert.Equal(t, "-1", cm.Data[opcontroller.MaxMonIDKey])

	// update the config map
	c.ClusterInfo.Monitors["a"].Endpoint = "2.3.4.5:6789"
	c.maxMonID = 2
	c.mapping.Schedule["a"] = &opcontroller.MonScheduleInfo{
		Name:     "node0",
		Address:  "1.1.1.1",
		Hostname: "myhost",
	}
	err = c.saveMonConfig()
	assert.NoError(t, err)

	cm, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(ctx, EndpointConfigMapName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, "a=2.3.4.5:6789", cm.Data[EndpointDataKey])
	assert.Equal(t, `{"node":{"a":{"Name":"node0","Hostname":"myhost","Address":"1.1.1.1"}}}`, cm.Data[opcontroller.MappingKey])
	assert.Equal(t, "-1", cm.Data[opcontroller.MaxMonIDKey])

	// Update the maxMonID to some random value
	cm.Data[opcontroller.MaxMonIDKey] = "23"
	_, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
	assert.NoError(t, err)
	// Confirm the maxMonId will be persisted and not updated to anything else.
	// The value is only expected to be set directly to the configmap when a mon deployment is started.
	err = c.saveMonConfig()
	assert.NoError(t, err)
	cm, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(ctx, EndpointConfigMapName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, "23", cm.Data[opcontroller.MaxMonIDKey])
}

func TestMaxMonID(t *testing.T) {
	clientset := test.New(t, 1)
	configDir := t.TempDir()
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	c := New(context.TODO(), &clusterd.Context{Clientset: clientset, ConfigDir: configDir}, "ns", cephv1.ClusterSpec{}, ownerInfo)
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)

	// when the configmap is not found, the maxMonID is -1
	maxMonID, err := c.getStoredMaxMonID()
	assert.NoError(t, err)
	assert.Equal(t, "-1", maxMonID)

	// initialize the configmap
	setCommonMonProperties(c, 1, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")
	err = c.saveMonConfig()
	assert.NoError(t, err)

	// invalid mon names won't update the maxMonID
	err = c.commitMaxMonID("bad-id")
	assert.Error(t, err)

	// starting a mon deployment will set the maxMonID
	err = c.commitMaxMonID("a")
	assert.NoError(t, err)
	maxMonID, err = c.getStoredMaxMonID()
	assert.NoError(t, err)
	assert.Equal(t, "0", maxMonID)

	// set to a higher id
	err = c.commitMaxMonID("d")
	assert.NoError(t, err)
	maxMonID, err = c.getStoredMaxMonID()
	assert.NoError(t, err)
	assert.Equal(t, "3", maxMonID)

	// setting to an id lower than the max will not update it
	err = c.commitMaxMonID("c")
	assert.NoError(t, err)
	maxMonID, err = c.getStoredMaxMonID()
	assert.NoError(t, err)
	assert.Equal(t, "3", maxMonID)
}

func TestMonInQuorum(t *testing.T) {
	entry := cephclient.MonMapEntry{Name: "foo", Rank: 23}
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

func TestNameToIndex(t *testing.T) {
	// invalid
	id, err := fullNameToIndex("rook-ceph-monitor0")
	assert.NotNil(t, err)
	assert.Equal(t, -1, id)
	id, err = fullNameToIndex("rook-ceph-mon123")
	assert.NotNil(t, err)
	assert.Equal(t, -1, id)

	// valid
	id, err = fullNameToIndex("b")
	assert.NoError(t, err)
	assert.Equal(t, 1, id)
	id, err = fullNameToIndex("m")
	assert.NoError(t, err)
	assert.Equal(t, 12, id)
	id, err = fullNameToIndex("rook-ceph-mon-a")
	assert.NoError(t, err)
	assert.Equal(t, 0, id)
}

func TestWaitForQuorum(t *testing.T) {
	namespace := "ns"
	quorumChecks := 0
	quorumResponse := func() (string, error) {
		mons := map[string]*cephclient.MonInfo{
			"a": {},
		}
		quorumChecks++
		if quorumChecks == 1 {
			// return an error the first time while we're waiting for the mon to join quorum
			return "", errors.New("test error")
		}
		// a successful response indicates that we have quorum, even if we didn't check which specific mons were in quorum
		return clienttest.MonInQuorumResponseFromMons(mons), nil
	}
	context, err := newTestStartClusterWithQuorumResponse(t, namespace, quorumResponse)
	assert.NoError(t, err)
	requireAllInQuorum := false
	expectedMons := []string{"a"}
	clusterInfo := cephclient.AdminTestClusterInfo("mycluster")
	err = waitForQuorumWithMons(context, clusterInfo, expectedMons, 0, requireAllInQuorum)
	assert.NoError(t, err)
}

func TestMonFoundInQuorum(t *testing.T) {
	response := cephclient.MonStatusResponse{}

	// "a" is in quorum
	response.Quorum = []int{0}
	response.MonMap.Mons = []cephclient.MonMapEntry{
		{Name: "a", Rank: 0},
		{Name: "b", Rank: 1},
		{Name: "c", Rank: 2},
	}
	assert.True(t, monFoundInQuorum("a", response))
	assert.False(t, monFoundInQuorum("b", response))
	assert.False(t, monFoundInQuorum("c", response))

	// b and c also in quorum, but not d
	response.Quorum = []int{0, 1, 2}
	assert.True(t, monFoundInQuorum("a", response))
	assert.True(t, monFoundInQuorum("b", response))
	assert.True(t, monFoundInQuorum("c", response))
	assert.False(t, monFoundInQuorum("d", response))
}

func TestConfigureArbiter(t *testing.T) {
	c := &Cluster{spec: cephv1.ClusterSpec{
		Mon: cephv1.MonSpec{
			StretchCluster: &cephv1.StretchClusterSpec{
				Zones: []cephv1.MonZoneSpec{
					{Name: "a", Arbiter: true},
					{Name: "b"},
					{Name: "c"},
				},
			},
		},
	}}
	c.arbiterMon = "arb"
	currentArbiter := c.arbiterMon
	setNewTiebreaker := false
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("%s %v", command, args)
			if args[0] == "mon" {
				if args[1] == "dump" {
					return fmt.Sprintf(`{"tiebreaker_mon": "%s", "stretch_mode": true}`, currentArbiter), nil
				}
				if args[1] == "set_new_tiebreaker" {
					assert.Equal(t, c.arbiterMon, args[2])
					setNewTiebreaker = true
					return "", nil
				}
			}
			return "", fmt.Errorf("unrecognized output file command: %s %v", command, args)
		},
	}
	c.context = &clusterd.Context{Clientset: test.New(t, 5), Executor: executor}
	c.ClusterInfo = clienttest.CreateTestClusterInfo(5)

	t.Run("no arbiter failover for old ceph version", func(t *testing.T) {
		c.arbiterMon = "changed"
		c.ClusterInfo.CephVersion = cephver.CephVersion{Major: 16, Minor: 2, Extra: 6}
		err := c.ConfigureArbiter()
		assert.NoError(t, err)
		assert.False(t, setNewTiebreaker)
	})
	t.Run("stretch mode already configured - new", func(t *testing.T) {
		c.arbiterMon = currentArbiter
		c.ClusterInfo.CephVersion = cephver.CephVersion{Major: 16, Minor: 2, Extra: 7}
		err := c.ConfigureArbiter()
		assert.NoError(t, err)
		assert.False(t, setNewTiebreaker)
	})
	t.Run("tiebreaker changed", func(t *testing.T) {
		c.arbiterMon = "changed"
		c.ClusterInfo.CephVersion = cephver.CephVersion{Major: 16, Minor: 2, Extra: 7}
		err := c.ConfigureArbiter()
		assert.NoError(t, err)
		assert.True(t, setNewTiebreaker)
	})
}

func TestFindAvailableZoneMon(t *testing.T) {
	c := &Cluster{spec: cephv1.ClusterSpec{
		Mon: cephv1.MonSpec{
			Zones: []cephv1.MonZoneSpec{
				{Name: "a"},
				{Name: "b"},
				{Name: "c"},
			},
		},
	}}

	// No mons are assigned to a zone yet
	existingMons := []*monConfig{}
	availableZone, err := c.findAvailableZone(existingMons)
	assert.NoError(t, err)
	assert.NotEqual(t, "", availableZone)

	// With 3 mons, we have one available zone
	existingMons = []*monConfig{
		{ResourceName: "x", Zone: "a"},
		{ResourceName: "y", Zone: "b"},
	}
	c.spec.Mon.Count = 3
	availableZone, err = c.findAvailableZone(existingMons)
	assert.NoError(t, err)
	assert.Equal(t, "c", availableZone)

	// With 3 mons and no available zones
	existingMons = []*monConfig{
		{ResourceName: "x", Zone: "a"},
		{ResourceName: "y", Zone: "b"},
		{ResourceName: "z", Zone: "c"},
	}
	c.spec.Mon.Count = 3
	availableZone, err = c.findAvailableZone(existingMons)
	assert.Error(t, err)
	assert.Equal(t, "", availableZone)
}
func TestFindAvailableZoneForStretchedMon(t *testing.T) {
	c := &Cluster{spec: cephv1.ClusterSpec{
		Mon: cephv1.MonSpec{
			StretchCluster: &cephv1.StretchClusterSpec{
				Zones: []cephv1.MonZoneSpec{
					{Name: "a", Arbiter: true},
					{Name: "b"},
					{Name: "c"},
				},
			},
		},
	}}

	// No mons are assigned to a zone yet
	existingMons := []*monConfig{}
	availableZone, err := c.findAvailableZone(existingMons)
	assert.NoError(t, err)
	assert.NotEqual(t, "", availableZone)

	// With 3 mons, we have one available zone
	existingMons = []*monConfig{
		{ResourceName: "x", Zone: "a"},
		{ResourceName: "y", Zone: "b"},
	}
	c.spec.Mon.Count = 3
	availableZone, err = c.findAvailableZone(existingMons)
	assert.NoError(t, err)
	assert.Equal(t, "c", availableZone)

	// With 3 mons and no available zones
	existingMons = []*monConfig{
		{ResourceName: "x", Zone: "a"},
		{ResourceName: "y", Zone: "b"},
		{ResourceName: "z", Zone: "c"},
	}
	c.spec.Mon.Count = 3
	availableZone, err = c.findAvailableZone(existingMons)
	assert.Error(t, err)
	assert.Equal(t, "", availableZone)

	// With 5 mons and no available zones
	existingMons = []*monConfig{
		{ResourceName: "w", Zone: "a"},
		{ResourceName: "x", Zone: "b"},
		{ResourceName: "y", Zone: "b"},
		{ResourceName: "z", Zone: "c"},
		{ResourceName: "q", Zone: "c"},
	}
	c.spec.Mon.Count = 5
	availableZone, err = c.findAvailableZone(existingMons)
	assert.Error(t, err)
	assert.Equal(t, "", availableZone)

	// With 5 mons and one available zone
	existingMons = []*monConfig{
		{ResourceName: "w", Zone: "a"},
		{ResourceName: "x", Zone: "b"},
		{ResourceName: "y", Zone: "b"},
		{ResourceName: "z", Zone: "c"},
	}
	availableZone, err = c.findAvailableZone(existingMons)
	assert.NoError(t, err)
	assert.Equal(t, "c", availableZone)

	// With 5 mons and arbiter zone is available zone
	existingMons = []*monConfig{
		{ResourceName: "w", Zone: "b"},
		{ResourceName: "x", Zone: "b"},
		{ResourceName: "y", Zone: "c"},
		{ResourceName: "z", Zone: "c"},
	}
	availableZone, err = c.findAvailableZone(existingMons)
	assert.NoError(t, err)
	assert.Equal(t, "a", availableZone)
}

func TestMonVolumeClaimTemplate(t *testing.T) {
	generalSC := "generalSC"
	zoneSC := "zoneSC"
	defaultTemplate := &v1.PersistentVolumeClaim{Spec: v1.PersistentVolumeClaimSpec{StorageClassName: &generalSC}}
	zoneTemplate := &v1.PersistentVolumeClaim{Spec: v1.PersistentVolumeClaimSpec{StorageClassName: &zoneSC}}
	type fields struct {
		spec cephv1.ClusterSpec
	}
	type args struct {
		mon *monConfig
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *v1.PersistentVolumeClaim
	}{
		{"no template", fields{cephv1.ClusterSpec{}}, args{&monConfig{Zone: "z1"}}, nil},
		{"default template", fields{cephv1.ClusterSpec{Mon: cephv1.MonSpec{VolumeClaimTemplate: defaultTemplate}}}, args{&monConfig{Zone: "z1"}}, defaultTemplate},
		{"default template with 3 zones", fields{cephv1.ClusterSpec{Mon: cephv1.MonSpec{
			VolumeClaimTemplate: defaultTemplate,
			Zones:               []cephv1.MonZoneSpec{{Name: "z1"}, {Name: "z2"}, {Name: "z3"}}}}},
			args{&monConfig{Zone: "z1"}},
			defaultTemplate},
		{"overridden template", fields{cephv1.ClusterSpec{Mon: cephv1.MonSpec{
			VolumeClaimTemplate: defaultTemplate,
			Zones:               []cephv1.MonZoneSpec{{Name: "z1", VolumeClaimTemplate: zoneTemplate}, {Name: "z2"}, {Name: "z3"}}}}},
			args{&monConfig{Zone: "z1"}},
			zoneTemplate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Cluster{
				spec: tt.fields.spec,
			}
			if got := c.monVolumeClaimTemplate(tt.args.mon); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Cluster.monVolumeClaimTemplate() = %v, want %v", got, tt.want)
			}
		})
	}
}
func TestStretchMonVolumeClaimTemplate(t *testing.T) {
	generalSC := "generalSC"
	zoneSC := "zoneSC"
	defaultTemplate := &v1.PersistentVolumeClaim{Spec: v1.PersistentVolumeClaimSpec{StorageClassName: &generalSC}}
	zoneTemplate := &v1.PersistentVolumeClaim{Spec: v1.PersistentVolumeClaimSpec{StorageClassName: &zoneSC}}
	type fields struct {
		spec cephv1.ClusterSpec
	}
	type args struct {
		mon *monConfig
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *v1.PersistentVolumeClaim
	}{
		{"no template", fields{cephv1.ClusterSpec{}}, args{&monConfig{Zone: "z1"}}, nil},
		{"default template", fields{cephv1.ClusterSpec{Mon: cephv1.MonSpec{VolumeClaimTemplate: defaultTemplate}}}, args{&monConfig{Zone: "z1"}}, defaultTemplate},
		{"default template with 3 zones", fields{cephv1.ClusterSpec{Mon: cephv1.MonSpec{
			VolumeClaimTemplate: defaultTemplate,
			StretchCluster:      &cephv1.StretchClusterSpec{Zones: []cephv1.MonZoneSpec{{Name: "z1"}, {Name: "z2"}, {Name: "z3"}}}}}},
			args{&monConfig{Zone: "z1"}},
			defaultTemplate},
		{"overridden template", fields{cephv1.ClusterSpec{Mon: cephv1.MonSpec{
			VolumeClaimTemplate: defaultTemplate,
			StretchCluster:      &cephv1.StretchClusterSpec{Zones: []cephv1.MonZoneSpec{{Name: "z1", VolumeClaimTemplate: zoneTemplate}, {Name: "z2"}, {Name: "z3"}}}}}},
			args{&monConfig{Zone: "z1"}},
			zoneTemplate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Cluster{
				spec: tt.fields.spec,
			}
			if got := c.monVolumeClaimTemplate(tt.args.mon); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Cluster.monVolumeClaimTemplate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestArbiterPlacement(t *testing.T) {
	placement := cephv1.Placement{
		NodeAffinity: &v1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      "foo",
								Operator: v1.NodeSelectorOpExists,
								Values:   []string{"bar"},
							},
						},
					},
				},
			},
		},
	}
	c := &Cluster{spec: cephv1.ClusterSpec{
		Mon: cephv1.MonSpec{
			StretchCluster: &cephv1.StretchClusterSpec{
				Zones: []cephv1.MonZoneSpec{
					{Name: "a", Arbiter: true},
					{Name: "b"},
					{Name: "c"},
				},
			},
		},
	}}

	c.spec.Placement = cephv1.PlacementSpec{}
	c.spec.Placement[cephv1.KeyMonArbiter] = placement

	// No placement is found if not requesting the arbiter placement
	result := c.getMonPlacement("c")
	assert.Equal(t, cephv1.Placement{}, result)

	// Placement is found if requesting the arbiter
	result = c.getMonPlacement("a")
	assert.Equal(t, placement, result)

	// Arbiter and all mons have the same placement if no arbiter placement is specified
	c.spec.Placement = cephv1.PlacementSpec{}
	c.spec.Placement[cephv1.KeyMon] = placement
	result = c.getMonPlacement("a")
	assert.Equal(t, placement, result)
	result = c.getMonPlacement("c")
	assert.Equal(t, placement, result)
}

func TestCheckIfArbiterReady(t *testing.T) {

	c := &Cluster{
		Namespace: "ns",
		spec: cephv1.ClusterSpec{
			Mon: cephv1.MonSpec{
				StretchCluster: &cephv1.StretchClusterSpec{
					Zones: []cephv1.MonZoneSpec{
						{Name: "a", Arbiter: true},
						{Name: "b"},
						{Name: "c"},
					},
				},
			},
		}}
	crushZoneCount := 0
	balanced := true
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			switch {
			case args[0] == "osd" && args[1] == "crush" && args[2] == "dump":
				crushBuckets := `
					{"id": -1,"name": "default","type_id": 10,"type_name": "root","weight": 1028},
					{"id": -2,"name": "default~hdd","type_id": 10,"type_name": "root","weight": 1028},
					{"id": -3,"name": "mynode","type_id": 1,"type_name": "host","weight": 1028},
					{"id": -4,"name": "mynode~hdd","type_id": 1,"type_name": "host","weight": 1028}`
				for i := 0; i < crushZoneCount; i++ {
					weight := 2056
					if !balanced && i%2 == 1 {
						// simulate unbalanced with every other zone having half the weight
						weight = 1028
					}
					crushBuckets = crushBuckets +
						fmt.Sprintf(`,{"id": -%d,"name": "zone%d","type_id": 1,"type_name": "zone","weight": %d}
						 ,{"id": -%d,"name": "zone%d~ssd","type_id": 1,"type_name": "zone","weight": 2056}`, i+5, i, weight, i+6, i)
				}
				return fmt.Sprintf(`{"buckets": [%s]}`, crushBuckets), nil

			}
			return "", fmt.Errorf("unrecognized output file command: %s %v", command, args)
		},
	}
	c.context = &clusterd.Context{Clientset: test.New(t, 5), Executor: executor}
	c.ClusterInfo = clienttest.CreateTestClusterInfo(5)

	// Not ready if no pods running
	ready, err := c.readyToConfigureArbiter(true)
	assert.False(t, ready)
	assert.NoError(t, err)

	// For the remainder of tests, skip checking OSD pods
	// Now there are not enough zones
	ready, err = c.readyToConfigureArbiter(false)
	assert.False(t, ready)
	assert.NoError(t, err)

	// Valid
	crushZoneCount = 2
	ready, err = c.readyToConfigureArbiter(false)
	assert.True(t, ready)
	assert.NoError(t, err)

	// Valid, except the CRUSH map is not balanced
	balanced = false
	ready, err = c.readyToConfigureArbiter(false)
	assert.False(t, ready)
	assert.NoError(t, err)

	// Too many zones in the CRUSH map
	crushZoneCount = 3
	balanced = true
	ready, err = c.readyToConfigureArbiter(false)
	assert.False(t, ready)
	assert.Error(t, err)
}

func TestSkipReconcile(t *testing.T) {
	c := New(context.TODO(), &clusterd.Context{Clientset: test.New(t, 1), ConfigDir: t.TempDir()}, "ns", cephv1.ClusterSpec{}, cephclient.NewMinimumOwnerInfoWithOwnerRef())
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)
	c.ClusterInfo.Namespace = "ns"

	monDeployment := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-ceph-mon-a",
			Namespace: c.ClusterInfo.Namespace,
			Labels: map[string]string{
				k8sutil.AppAttr: AppName,
				config.MonType:  "a",
			},
		}}

	deployment, err := c.context.Clientset.AppsV1().Deployments(c.ClusterInfo.Namespace).Create(c.ClusterInfo.Context, monDeployment, metav1.CreateOptions{})
	assert.NoError(t, err)

	result, err := controller.GetDaemonsToSkipReconcile(c.ClusterInfo.Context, c.context, c.ClusterInfo.Namespace, config.MonType, AppName)
	assert.NoError(t, err)
	assert.Equal(t, 0, result.Len())

	deployment, err = c.context.Clientset.AppsV1().Deployments(c.ClusterInfo.Namespace).Get(c.ClusterInfo.Context, deployment.Name, metav1.GetOptions{})
	assert.NoError(t, err)

	deployment.Labels[cephv1.SkipReconcileLabelKey] = ""
	_, err = c.context.Clientset.AppsV1().Deployments(c.ClusterInfo.Namespace).Update(c.ClusterInfo.Context, deployment, metav1.UpdateOptions{})
	assert.NoError(t, err)

	result, err = controller.GetDaemonsToSkipReconcile(c.ClusterInfo.Context, c.context, c.ClusterInfo.Namespace, config.MonType, AppName)
	assert.NoError(t, err)
	assert.Equal(t, 1, result.Len())
}

func TestHasMonPathChanged(t *testing.T) {
	monDeployment := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-ceph-mon-a",
			Namespace: "test",
			Labels: map[string]string{
				"pvc_name": "test-pvc",
			},
		}}

	pvcTemplate := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-claim",
		},
	}
	t.Run("mon path changed from pv to hostpath", func(t *testing.T) {
		assert.True(t, hasMonPathChanged(monDeployment, nil))
	})

	t.Run("mon path not changed from pv to hostpath", func(t *testing.T) {
		assert.False(t, hasMonPathChanged(monDeployment, pvcTemplate))
	})
	t.Run("mon path changed from hostPath to pvc", func(t *testing.T) {
		delete(monDeployment.Labels, "pvc_name")
		assert.True(t, hasMonPathChanged(monDeployment, pvcTemplate))
	})

	t.Run("mon path not changed from hostPath to pvc", func(t *testing.T) {
		delete(monDeployment.Labels, "pvc_name")
		assert.False(t, hasMonPathChanged(monDeployment, nil))
	})
}
