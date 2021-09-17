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
	"testing"
	"time"

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
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	healthyCephStatus   = `{"fsid":"877a47e0-7f6c-435e-891a-76983ab8c509","health":{"checks":{},"status":"HEALTH_OK"},"election_epoch":12,"quorum":[0,1,2],"quorum_names":["a","b","c"],"monmap":{"epoch":3,"fsid":"877a47e0-7f6c-435e-891a-76983ab8c509","modified":"2020-11-02 09:58:23.015313","created":"2020-11-02 09:57:37.719235","min_mon_release":14,"min_mon_release_name":"nautilus","features":{"persistent":["kraken","luminous","mimic","osdmap-prune","nautilus"],"optional":[]},"mons":[{"rank":0,"name":"a","public_addrs":{"addrvec":[{"type":"v2","addr":"172.30.74.42:3300","nonce":0},{"type":"v1","addr":"172.30.74.42:6789","nonce":0}]},"addr":"172.30.74.42:6789/0","public_addr":"172.30.74.42:6789/0"},{"rank":1,"name":"b","public_addrs":{"addrvec":[{"type":"v2","addr":"172.30.101.61:3300","nonce":0},{"type":"v1","addr":"172.30.101.61:6789","nonce":0}]},"addr":"172.30.101.61:6789/0","public_addr":"172.30.101.61:6789/0"},{"rank":2,"name":"c","public_addrs":{"addrvec":[{"type":"v2","addr":"172.30.250.55:3300","nonce":0},{"type":"v1","addr":"172.30.250.55:6789","nonce":0}]},"addr":"172.30.250.55:6789/0","public_addr":"172.30.250.55:6789/0"}]},"osdmap":{"osdmap":{"epoch":19,"num_osds":3,"num_up_osds":3,"num_in_osds":3,"num_remapped_pgs":0}},"pgmap":{"pgs_by_state":[{"state_name":"active+clean","count":96}],"num_pgs":96,"num_pools":3,"num_objects":79,"data_bytes":81553681,"bytes_used":3255447552,"bytes_avail":1646011994112,"bytes_total":1649267441664,"read_bytes_sec":853,"write_bytes_sec":5118,"read_op_per_sec":1,"write_op_per_sec":0},"fsmap":{"epoch":9,"id":1,"up":1,"in":1,"max":1,"by_rank":[{"filesystem_id":1,"rank":0,"name":"ocs-storagecluster-cephfilesystem-b","status":"up:active","gid":14161},{"filesystem_id":1,"rank":0,"name":"ocs-storagecluster-cephfilesystem-a","status":"up:standby-replay","gid":24146}],"up:standby":0},"mgrmap":{"epoch":10,"active_gid":14122,"active_name":"a","active_addrs":{"addrvec":[{"type":"v2","addr":"10.131.0.28:6800","nonce":1},{"type":"v1","addr":"10.131.0.28:6801","nonce":1}]}}}`
	unHealthyCephStatus = `{"fsid":"613975f3-3025-4802-9de1-a2280b950e75","health":{"checks":{"OSD_DOWN":{"severity":"HEALTH_WARN","summary":{"message":"1 osds down"}},"OSD_HOST_DOWN":{"severity":"HEALTH_WARN","summary":{"message":"1 host (1 osds) down"}},"PG_AVAILABILITY":{"severity":"HEALTH_WARN","summary":{"message":"Reduced data availability: 101 pgs stale"}},"POOL_APP_NOT_ENABLED":{"severity":"HEALTH_WARN","summary":{"message":"application not enabled on 1 pool(s)"}}},"status":"HEALTH_WARN","overall_status":"HEALTH_WARN"},"election_epoch":12,"quorum":[0,1,2],"quorum_names":["rook-ceph-mon0","rook-ceph-mon2","rook-ceph-mon1"],"monmap":{"epoch":3,"fsid":"613975f3-3025-4802-9de1-a2280b950e75","modified":"2017-08-11 20:13:02.075679","created":"2017-08-11 20:12:35.314510","features":{"persistent":["kraken","luminous"],"optional":[]},"mons":[{"rank":0,"name":"rook-ceph-mon0","addr":"10.3.0.45:6789/0","public_addr":"10.3.0.45:6789/0"},{"rank":1,"name":"rook-ceph-mon2","addr":"10.3.0.249:6789/0","public_addr":"10.3.0.249:6789/0"},{"rank":2,"name":"rook-ceph-mon1","addr":"10.3.0.252:6789/0","public_addr":"10.3.0.252:6789/0"}]},"osdmap":{"osdmap":{"epoch":17,"num_osds":2,"num_up_osds":1,"num_in_osds":2,"full":false,"nearfull":true,"num_remapped_pgs":0}},"pgmap":{"pgs_by_state":[{"state_name":"stale+active+clean","count":101},{"state_name":"active+clean","count":99}],"num_pgs":200,"num_pools":2,"num_objects":243,"data_bytes":976793635,"bytes_used":13611479040,"bytes_avail":19825307648,"bytes_total":33436786688},"fsmap":{"epoch":1,"by_rank":[]},"mgrmap":{"epoch":3,"active_gid":14111,"active_name":"rook-ceph-mgr0","active_addr":"10.2.73.6:6800/9","available":true,"standbys":[],"modules":["restful","status"],"available_modules":["dashboard","prometheus","restful","status","zabbix"]},"servicemap":{"epoch":1,"modified":"0.000000","services":{}}}`
)

var nodeName = "node01"
var namespace = "rook-ceph"

var cephCluster = &cephv1.CephCluster{
	ObjectMeta: metav1.ObjectMeta{Name: "ceph-cluster"},
}

var nodeObj = &corev1.Node{
	ObjectMeta: metav1.ObjectMeta{Name: nodeName},
	Spec: corev1.NodeSpec{
		Unschedulable: false,
	},
}

var unschedulableNodeObj = &corev1.Node{
	ObjectMeta: metav1.ObjectMeta{Name: nodeName},
	Spec: corev1.NodeSpec{
		Unschedulable: true,
	},
}

func fakeOSDDeployment(id, readyReplicas int) appsv1.Deployment {
	osd := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("osd-%d", id),
			Namespace: namespace,
			Labels: map[string]string{
				"app": "rook-ceph-osd",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"topology-location-zone": fmt.Sprintf("zone-%d", id),
						"ceph-osd-id":            fmt.Sprintf("%d", id),
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: int32(readyReplicas),
		},
	}
	return osd
}

func fakeOSDPod(id int, nodeName string) corev1.Pod {
	osdPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("osd-%d", id),
			Namespace: namespace,
			Labels: map[string]string{
				"app":         "rook-ceph-osd",
				"ceph-osd-id": fmt.Sprintf("%d", id),
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
		},
	}
	return osdPod
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
	err = policyv1beta1.AddToScheme(scheme)
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
		osdPods                        []corev1.Pod
		node                           *corev1.Node
		expectedAllFailureDomains      []string
		expectedDrainingFailureDomains []string
		expectedOsdDownFailureDomains  []string
	}{
		{
			name: "case 1: all osds are running",
			osds: []appsv1.Deployment{fakeOSDDeployment(1, 1), fakeOSDDeployment(2, 1),
				fakeOSDDeployment(3, 1)},
			osdPods: []corev1.Pod{fakeOSDPod(1, nodeName), fakeOSDPod(2, nodeName),
				fakeOSDPod(3, nodeName)},
			node:                           nodeObj,
			expectedAllFailureDomains:      []string{"zone-1", "zone-2", "zone-3"},
			expectedOsdDownFailureDomains:  []string{},
			expectedDrainingFailureDomains: []string{},
		},
		{
			name: "case 2: osd in zone-1 is pending and node is unscheduable",
			osds: []appsv1.Deployment{fakeOSDDeployment(1, 0), fakeOSDDeployment(2, 1),
				fakeOSDDeployment(3, 1)},
			osdPods: []corev1.Pod{fakeOSDPod(1, ""), fakeOSDPod(2, nodeName),
				fakeOSDPod(3, nodeName)},
			node:                           nodeObj,
			expectedAllFailureDomains:      []string{"zone-1", "zone-2", "zone-3"},
			expectedOsdDownFailureDomains:  []string{"zone-1"},
			expectedDrainingFailureDomains: []string{"zone-1"},
		},
		{
			name: "case 3: osd in zone-1 and zone-2 are pending and node is unscheduable",
			osds: []appsv1.Deployment{fakeOSDDeployment(1, 0), fakeOSDDeployment(2, 0),
				fakeOSDDeployment(3, 1)},
			osdPods: []corev1.Pod{fakeOSDPod(1, ""), fakeOSDPod(2, ""),
				fakeOSDPod(3, nodeName)},
			node:                           nodeObj,
			expectedAllFailureDomains:      []string{"zone-1", "zone-2", "zone-3"},
			expectedOsdDownFailureDomains:  []string{"zone-1", "zone-2"},
			expectedDrainingFailureDomains: []string{"zone-1", "zone-2"},
		},
		{
			name: "case 4: osd in zone-1 is pending but osd node is scheduable",
			osds: []appsv1.Deployment{fakeOSDDeployment(1, 0), fakeOSDDeployment(2, 1),
				fakeOSDDeployment(3, 1)},
			osdPods: []corev1.Pod{fakeOSDPod(1, nodeName), fakeOSDPod(2, nodeName),
				fakeOSDPod(3, nodeName)},
			node:                           nodeObj,
			expectedAllFailureDomains:      []string{"zone-1", "zone-2", "zone-3"},
			expectedOsdDownFailureDomains:  []string{"zone-1"},
			expectedDrainingFailureDomains: []string{},
		},
		{
			name: "case 5: osd in zone-1 is pending but osd node is not scheduable",
			osds: []appsv1.Deployment{fakeOSDDeployment(1, 0), fakeOSDDeployment(2, 1),
				fakeOSDDeployment(3, 1)},
			osdPods: []corev1.Pod{fakeOSDPod(1, nodeName), fakeOSDPod(2, nodeName),
				fakeOSDPod(3, nodeName)},
			node:                           unschedulableNodeObj,
			expectedAllFailureDomains:      []string{"zone-1", "zone-2", "zone-3"},
			expectedOsdDownFailureDomains:  []string{"zone-1"},
			expectedDrainingFailureDomains: []string{"zone-1"},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			objs := []runtime.Object{
				cephCluster,
				&corev1.ConfigMap{},
				tc.node,
			}
			for _, osdDeployment := range tc.osds {
				objs = append(objs, osdDeployment.DeepCopy())
			}
			for _, osdPod := range tc.osdPods {
				objs = append(objs, osdPod.DeepCopy())
			}
			r := getFakeReconciler(t, objs...)
			clusterInfo := getFakeClusterInfo()
			clusterInfo.Context = context.TODO()
			request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}
			allfailureDomains, nodeDrainFailureDomains, osdDownFailureDomains, err := r.getOSDFailureDomains(clusterInfo, request, "zone")
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedAllFailureDomains, allfailureDomains)
			assert.Equal(t, tc.expectedDrainingFailureDomains, nodeDrainFailureDomains)
			assert.Equal(t, tc.expectedOsdDownFailureDomains, osdDownFailureDomains)
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
	}{
		{
			name: "case 1: one or more OSD deployment is missing crush location label",
			osds: []appsv1.Deployment{fakeOSDDeployment(1, 1), fakeOSDDeployment(2, 1),
				fakeOSDDeployment(3, 1)},
			expectedAllFailureDomains:      nil,
			expectedDrainingFailureDomains: nil,
			expectedOsdDownFailureDomains:  nil,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			osd := tc.osds[0].DeepCopy()
			osd.Spec.Template.ObjectMeta.Labels["topology-location-zone"] = ""
			r := getFakeReconciler(t, cephCluster, &corev1.ConfigMap{},
				tc.osds[1].DeepCopy(), tc.osds[2].DeepCopy(), osd)
			clusterInfo := getFakeClusterInfo()
			clusterInfo.Context = context.TODO()
			request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}
			allfailureDomains, nodeDrainFailureDomains, osdDownFailureDomains, err := r.getOSDFailureDomains(clusterInfo, request, "zone")
			assert.Error(t, err)
			assert.Equal(t, tc.expectedAllFailureDomains, allfailureDomains)
			assert.Equal(t, tc.expectedDrainingFailureDomains, nodeDrainFailureDomains)
			assert.Equal(t, tc.expectedOsdDownFailureDomains, osdDownFailureDomains)
		})
	}
}

func TestReconcilePDBForOSD(t *testing.T) {
	testcases := []struct {
		name                              string
		fakeCephStatus                    string
		fakeOSDDump                       string
		configMap                         *corev1.ConfigMap
		allFailureDomains                 []string
		osdDownFailureDomains             []string
		activeNodeDrains                  bool
		expectedSetNoOutValue             string
		expectedOSDPDBCount               int
		expectedMaxUnavailableCount       int
		expectedDrainingFailureDomainName string
	}{
		{
			name:                              "case 1: no draining failure domain and all pgs are healthy",
			fakeCephStatus:                    healthyCephStatus,
			fakeOSDDump:                       `{"OSDs": [{"OSD": 3, "Up": 3, "In": 3}]}`,
			allFailureDomains:                 []string{"zone-1", "zone-2", "zone-3"},
			osdDownFailureDomains:             []string{},
			configMap:                         fakePDBConfigMap(""),
			activeNodeDrains:                  false,
			expectedSetNoOutValue:             "",
			expectedOSDPDBCount:               1,
			expectedMaxUnavailableCount:       1,
			expectedDrainingFailureDomainName: "",
		},
		{
			name:                              "case 2: zone-1 failure domain is draining and pgs are unhealthy",
			fakeCephStatus:                    unHealthyCephStatus,
			fakeOSDDump:                       `{"OSDs": [{"OSD": 3, "Up": 3, "In": 2}]}`,
			allFailureDomains:                 []string{"zone-1", "zone-2", "zone-3"},
			osdDownFailureDomains:             []string{"zone-1"},
			configMap:                         fakePDBConfigMap(""),
			activeNodeDrains:                  true,
			expectedSetNoOutValue:             "true",
			expectedOSDPDBCount:               2,
			expectedMaxUnavailableCount:       0,
			expectedDrainingFailureDomainName: "zone-1",
		},
		{
			name:                              "case 3: zone-1 is back online. But pgs are still unhealthy from zone-1 drain",
			fakeCephStatus:                    unHealthyCephStatus,
			fakeOSDDump:                       `{"OSDs": [{"OSD": 3, "Up": 3, "In": 3}]}`,
			allFailureDomains:                 []string{"zone-1", "zone-2", "zone-3"},
			osdDownFailureDomains:             []string{},
			configMap:                         fakePDBConfigMap("zone-1"),
			activeNodeDrains:                  true,
			expectedSetNoOutValue:             "",
			expectedOSDPDBCount:               2,
			expectedMaxUnavailableCount:       0,
			expectedDrainingFailureDomainName: "zone-1",
		},
		{
			name:                              "case 4: zone-1 is back online and pgs are also healthy",
			fakeCephStatus:                    healthyCephStatus,
			fakeOSDDump:                       `{"OSDs": [{"OSD": 3, "Up": 3, "In": 3}]}`,
			allFailureDomains:                 []string{"zone-1", "zone-2", "zone-3"},
			osdDownFailureDomains:             []string{},
			configMap:                         fakePDBConfigMap(""),
			activeNodeDrains:                  true,
			expectedSetNoOutValue:             "",
			expectedOSDPDBCount:               1,
			expectedMaxUnavailableCount:       1,
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
					return tc.fakeOSDDump, nil
				}
				return "", errors.Errorf("unexpected ceph command '%v'", args)
			}
			clientset := test.New(t, 3)

			// check for PDBV1 version
			test.SetFakeKubernetesVersion(clientset, "v1.21.0")
			r.context = &controllerconfig.Context{ClusterdContext: &clusterd.Context{Executor: executor, Clientset: clientset}}
			_, err := r.reconcilePDBsForOSDs(clusterInfo, request, tc.configMap, "zone", tc.allFailureDomains, tc.osdDownFailureDomains, tc.activeNodeDrains)
			assert.NoError(t, err)

			// assert that pdb for osd are created correctly
			existingPDBsV1 := &policyv1.PodDisruptionBudgetList{}
			err = r.client.List(context.TODO(), existingPDBsV1)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedOSDPDBCount, len(existingPDBsV1.Items))
			for _, pdb := range existingPDBsV1.Items {
				assert.Equal(t, tc.expectedMaxUnavailableCount, pdb.Spec.MaxUnavailable.IntValue())
			}
			// check for PDBV1Beta1 version
			test.SetFakeKubernetesVersion(clientset, "v1.20.0")
			r.context = &controllerconfig.Context{ClusterdContext: &clusterd.Context{Executor: executor, Clientset: clientset}}
			_, err = r.reconcilePDBsForOSDs(clusterInfo, request, tc.configMap, "zone", tc.allFailureDomains, tc.osdDownFailureDomains, tc.activeNodeDrains)
			assert.NoError(t, err)
			existingPDBsV1Beta1 := &policyv1beta1.PodDisruptionBudgetList{}
			err = r.client.List(context.TODO(), existingPDBsV1Beta1)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedOSDPDBCount, len(existingPDBsV1Beta1.Items))
			for _, pdb := range existingPDBsV1Beta1.Items {
				assert.Equal(t, tc.expectedMaxUnavailableCount, pdb.Spec.MaxUnavailable.IntValue())
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

func TestPGHealthcheckTimeout(t *testing.T) {
	pdbConfig := fakePDBConfigMap("")
	r := getFakeReconciler(t, cephCluster, pdbConfig)
	clusterInfo := getFakeClusterInfo()
	clusterInfo.Context = context.TODO()
	request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "status" {
			return unHealthyCephStatus, nil
		}
		if args[0] == "osd" && args[1] == "dump" {
			return `{"OSDs": [{"OSD": 3, "Up": 3, "In": 3}]}`, nil
		}
		return "", errors.Errorf("unexpected ceph command '%v'", args)
	}
	clientset := test.New(t, 3)
	r.context = &controllerconfig.Context{ClusterdContext: &clusterd.Context{Executor: executor, Clientset: clientset}}
	// set PG health check timeout to 10 minutes
	r.pgHealthCheckTimeout = time.Duration(time.Minute * 10)

	// reconcile OSD PDB with active drains (on zone-1) and unhealthy PGs
	_, err := r.reconcilePDBsForOSDs(clusterInfo, request, pdbConfig, "zone", []string{"zone-1", "zone-2"}, []string{"zone-1"}, true)
	assert.NoError(t, err)
	assert.Equal(t, "zone-1", pdbConfig.Data[drainingFailureDomainKey])
	assert.Equal(t, "true", pdbConfig.Data[setNoOut])

	// update the pgHealthCheckDuration time by -9 minutes
	pdbConfig.Data[pgHealthCheckDurationKey] = time.Now().Add(time.Duration(-7) * time.Minute).Format(time.RFC3339)
	// reconcile OSD PDB with no active drains and unhealthy PGs
	_, err = r.reconcilePDBsForOSDs(clusterInfo, request, pdbConfig, "zone", []string{"zone-1", "zone-2"}, []string{}, true)
	assert.NoError(t, err)
	// assert that pdb config map was not reset as the PG health check was not timed out
	assert.Equal(t, "zone-1", pdbConfig.Data[drainingFailureDomainKey])
	assert.Equal(t, "true", pdbConfig.Data[setNoOut])

	// update the drainingFailureDomain time by -9 minutes
	pdbConfig.Data[pgHealthCheckDurationKey] = time.Now().Add(time.Duration(-11) * time.Minute).Format(time.RFC3339)
	// reconcile OSD PDB with no active drains and unhealthy PGs
	_, err = r.reconcilePDBsForOSDs(clusterInfo, request, pdbConfig, "zone", []string{"zone-1", "zone-2"}, []string{}, false)
	assert.NoError(t, err)
	// assert that pdb config map was reset as the PG health check was timed out
	assert.Equal(t, "", pdbConfig.Data[drainingFailureDomainKey])
	assert.Equal(t, "", pdbConfig.Data[setNoOut])
}

func TestHasNodeDrained(t *testing.T) {
	osdPOD := fakeOSDPod(0, nodeName)
	// Not expecting node drain because OSD pod is assigned to a schedulable node
	r := getFakeReconciler(t, nodeObj, osdPOD.DeepCopy(), &corev1.ConfigMap{})
	expected, err := hasOSDNodeDrained(r.client, namespace, "0")
	assert.NoError(t, err)
	assert.False(t, expected)

	// Expecting node drain because OSD pod is assigned to an unschedulable node
	r = getFakeReconciler(t, unschedulableNodeObj, osdPOD.DeepCopy(), &corev1.ConfigMap{})
	expected, err = hasOSDNodeDrained(r.client, namespace, "0")
	assert.NoError(t, err)
	assert.True(t, expected)

	// Expecting node drain because OSD pod is not assigned to any node
	osdPodObj := osdPOD.DeepCopy()
	osdPodObj.Spec.NodeName = ""
	r = getFakeReconciler(t, nodeObj, osdPodObj, &corev1.ConfigMap{})
	expected, err = hasOSDNodeDrained(r.client, namespace, "0")
	assert.NoError(t, err)
	assert.True(t, expected)
}

func TestGetAllowedDisruptions(t *testing.T) {
	r := getFakeReconciler(t)
	clientset := test.New(t, 3)
	test.SetFakeKubernetesVersion(clientset, "v1.21.0")
	r.context = &controllerconfig.Context{ClusterdContext: &clusterd.Context{Clientset: clientset}}

	// Default PDB is not available
	allowedDisruptions, err := r.getAllowedDisruptions(osdPDBAppName, namespace)
	assert.Error(t, err)
	assert.Equal(t, int32(-1), allowedDisruptions)

	// Default PDB is available
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      osdPDBAppName,
			Namespace: namespace,
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			DisruptionsAllowed: int32(0),
		},
	}
	err = r.client.Create(context.TODO(), pdb)
	assert.NoError(t, err)
	allowedDisruptions, err = r.getAllowedDisruptions(osdPDBAppName, namespace)
	assert.NoError(t, err)
	assert.Equal(t, int32(0), allowedDisruptions)
}
