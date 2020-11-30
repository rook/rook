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
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	"github.com/rook/rook/pkg/operator/ceph/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/tevino/abool"
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
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				err := clienttest.CreateConfigDir(path.Join(configDir, namespace))
				return "", errors.Wrap(err, "failed testing of start cluster without quorum response")
			}
			return "", nil
		},
		MockExecuteCommandWithOutputFile: func(command string, outFileArg string, args ...string) (string, error) {
			// mock quorum health check because a second `Start()` triggers a health check
			return monResponse()
		},
	}
	return &clusterd.Context{
		Clientset:                  clientset,
		Executor:                   executor,
		ConfigDir:                  configDir,
		RequestCancelOrchestration: abool.New(),
	}, nil
}

func newCluster(context *clusterd.Context, namespace string, allowMultiplePerNode bool, resources v1.ResourceRequirements) *Cluster {
	return &Cluster{
		ClusterInfo: nil,
		context:     context,
		Namespace:   namespace,
		rookVersion: "myversion",
		spec: cephv1.ClusterSpec{
			Mon: cephv1.MonSpec{
				Count:                3,
				AllowMultiplePerNode: allowMultiplePerNode,
			},
			Resources: map[string]v1.ResourceRequirements{"mon": resources},
		},
		maxMonID:            -1,
		waitForStart:        false,
		monPodRetryInterval: 10 * time.Millisecond,
		monPodTimeout:       1 * time.Second,
		monTimeoutList:      map[string]time.Time{},
		mapping: &Mapping{
			Schedule: map[string]*MonScheduleInfo{},
		},
		ownerInfo: &client.OwnerInfo{},
	}
}

// setCommonMonProperties is a convenience helper for setting common test properties
func setCommonMonProperties(c *Cluster, currentMons int, mon cephv1.MonSpec, rookVersion string) {
	c.ClusterInfo = clienttest.CreateTestClusterInfo(currentMons)
	c.spec.Mon.Count = mon.Count
	c.spec.Mon.AllowMultiplePerNode = mon.AllowMultiplePerNode
	c.rookVersion = rookVersion
}

func TestResourceName(t *testing.T) {
	assert.Equal(t, "rook-ceph-mon-a", resourceName("rook-ceph-mon-a"))
	assert.Equal(t, "rook-ceph-mon123", resourceName("rook-ceph-mon123"))
	assert.Equal(t, "rook-ceph-mon-b", resourceName("b"))
}

func TestStartMonPods(t *testing.T) {
	ctx := context.TODO()
	namespace := "ns"
	context, err := newTestStartCluster(t, namespace)
	assert.Nil(t, err)
	c := newCluster(context, namespace, true, v1.ResourceRequirements{})
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)

	// start a basic cluster
	_, err = c.Start(c.ClusterInfo, c.rookVersion, cephver.Nautilus, c.spec)
	assert.Nil(t, err)

	validateStart(ctx, t, c)

	// starting again should be a no-op, but still results in an error
	_, err = c.Start(c.ClusterInfo, c.rookVersion, cephver.Nautilus, c.spec)
	assert.Nil(t, err)

	validateStart(ctx, t, c)
}

func TestOperatorRestart(t *testing.T) {
	ctx := context.TODO()
	namespace := "ns"
	context, err := newTestStartCluster(t, namespace)
	assert.Nil(t, err)
	c := newCluster(context, namespace, true, v1.ResourceRequirements{})
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)

	// start a basic cluster
	info, err := c.Start(c.ClusterInfo, c.rookVersion, cephver.Nautilus, c.spec)
	assert.Nil(t, err)
	assert.True(t, info.IsInitialized(true))

	validateStart(ctx, t, c)

	c = newCluster(context, namespace, true, v1.ResourceRequirements{})
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)

	// starting again should be a no-op, but will not result in an error
	info, err = c.Start(c.ClusterInfo, c.rookVersion, cephver.Nautilus, c.spec)
	assert.Nil(t, err)
	assert.True(t, info.IsInitialized(true))

	validateStart(ctx, t, c)
}

// safety check that if hostNetwork is used no changes occur on an operator restart
func TestOperatorRestartHostNetwork(t *testing.T) {
	ctx := context.TODO()
	namespace := "ns"
	context, err := newTestStartCluster(t, namespace)
	assert.Nil(t, err)

	// cluster without host networking
	c := newCluster(context, namespace, false, v1.ResourceRequirements{})
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)

	// start a basic cluster
	info, err := c.Start(c.ClusterInfo, c.rookVersion, cephver.Nautilus, c.spec)
	assert.Nil(t, err)
	assert.True(t, info.IsInitialized(true))

	validateStart(ctx, t, c)

	// cluster with host networking
	c = newCluster(context, namespace, false, v1.ResourceRequirements{})
	c.spec.Network.HostNetwork = true
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)

	// starting again should be a no-op, but still results in an error
	info, err = c.Start(c.ClusterInfo, c.rookVersion, cephver.Nautilus, c.spec)
	assert.Nil(t, err)
	assert.True(t, info.IsInitialized(true), info)

	validateStart(ctx, t, c)
}

func validateStart(ctx context.Context, t *testing.T, c *Cluster) {
	s, err := c.context.Clientset.CoreV1().Secrets(c.Namespace).Get(ctx, AppName, metav1.GetOptions{})
	assert.NoError(t, err) // there shouldn't be an error due the secret existing
	assert.Equal(t, 4, len(s.Data))

	// there is only one pod created. the other two won't be created since the first one doesn't start
	_, err = c.context.Clientset.AppsV1().Deployments(c.Namespace).Get(ctx, "rook-ceph-mon-a", metav1.GetOptions{})
	assert.Nil(t, err)
}

func TestSaveMonEndpoints(t *testing.T) {
	ctx := context.TODO()
	clientset := test.New(t, 1)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: configDir}, "ns", cephv1.ClusterSpec{}, &client.OwnerInfo{}, &sync.Mutex{})
	setCommonMonProperties(c, 1, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

	// create the initial config map
	err := c.saveMonConfig()
	assert.Nil(t, err)

	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(ctx, EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "a=1.2.3.1:6789", cm.Data[EndpointDataKey])
	assert.Equal(t, `{"node":{}}`, cm.Data[MappingKey])
	assert.Equal(t, "-1", cm.Data[MaxMonIDKey])

	// update the config map
	c.ClusterInfo.Monitors["a"].Endpoint = "2.3.4.5:6789"
	c.maxMonID = 2
	c.mapping.Schedule["a"] = &MonScheduleInfo{
		Name:     "node0",
		Address:  "1.1.1.1",
		Hostname: "myhost",
	}
	err = c.saveMonConfig()
	assert.Nil(t, err)

	cm, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(ctx, EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "a=2.3.4.5:6789", cm.Data[EndpointDataKey])
	assert.Equal(t, `{"node":{"a":{"Name":"node0","Hostname":"myhost","Address":"1.1.1.1"}}}`, cm.Data[MappingKey])
	assert.Equal(t, "2", cm.Data[MaxMonIDKey])
}

func TestMonInQuorum(t *testing.T) {
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
	assert.Nil(t, err)
	assert.Equal(t, 1, id)
	id, err = fullNameToIndex("m")
	assert.Nil(t, err)
	assert.Equal(t, 12, id)
	id, err = fullNameToIndex("rook-ceph-mon-a")
	assert.Nil(t, err)
	assert.Equal(t, 0, id)
}

func TestWaitForQuorum(t *testing.T) {
	namespace := "ns"
	quorumChecks := 0
	quorumResponse := func() (string, error) {
		mons := map[string]*client.MonInfo{
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
	clusterInfo := &client.ClusterInfo{Namespace: namespace}
	err = waitForQuorumWithMons(context, clusterInfo, expectedMons, 0, requireAllInQuorum)
	assert.Nil(t, err)
}

func TestMonFoundInQuorum(t *testing.T) {
	response := client.MonStatusResponse{}

	// "a" is in quorum
	response.Quorum = []int{0}
	response.MonMap.Mons = []client.MonMapEntry{
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

func TestFindAvailableZoneForStretchedMon(t *testing.T) {
	c := &Cluster{spec: cephv1.ClusterSpec{
		Mon: cephv1.MonSpec{
			StretchCluster: &cephv1.StretchClusterSpec{
				Zones: []cephv1.StretchClusterZoneSpec{
					{Name: "a", Arbiter: true},
					{Name: "b"},
					{Name: "c"},
				},
			},
		},
	}}

	// No mons are assigned to a zone yet
	existingMons := []*monConfig{}
	availableZone, err := c.findAvailableZoneIfStretched(existingMons)
	assert.NoError(t, err)
	assert.NotEqual(t, "", availableZone)

	// With 3 mons, we have one available zone
	existingMons = []*monConfig{
		{ResourceName: "x", Zone: "a"},
		{ResourceName: "y", Zone: "b"},
	}
	c.spec.Mon.Count = 3
	availableZone, err = c.findAvailableZoneIfStretched(existingMons)
	assert.NoError(t, err)
	assert.Equal(t, "c", availableZone)

	// With 3 mons and no available zones
	existingMons = []*monConfig{
		{ResourceName: "x", Zone: "a"},
		{ResourceName: "y", Zone: "b"},
		{ResourceName: "z", Zone: "c"},
	}
	c.spec.Mon.Count = 3
	availableZone, err = c.findAvailableZoneIfStretched(existingMons)
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
	availableZone, err = c.findAvailableZoneIfStretched(existingMons)
	assert.Error(t, err)
	assert.Equal(t, "", availableZone)

	// With 5 mons and one available zone
	existingMons = []*monConfig{
		{ResourceName: "w", Zone: "a"},
		{ResourceName: "x", Zone: "b"},
		{ResourceName: "y", Zone: "b"},
		{ResourceName: "z", Zone: "c"},
	}
	availableZone, err = c.findAvailableZoneIfStretched(existingMons)
	assert.NoError(t, err)
	assert.Equal(t, "c", availableZone)

	// With 5 mons and arbiter zone is available zone
	existingMons = []*monConfig{
		{ResourceName: "w", Zone: "b"},
		{ResourceName: "x", Zone: "b"},
		{ResourceName: "y", Zone: "c"},
		{ResourceName: "z", Zone: "c"},
	}
	availableZone, err = c.findAvailableZoneIfStretched(existingMons)
	assert.NoError(t, err)
	assert.Equal(t, "a", availableZone)
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
			StretchCluster:      &cephv1.StretchClusterSpec{Zones: []cephv1.StretchClusterZoneSpec{{Name: "z1"}, {Name: "z2"}, {Name: "z3"}}}}}},
			args{&monConfig{Zone: "z1"}},
			defaultTemplate},
		{"overridden template", fields{cephv1.ClusterSpec{Mon: cephv1.MonSpec{
			VolumeClaimTemplate: defaultTemplate,
			StretchCluster:      &cephv1.StretchClusterSpec{Zones: []cephv1.StretchClusterZoneSpec{{Name: "z1", VolumeClaimTemplate: zoneTemplate}, {Name: "z2"}, {Name: "z3"}}}}}},
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
