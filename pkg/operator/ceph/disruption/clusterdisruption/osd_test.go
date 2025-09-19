/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package clusterdisruption

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	healthyCephStatus         = `{"fsid":"877a47e0-7f6c-435e-891a-76983ab8c509","health":{"checks":{},"status":"HEALTH_OK"},"election_epoch":12,"quorum":[0,1,2],"quorum_names":["a","b","c"],"monmap":{"epoch":3,"fsid":"877a47e0-7f6c-435e-891a-76983ab8c509","modified":"2020-11-02 09:58:23.015313","created":"2020-11-02 09:57:37.719235","min_mon_release":14,"min_mon_release_name":"nautilus","features":{"persistent":["kraken","luminous","mimic","osdmap-prune","nautilus"],"optional":[]},"mons":[{"rank":0,"name":"a","public_addrs":{"addrvec":[{"type":"v2","addr":"172.30.74.42:3300","nonce":0},{"type":"v1","addr":"172.30.74.42:6789","nonce":0}]},"addr":"172.30.74.42:6789/0","public_addr":"172.30.74.42:6789/0"},{"rank":1,"name":"b","public_addrs":{"addrvec":[{"type":"v2","addr":"172.30.101.61:3300","nonce":0},{"type":"v1","addr":"172.30.101.61:6789","nonce":0}]},"addr":"172.30.101.61:6789/0","public_addr":"172.30.101.61:6789/0"},{"rank":2,"name":"c","public_addrs":{"addrvec":[{"type":"v2","addr":"172.30.250.55:3300","nonce":0},{"type":"v1","addr":"172.30.250.55:6789","nonce":0}]},"addr":"172.30.250.55:6789/0","public_addr":"172.30.250.55:6789/0"}]},"osdmap":{"osdmap":{"epoch":19,"num_osds":3,"num_up_osds":3,"num_in_osds":3,"num_remapped_pgs":0}},"pgmap":{"pgs_by_state":[{"state_name":"active+clean","count":96}],"num_pgs":96,"num_pools":3,"num_objects":79,"data_bytes":81553681,"bytes_used":3255447552,"bytes_avail":1646011994112,"bytes_total":1649267441664,"read_bytes_sec":853,"write_bytes_sec":5118,"read_op_per_sec":1,"write_op_per_sec":0},"fsmap":{"epoch":9,"id":1,"up":1,"in":1,"max":1,"by_rank":[{"filesystem_id":1,"rank":0,"name":"ocs-storagecluster-cephfilesystem-b","status":"up:active","gid":14161},{"filesystem_id":1,"rank":0,"name":"ocs-storagecluster-cephfilesystem-a","status":"up:standby-replay","gid":24146}],"up:standby":0},"mgrmap":{"epoch":10,"active_gid":14122,"active_name":"a","active_addrs":{"addrvec":[{"type":"v2","addr":"10.131.0.28:6800","nonce":1},{"type":"v1","addr":"10.131.0.28:6801","nonce":1}]}}}`
	unHealthyCephStatus       = `{"fsid":"613975f3-3025-4802-9de1-a2280b950e75","health":{"checks":{"OSD_DOWN":{"severity":"HEALTH_WARN","summary":{"message":"1 osds down"}},"OSD_HOST_DOWN":{"severity":"HEALTH_WARN","summary":{"message":"1 host (1 osds) down"}},"PG_AVAILABILITY":{"severity":"HEALTH_WARN","summary":{"message":"Reduced data availability: 101 pgs stale"}},"POOL_APP_NOT_ENABLED":{"severity":"HEALTH_WARN","summary":{"message":"application not enabled on 1 pool(s)"}}},"status":"HEALTH_WARN","overall_status":"HEALTH_WARN"},"election_epoch":12,"quorum":[0,1,2],"quorum_names":["rook-ceph-mon0","rook-ceph-mon2","rook-ceph-mon1"],"monmap":{"epoch":3,"fsid":"613975f3-3025-4802-9de1-a2280b950e75","modified":"2017-08-11 20:13:02.075679","created":"2017-08-11 20:12:35.314510","features":{"persistent":["kraken","luminous"],"optional":[]},"mons":[{"rank":0,"name":"rook-ceph-mon0","addr":"10.3.0.45:6789/0","public_addr":"10.3.0.45:6789/0"},{"rank":1,"name":"rook-ceph-mon2","addr":"10.3.0.249:6789/0","public_addr":"10.3.0.249:6789/0"},{"rank":2,"name":"rook-ceph-mon1","addr":"10.3.0.252:6789/0","public_addr":"10.3.0.252:6789/0"}]},"osdmap":{"osdmap":{"epoch":17,"num_osds":2,"num_up_osds":1,"num_in_osds":2,"full":false,"nearfull":true,"num_remapped_pgs":0}},"pgmap":{"pgs_by_state":[{"state_name":"stale+active+clean","count":101},{"state_name":"active+clean","count":99}],"num_pgs":200,"num_pools":2,"num_objects":243,"data_bytes":976793635,"bytes_used":13611479040,"bytes_avail":19825307648,"bytes_total":33436786688},"fsmap":{"epoch":1,"by_rank":[]},"mgrmap":{"epoch":3,"active_gid":14111,"active_name":"rook-ceph-mgr0","active_addr":"10.2.73.6:6800/9","available":true,"standbys":[],"modules":["restful","status"],"available_modules":["dashboard","prometheus","restful","status","zabbix"]},"servicemap":{"epoch":1,"modified":"0.000000","services":{}}}`
	healthyCephStatusRemapped = `{"fsid":"e32d91a2-24ff-4953-bc4a-6864d31dd2a0","health":{"status":"HEALTH_OK","checks":{},"mutes":[]},"election_epoch":3,"quorum":[0],"quorum_names":["a"],"quorum_age":1177701,"monmap":{"epoch":1,"min_mon_release_name":"reef","num_mons":1},"osdmap":{"epoch":1800,"num_osds":5,"num_up_osds":5,"osd_up_since":1699834324,"num_in_osds":5,"osd_in_since":1699834304,"num_remapped_pgs":11},"pgmap":{"pgs_by_state":[{"state_name":"active+clean","count":174},{"state_name":"active+remapped+backfilling","count":10},{"state_name":"active+clean+remapped","count":1}],"num_pgs":185,"num_pools":9,"num_objects":2383,"data_bytes":2222656224,"bytes_used":8793104384,"bytes_avail":18050441216,"bytes_total":26843545600,"misplaced_objects":139,"misplaced_total":7149,"misplaced_ratio":0.019443278780248985,"recovering_objects_per_sec":10,"recovering_bytes_per_sec":9739877,"recovering_keys_per_sec":0,"num_objects_recovered":62,"num_bytes_recovered":58471087,"num_keys_recovered":0,"write_bytes_sec":2982994,"read_op_per_sec":0,"write_op_per_sec":26},"fsmap":{"epoch":1,"by_rank":[],"up:standby":0},"mgrmap":{"available":true,"num_standbys":0,"modules":["iostat","nfs","prometheus","restful"],"services":{"prometheus":"http://10.244.0.36:9283/"}},"servicemap":{"epoch":1,"modified":"0.000000","services":{}},"progress_events":{}}`
)

var namespace = "rook-ceph"

var cephCluster = &cephv1.CephCluster{
	ObjectMeta: metav1.ObjectMeta{Name: "ceph-cluster"},
}

func getNodeObject(name string, unschedulable bool) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.NodeSpec{
			Unschedulable: unschedulable,
		},
	}
}

func fakeOSDDeployment(id, readyReplicas int) appsv1.Deployment {
	osd := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("osd-%d", id),
			Namespace: namespace,
			Labels: map[string]string{
				"app":                    "rook-ceph-osd",
				"topology-location-zone": fmt.Sprintf("zone-%d", id),
				"ceph-osd-id":            fmt.Sprintf("%d", id),
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: int32(readyReplicas), // nolint:gosec // G115 no overflow expected for ready replicas
		},
	}
	return osd
}

func fakePDBConfigMap(drainingFailureDomain string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: pdbStateMapName, Namespace: namespace},
		Data:       map[string]string{drainingFailureDomainKey: drainingFailureDomain, setNoOut: ""},
	}
}

func getFakeReconciler(t *testing.T, obj ...runtime.Object) *ReconcileClusterDisruption {
	scheme := scheme.Scheme
	err := policyv1.AddToScheme(scheme)
	assert.NoError(t, err)

	err = appsv1.AddToScheme(scheme)
	assert.NoError(t, err)
	err = corev1.AddToScheme(scheme)
	assert.NoError(t, err)
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(obj...).Build()

	return &ReconcileClusterDisruption{
		client:     client,
		scheme:     scheme,
		clusterMap: &ClusterMap{clusterMap: map[string]*cephv1.CephCluster{namespace: cephCluster}},
	}
}

func getFakeClusterInfo() *client.ClusterInfo {
	sharedClusterMap := &ClusterMap{}
	sharedClusterMap.UpdateClusterMap(namespace, cephCluster)
	return sharedClusterMap.GetClusterInfo(namespace)
}

func TestGetOSDFailureDomains(t *testing.T) {
	testcases := []struct {
		name                           string
		osds                           []appsv1.Deployment
		osdMetadata                    string
		nodes                          []corev1.Node
		expectedAllFailureDomains      []string
		expectedDrainingFailureDomains []string
		expectedOsdDownFailureDomains  []string
		expectedDownOSDs               []int
	}{
		{
			name: "case 1: all osds are running",
			osds: []appsv1.Deployment{
				fakeOSDDeployment(1, 1), fakeOSDDeployment(2, 1),
				fakeOSDDeployment(3, 1),
			},
			nodes:                          []corev1.Node{*getNodeObject("node-1", false), *getNodeObject("node-2", false), *getNodeObject("node-3", false)},
			expectedAllFailureDomains:      []string{"zone-1", "zone-2", "zone-3"},
			expectedOsdDownFailureDomains:  []string{},
			expectedDrainingFailureDomains: []string{},
			expectedDownOSDs:               []int{},
		},
		{
			name: "case 2: osd in zone-1 is pending and node is unschedulable",
			osds: []appsv1.Deployment{
				fakeOSDDeployment(1, 0), fakeOSDDeployment(2, 1),
				fakeOSDDeployment(3, 1),
			},
			nodes:                          []corev1.Node{*getNodeObject("node-1", true), *getNodeObject("node-2", false), *getNodeObject("node-3", false)},
			expectedAllFailureDomains:      []string{"zone-1", "zone-2", "zone-3"},
			expectedOsdDownFailureDomains:  []string{"zone-1"},
			expectedDrainingFailureDomains: []string{"zone-1"},
			expectedDownOSDs:               []int{1},
		},
		{
			name: "case 3: osd in zone-1 and zone-2 are pending and node is unschedulable",
			osds: []appsv1.Deployment{
				fakeOSDDeployment(1, 0), fakeOSDDeployment(2, 0),
				fakeOSDDeployment(3, 1),
			},
			nodes:                          []corev1.Node{*getNodeObject("node-1", true), *getNodeObject("node-2", true), *getNodeObject("node-3", false)},
			expectedAllFailureDomains:      []string{"zone-1", "zone-2", "zone-3"},
			expectedOsdDownFailureDomains:  []string{"zone-1", "zone-2"},
			expectedDrainingFailureDomains: []string{"zone-1", "zone-2"},
			expectedDownOSDs:               []int{1, 2},
		},
		{
			name: "case 4: osd in zone-1 is pending but osd node is schedulable",
			osds: []appsv1.Deployment{
				fakeOSDDeployment(1, 0), fakeOSDDeployment(2, 1),
				fakeOSDDeployment(3, 1),
			},
			nodes:                          []corev1.Node{*getNodeObject("node-1", false), *getNodeObject("node-2", false), *getNodeObject("node-3", false)},
			expectedAllFailureDomains:      []string{"zone-1", "zone-2", "zone-3"},
			expectedOsdDownFailureDomains:  []string{"zone-1"},
			expectedDrainingFailureDomains: []string{},
			expectedDownOSDs:               []int{1},
		},
		{
			name: "case 5: osd in zone-3 is pending and the osd node is not schedulable",
			osds: []appsv1.Deployment{
				fakeOSDDeployment(1, 1), fakeOSDDeployment(2, 1),
				fakeOSDDeployment(3, 0),
			},
			nodes:                          []corev1.Node{*getNodeObject("node-1", false), *getNodeObject("node-2", false), *getNodeObject("node-3", true)},
			expectedAllFailureDomains:      []string{"zone-1", "zone-2", "zone-3"},
			expectedOsdDownFailureDomains:  []string{"zone-3"},
			expectedDrainingFailureDomains: []string{"zone-3"},
			expectedDownOSDs:               []int{3},
		},
		{
			name: "case 6: osd in zone-3 is pending and the osd node does not exist",
			osds: []appsv1.Deployment{
				fakeOSDDeployment(1, 1), fakeOSDDeployment(2, 1),
				fakeOSDDeployment(3, 0),
			},
			nodes:                          []corev1.Node{*getNodeObject("node-1", false), *getNodeObject("node-2", false)},
			expectedAllFailureDomains:      []string{"zone-1", "zone-2", "zone-3"},
			expectedOsdDownFailureDomains:  []string{"zone-3"},
			expectedDrainingFailureDomains: []string{"zone-3"},
			expectedDownOSDs:               []int{3},
		},
		{
			name: "case 7: osd in zone-1 and zone-2 are pending and node does not exist",
			osds: []appsv1.Deployment{
				fakeOSDDeployment(1, 0), fakeOSDDeployment(2, 0),
				fakeOSDDeployment(3, 1),
			},
			nodes:                          []corev1.Node{*getNodeObject("node-3", false)},
			expectedAllFailureDomains:      []string{"zone-1", "zone-2", "zone-3"},
			expectedOsdDownFailureDomains:  []string{"zone-1", "zone-2"},
			expectedDrainingFailureDomains: []string{"zone-1", "zone-2"},
			expectedDownOSDs:               []int{1, 2},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			objs := []runtime.Object{
				cephCluster,
				&corev1.ConfigMap{},
			}
			for _, node := range tc.nodes {
				objs = append(objs, node.DeepCopy())
			}
			for _, osdDeployment := range tc.osds {
				objs = append(objs, osdDeployment.DeepCopy())
			}

			executor := &exectest.MockExecutor{}
			executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" && args[1] == "metadata" {
					return `[{"id": 1, "hostname": "node-1"}, {"id": 2, "hostname": "node-2"}, {"id": 3, "hostname": "node-3"}]`, nil
				}
				return "", errors.Errorf("unexpected ceph command '%v'", args)
			}

			r := getFakeReconciler(t, objs...)
			clusterInfo := getFakeClusterInfo()
			clusterInfo.Context = context.TODO()
			r.context = &controllerconfig.Context{ClusterdContext: &clusterd.Context{Executor: executor}}
			request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}
			allfailureDomains, nodeDrainFailureDomains, osdDownFailureDomains, downOSDs, err := r.getOSDFailureDomains(clusterInfo, request, "zone")
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedAllFailureDomains, allfailureDomains)
			assert.Equal(t, tc.expectedDrainingFailureDomains, nodeDrainFailureDomains)
			assert.Equal(t, tc.expectedOsdDownFailureDomains, osdDownFailureDomains)
			assert.Equal(t, tc.expectedDownOSDs, downOSDs)
		})
	}
}

func TestGetOSDFailureDomainsError(t *testing.T) {
	testcases := []struct {
		name                           string
		osds                           []appsv1.Deployment
		expectedAllFailureDomains      []string
		expectedDrainingFailureDomains []string
		expectedOsdDownFailureDomains  []string
		expectedDownOSDs               []int
	}{
		{
			name: "case 1: one or more OSD deployment is missing crush location label",
			osds: []appsv1.Deployment{
				fakeOSDDeployment(1, 1), fakeOSDDeployment(2, 1),
				fakeOSDDeployment(3, 1),
			},
			expectedAllFailureDomains:      nil,
			expectedDrainingFailureDomains: nil,
			expectedOsdDownFailureDomains:  nil,
			expectedDownOSDs:               nil,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			executor := &exectest.MockExecutor{}
			executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" && args[1] == "metadata" {
					return `[{"id": 1, "hostname": "node-1"}, {"id": 2, "hostname": "node-2"}, {"id": 3, "hostname": "node-3"}]`, nil
				}
				return "", errors.Errorf("unexpected ceph command '%v'", args)
			}
			osd := tc.osds[0].DeepCopy()
			osd.Labels["topology-location-zone"] = ""
			r := getFakeReconciler(t, cephCluster, &corev1.ConfigMap{},
				tc.osds[1].DeepCopy(), tc.osds[2].DeepCopy(), osd)
			r.context = &controllerconfig.Context{ClusterdContext: &clusterd.Context{Executor: executor}}
			clusterInfo := getFakeClusterInfo()
			clusterInfo.Context = context.TODO()
			request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}
			allfailureDomains, nodeDrainFailureDomains, osdDownFailureDomains, downOSDs, err := r.getOSDFailureDomains(clusterInfo, request, "zone")
			assert.Error(t, err)
			assert.Equal(t, tc.expectedAllFailureDomains, allfailureDomains)
			assert.Equal(t, tc.expectedDrainingFailureDomains, nodeDrainFailureDomains)
			assert.Equal(t, tc.expectedOsdDownFailureDomains, osdDownFailureDomains)
			assert.Equal(t, tc.expectedDownOSDs, downOSDs)
		})
	}
}

func TestReconcilePDBForOSD(t *testing.T) {
	testcases := []struct {
		name                              string
		fakeCephStatus                    string
		configMap                         *corev1.ConfigMap
		allFailureDomains                 []string
		osdDownFailureDomains             []string
		activeNodeDrains                  []string
		downOSDs                          []int
		pgHealthyRegex                    string
		expectedSetNoOutValue             string
		expectedOSDPDBCount               int
		expectedMaxUnavailableCount       int
		excludedOSDs                      []string
		expectedDrainingFailureDomainName string
	}{
		{
			name:                              "case 1: No draining failure domains, all OSDs are up/in and all pgs are healthy",
			fakeCephStatus:                    healthyCephStatus,
			allFailureDomains:                 []string{"zone-1", "zone-2", "zone-3"},
			osdDownFailureDomains:             []string{},
			downOSDs:                          []int{},
			activeNodeDrains:                  []string{},
			configMap:                         fakePDBConfigMap(""),
			pgHealthyRegex:                    "",
			expectedSetNoOutValue:             "",
			expectedOSDPDBCount:               1,
			expectedMaxUnavailableCount:       1,
			expectedDrainingFailureDomainName: "",
		},
		{
			name:                              "case 2: zone-1 failure domain is draining and pgs are unhealthy",
			fakeCephStatus:                    unHealthyCephStatus,
			allFailureDomains:                 []string{"zone-1", "zone-2", "zone-3"},
			osdDownFailureDomains:             []string{"zone-1"},
			downOSDs:                          []int{1},
			configMap:                         fakePDBConfigMap(""),
			activeNodeDrains:                  []string{"zone-1"},
			pgHealthyRegex:                    "",
			expectedSetNoOutValue:             "true",
			expectedOSDPDBCount:               2,
			expectedMaxUnavailableCount:       0,
			expectedDrainingFailureDomainName: "zone-1",
		},
		{
			name:                              "case 3: zone-1 is back online. But pgs are still unhealthy from zone-1 drain",
			fakeCephStatus:                    unHealthyCephStatus,
			allFailureDomains:                 []string{"zone-1", "zone-2", "zone-3"},
			osdDownFailureDomains:             []string{},
			downOSDs:                          []int{},
			configMap:                         fakePDBConfigMap("zone-1"),
			activeNodeDrains:                  []string{},
			pgHealthyRegex:                    "",
			expectedSetNoOutValue:             "",
			expectedOSDPDBCount:               2,
			expectedMaxUnavailableCount:       0,
			expectedDrainingFailureDomainName: "zone-1",
		},
		{
			name:                              "case 4: zone-1 is back online and pgs are also healthy",
			fakeCephStatus:                    healthyCephStatus,
			allFailureDomains:                 []string{"zone-1", "zone-2", "zone-3"},
			osdDownFailureDomains:             []string{},
			downOSDs:                          []int{},
			configMap:                         fakePDBConfigMap("zone-1"),
			activeNodeDrains:                  []string{},
			pgHealthyRegex:                    "",
			expectedSetNoOutValue:             "",
			expectedOSDPDBCount:               1,
			expectedMaxUnavailableCount:       1,
			expectedDrainingFailureDomainName: "",
		},
		{
			name:                              "case 5: remapped pgs are regarded as clean if pgHealthyRegex allows it",
			fakeCephStatus:                    healthyCephStatusRemapped,
			allFailureDomains:                 []string{"zone-1", "zone-2", "zone-3"},
			osdDownFailureDomains:             []string{},
			downOSDs:                          []int{},
			configMap:                         fakePDBConfigMap("zone-1"),
			activeNodeDrains:                  []string{},
			pgHealthyRegex:                    `^(active\+clean|active\+clean\+scrubbing|active\+clean\+scrubbing\+deep|active\+clean\+remapped|active\+remapped\+backfilling)$`,
			expectedSetNoOutValue:             "",
			expectedOSDPDBCount:               1,
			expectedMaxUnavailableCount:       1,
			expectedDrainingFailureDomainName: "",
		},
		{
			name:                  "case 6: Cluster is healthy but OSDs are down. MaxUnavailable should be set to 1 and down OSDs excluded from PDB",
			fakeCephStatus:        healthyCephStatus,
			allFailureDomains:     []string{"zone-1", "zone-2", "zone-3"},
			osdDownFailureDomains: []string{},
			// OSD 0 and 1 are down but ceph health is good. Max unavailable should be set to 1, and we should exclude OSD
			// 0 and 1 from the default PDB.
			downOSDs:                          []int{0, 1},
			configMap:                         fakePDBConfigMap("zone-1"),
			activeNodeDrains:                  []string{},
			pgHealthyRegex:                    "",
			expectedSetNoOutValue:             "",
			expectedOSDPDBCount:               1,
			expectedMaxUnavailableCount:       1,
			excludedOSDs:                      []string{"0", "1"},
			expectedDrainingFailureDomainName: "",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			r := getFakeReconciler(t, cephCluster, tc.configMap)
			clusterInfo := getFakeClusterInfo()
			clusterInfo.Context = context.TODO()
			request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}
			executor := &exectest.MockExecutor{}
			executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "status" {
					return tc.fakeCephStatus, nil
				}
				if args[0] == "osd" && args[1] == "dump" {
					return `{"OSDs": [{"OSD": 3, "Up": 3, "In": 3}]}`, nil
				}
				return "", errors.Errorf("unexpected ceph command '%v'", args)
			}
			clientset := test.New(t, 3)

			// check for PDBV1 version
			test.SetFakeKubernetesVersion(clientset, "v1.21.0")
			r.context = &controllerconfig.Context{ClusterdContext: &clusterd.Context{Executor: executor, Clientset: clientset}}

			_, err := r.reconcilePDBsForOSDs(clusterInfo, request, tc.configMap, "zone", tc.allFailureDomains, tc.osdDownFailureDomains, tc.activeNodeDrains, tc.downOSDs, tc.pgHealthyRegex)
			assert.NoError(t, err)

			// assert that pdb for osd are created correctly
			existingPDBsV1 := &policyv1.PodDisruptionBudgetList{}
			err = r.client.List(context.TODO(), existingPDBsV1)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedOSDPDBCount, len(existingPDBsV1.Items))
			if tc.expectedDrainingFailureDomainName != "" {
				// active drains, validate PDBs for non-draining zone
				nonDrainingZones := slices.DeleteFunc(tc.allFailureDomains, func(zone string) bool {
					return zone == tc.expectedDrainingFailureDomainName
				})
				for i, zone := range nonDrainingZones {
					maxUnavailable := intstr.FromInt32(int32(tc.expectedMaxUnavailableCount)) // nolint:gosec // G115 no overflow expected
					expectedPDBSpec := policyv1.PodDisruptionBudgetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								fmt.Sprintf(osd.TopologyLocationLabel, "zone"): zone,
							},
						},
						MaxUnavailable: &maxUnavailable,
					}
					assert.Equal(t, expectedPDBSpec, existingPDBsV1.Items[i].Spec)
				}
			} else {
				// no active drains, validate default PDB
				defaultPDB := existingPDBsV1.Items[0]
				expectedMatchExpressions := []metav1.LabelSelectorRequirement{
					{
						Key:      k8sutil.AppAttr,
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{osdPDBAppName},
					},
				}
				if len(tc.excludedOSDs) > 0 {
					expectedMatchExpressions = append(expectedMatchExpressions, metav1.LabelSelectorRequirement{
						Key:      osdPDBOsdIdLabel,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   tc.excludedOSDs,
					})
				}
				maxUnavailable := intstr.FromInt32(int32(tc.expectedMaxUnavailableCount)) // nolint:gosec // G115 no overflow expected
				expectedPDBSpec := policyv1.PodDisruptionBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchExpressions: expectedMatchExpressions,
					},
					MaxUnavailable: &maxUnavailable,
				}
				assert.Equal(t, expectedPDBSpec, defaultPDB.Spec)
			}

			// assert that config map is updated with correct failure domain
			existingConfigMaps := &corev1.ConfigMapList{}
			err = r.client.List(context.TODO(), existingConfigMaps)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedDrainingFailureDomainName, existingConfigMaps.Items[0].Data[drainingFailureDomainKey])
			assert.Equal(t, tc.expectedSetNoOutValue, existingConfigMaps.Items[0].Data[setNoOut])
		})
	}
}

func TestHasNodeDrained(t *testing.T) {
	osdDeployment := fakeOSDDeployment(1, 1)
	ctx := context.TODO()
	// Not expecting node drain because OSD pod is assigned to a schedulable node
	r := getFakeReconciler(t, getNodeObject("node-1", false), osdDeployment.DeepCopy(), &corev1.ConfigMap{})
	expected, err := hasOSDNodeDrained(ctx, r.client, "node-1")
	assert.NoError(t, err)
	assert.False(t, expected)

	// Expecting node drain because OSD pod is assigned to an unschedulable node
	osdDeployment = fakeOSDDeployment(2, 0)
	r = getFakeReconciler(t, getNodeObject("node-2", true), osdDeployment.DeepCopy(), &corev1.ConfigMap{})
	expected, err = hasOSDNodeDrained(ctx, r.client, "node-2")
	assert.NoError(t, err)
	assert.True(t, expected)

	// Expecting node drain because OSD pod is assigned to a non existent node
	osdDeployment = fakeOSDDeployment(3, 0)
	r = getFakeReconciler(t, osdDeployment.DeepCopy(), &corev1.ConfigMap{})
	expected, err = hasOSDNodeDrained(ctx, r.client, "node-3")
	assert.NoError(t, err)
	assert.True(t, expected)
}

func TestSetPDBConfig(t *testing.T) {
	testcases := []struct {
		name                          string
		pdbConfig                     *corev1.ConfigMap
		osdDownFailureDomains         []string
		drainingFailureDomains        []string
		expectedFailureDomainKeyValue string
		expecteNoOutSetting           string
	}{
		{
			name:                          "case 1: Empty PDB config and no draining failureDomain",
			pdbConfig:                     fakePDBConfigMap(""),
			osdDownFailureDomains:         []string{"zone-1", "zone-2"},
			drainingFailureDomains:        []string{},
			expectedFailureDomainKeyValue: "zone-1",
			expecteNoOutSetting:           "",
		},
		{
			name:                          "case 2: Non-empty PDB config and no draining failureDomain",
			pdbConfig:                     fakePDBConfigMap("zone-2"),
			osdDownFailureDomains:         []string{"zone-1", "zone-2"},
			drainingFailureDomains:        []string{},
			expectedFailureDomainKeyValue: "zone-2",
			expecteNoOutSetting:           "",
		},
		{
			name:                          "case 3: Node drain event should set the no-out flag on the failure domain",
			pdbConfig:                     fakePDBConfigMap(""),
			osdDownFailureDomains:         []string{"zone-1", "zone-2"},
			drainingFailureDomains:        []string{"zone-1"},
			expectedFailureDomainKeyValue: "zone-1",
			expecteNoOutSetting:           "true",
		},
		{
			name:                          "case 4: failure domain with drained nodes should get higher precedence",
			pdbConfig:                     fakePDBConfigMap(""),
			osdDownFailureDomains:         []string{"zone-1", "zone-2", "zone-3"},
			drainingFailureDomains:        []string{"zone-3"},
			expectedFailureDomainKeyValue: "zone-3",
			expecteNoOutSetting:           "true",
		},
		{
			name:                          "case 5: pdb configmap should be updated with new drainingFailureDomain, if previously drained failure domain is up",
			pdbConfig:                     fakePDBConfigMap("zone-1"),
			osdDownFailureDomains:         []string{"zone-2"},
			drainingFailureDomains:        []string{"zone-3"},
			expectedFailureDomainKeyValue: "zone-3",
			expecteNoOutSetting:           "true",
		},

		{
			name:                          "case 6: pdb configmap should be updated with new osdDownFailureDomains, if previously drained failure domain is up",
			pdbConfig:                     fakePDBConfigMap("zone-1"),
			osdDownFailureDomains:         []string{"zone-2"},
			drainingFailureDomains:        []string{},
			expectedFailureDomainKeyValue: "zone-2",
			expecteNoOutSetting:           "",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			setPDBConfig(tc.pdbConfig, tc.osdDownFailureDomains, tc.drainingFailureDomains)
			assert.Equal(t, tc.expectedFailureDomainKeyValue, tc.pdbConfig.Data[drainingFailureDomainKey])
			assert.Equal(t, tc.expecteNoOutSetting, tc.pdbConfig.Data[setNoOut])
		})
	}
}
