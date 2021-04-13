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
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

var namespace = "rook-ceph"

var cephCluster = &cephv1.CephCluster{
	ObjectMeta: metav1.ObjectMeta{Name: "ceph-cluster"}}

var pdbConfigMap = &corev1.ConfigMap{
	ObjectMeta: metav1.ObjectMeta{Name: pdbStateMapName, Namespace: namespace}}

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

func getFakeReconciler(t *testing.T, obj ...runtime.Object) *ReconcileClusterDisruption {
	scheme := scheme.Scheme
	err := policyv1beta1.AddToScheme(scheme)
	if err != nil {
		assert.Fail(t, "failed to build scheme")
	}
	err = appsv1.AddToScheme(scheme)
	if err != nil {
		assert.Fail(t, "failed to build scheme")
	}
	err = corev1.AddToScheme(scheme)
	if err != nil {
		assert.Fail(t, "failed to build scheme")
	}
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
		label                          string
		osds                           []appsv1.Deployment
		expectedAllFailureDomains      []string
		expectedDrainingFailureDomains []string
	}{
		{
			label: "case 1: all osds are running",
			osds: []appsv1.Deployment{fakeOSDDeployment(1, 1), fakeOSDDeployment(2, 1),
				fakeOSDDeployment(3, 1)},
			expectedAllFailureDomains:      []string{"zone-1", "zone-2", "zone-3"},
			expectedDrainingFailureDomains: []string{},
		},
		{
			label: "case 1: osd in zone-1 is pending",
			osds: []appsv1.Deployment{fakeOSDDeployment(1, 0), fakeOSDDeployment(2, 1),
				fakeOSDDeployment(3, 1)},
			expectedAllFailureDomains:      []string{"zone-2", "zone-3"},
			expectedDrainingFailureDomains: []string{"zone-1"},
		},
		{
			label: "case 1: osds in zone-1 and zone-2 are pending",
			osds: []appsv1.Deployment{fakeOSDDeployment(1, 0), fakeOSDDeployment(2, 0),
				fakeOSDDeployment(3, 1)},
			expectedAllFailureDomains:      []string{"zone-3"},
			expectedDrainingFailureDomains: []string{"zone-1", "zone-2"},
		},
	}

	for _, tc := range testcases {
		r := getFakeReconciler(t, cephCluster, &corev1.ConfigMap{}, tc.osds[0].DeepCopy(),
			tc.osds[1].DeepCopy(), tc.osds[2].DeepCopy())
		clusterInfo := getFakeClusterInfo()
		request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}
		allfailureDomains, drainingFailureDomains, err := r.getOSDFailureDomains(clusterInfo, request, "zone")
		assert.NoError(t, err)
		assert.Equal(t, tc.expectedAllFailureDomains, allfailureDomains)
		assert.Equal(t, tc.expectedDrainingFailureDomains, drainingFailureDomains)
	}
}

func TestGetOSDFailureDomainsError(t *testing.T) {
	testcases := []struct {
		label                          string
		osds                           []appsv1.Deployment
		expectedAllFailureDomains      []string
		expectedDrainingFailureDomains []string
	}{
		{
			label: "case 1: one or more OSD deployment is missing crush location label",
			osds: []appsv1.Deployment{fakeOSDDeployment(1, 1), fakeOSDDeployment(2, 1),
				fakeOSDDeployment(3, 1)},
			expectedAllFailureDomains:      []string{},
			expectedDrainingFailureDomains: []string{},
		},
	}

	for _, tc := range testcases {
		osd := tc.osds[0].DeepCopy()
		osd.Spec.Template.ObjectMeta.Labels["topology-location-zone"] = ""
		r := getFakeReconciler(t, cephCluster, &corev1.ConfigMap{},
			tc.osds[1].DeepCopy(), tc.osds[2].DeepCopy(), osd)
		clusterInfo := getFakeClusterInfo()
		request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}
		allfailureDomains, drainingFailureDomains, err := r.getOSDFailureDomains(clusterInfo, request, "zone")
		assert.Error(t, err)
		assert.Equal(t, tc.expectedAllFailureDomains, allfailureDomains)
		assert.Equal(t, tc.expectedDrainingFailureDomains, drainingFailureDomains)
	}
}

func TestReconcilePDBForOSD(t *testing.T) {
	pdbConfig := pdbConfigMap.DeepCopy()
	r := getFakeReconciler(t, cephCluster, pdbConfig)
	clusterInfo := getFakeClusterInfo()
	request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}

	testcases := []struct {
		label                             string
		fakeCephStatus                    string
		fakeOSDDump                       string
		allFailureDomains                 []string
		drainingFailureDomains            []string
		expectedOSDPDBCount               int
		expectedMaxUnavailableCount       int
		expectedDrainingFailureDomainName string
	}{
		{
			label:                             "case 1: no draining failure domain and all pgs are healthy",
			fakeCephStatus:                    healthyCephStatus,
			fakeOSDDump:                       `{"OSDs": [{"OSD": 3, "Up": 3, "In": 3}]}`,
			allFailureDomains:                 []string{"zone-1", "zone-2", "zone-3"},
			drainingFailureDomains:            []string{},
			expectedOSDPDBCount:               1,
			expectedMaxUnavailableCount:       1,
			expectedDrainingFailureDomainName: "",
		},
		{
			label:                             "case 2: zone-1 failure domain is draining and pgs are unhealthy",
			fakeCephStatus:                    unHealthyCephStatus,
			fakeOSDDump:                       `{"OSDs": [{"OSD": 3, "Up": 3, "In": 2}]}`,
			allFailureDomains:                 []string{"zone-1", "zone-2", "zone-3"},
			drainingFailureDomains:            []string{"zone-1"},
			expectedOSDPDBCount:               2,
			expectedMaxUnavailableCount:       0,
			expectedDrainingFailureDomainName: "zone-1",
		},
		{
			label:                             "case 3: zone-1 is back online. But pgs are still unhealthy from zone-1 drain",
			fakeCephStatus:                    unHealthyCephStatus,
			fakeOSDDump:                       `{"OSDs": [{"OSD": 3, "Up": 3, "In": 3}]}`,
			allFailureDomains:                 []string{"zone-1", "zone-2", "zone-3"},
			drainingFailureDomains:            []string{},
			expectedOSDPDBCount:               2,
			expectedMaxUnavailableCount:       0,
			expectedDrainingFailureDomainName: "zone-1",
		},
		{
			label:                             "case 4: zone-1 is back online and pgs are also healthy",
			fakeCephStatus:                    healthyCephStatus,
			fakeOSDDump:                       `{"OSDs": [{"OSD": 3, "Up": 3, "In": 3}]}`,
			allFailureDomains:                 []string{"zone-1", "zone-2", "zone-3"},
			drainingFailureDomains:            []string{},
			expectedOSDPDBCount:               1,
			expectedMaxUnavailableCount:       1,
			expectedDrainingFailureDomainName: "",
		},
	}

	for _, tc := range testcases {
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommandWithOutputFile = func(command, outputFile string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "status" {
				return tc.fakeCephStatus, nil
			}
			if args[0] == "osd" && args[1] == "dump" {
				return tc.fakeOSDDump, nil
			}
			return "", errors.Errorf("unexpected ceph command '%v'", args)
		}
		r.context = &controllerconfig.Context{ClusterdContext: &clusterd.Context{Executor: executor}}
		err := r.reconcilePDBsForOSDs(clusterInfo, request, pdbConfig, "zone", tc.allFailureDomains, tc.drainingFailureDomains)
		assert.NoError(t, err)

		// assert that pdb for osd are created correctly
		existingPDBs := &policyv1beta1.PodDisruptionBudgetList{}
		err = r.client.List(context.TODO(), existingPDBs)
		assert.NoError(t, err)
		assert.Equal(t, tc.expectedOSDPDBCount, len(existingPDBs.Items))
		for _, pdb := range existingPDBs.Items {
			assert.Equal(t, tc.expectedMaxUnavailableCount, pdb.Spec.MaxUnavailable.IntValue())
		}

		// assert that config map is updated with correct failure domain
		existingConfigMaps := &corev1.ConfigMapList{}
		err = r.client.List(context.TODO(), existingConfigMaps)
		assert.NoError(t, err)
		assert.Equal(t, tc.expectedDrainingFailureDomainName, existingConfigMaps.Items[0].Data[drainingFailureDomainKey])
	}
}

func TestPGHealthcheckTimeout(t *testing.T) {
	pdbConfig := pdbConfigMap.DeepCopy()
	r := getFakeReconciler(t, cephCluster, pdbConfig)
	clusterInfo := getFakeClusterInfo()
	request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutputFile = func(command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "status" {
			return unHealthyCephStatus, nil
		}
		if args[0] == "osd" && args[1] == "dump" {
			return `{"OSDs": [{"OSD": 3, "Up": 3, "In": 3}]}`, nil
		}
		return "", errors.Errorf("unexpected ceph command '%v'", args)
	}
	r.context = &controllerconfig.Context{ClusterdContext: &clusterd.Context{Executor: executor}}
	// set PG health check timeout to 10 minutes
	r.pgHealthCheckTimeout = time.Duration(time.Minute * 10)

	// reconcile OSD PDB with active drains (on zone-1) and unhealthy PGs
	err := r.reconcilePDBsForOSDs(clusterInfo, request, pdbConfig, "zone", []string{"zone-1", "zone-2"}, []string{"zone-1"})
	assert.NoError(t, err)
	assert.Equal(t, "zone-1", pdbConfig.Data[drainingFailureDomainKey])

	// update the pgHealthCheckDuration time by -9 minutes
	pdbConfig.Data[pgHealthCheckDurationKey] = time.Now().Add(time.Duration(-7) * time.Minute).Format(time.RFC3339)
	// reconcile OSD PDB with no active drains and unhealthy PGs
	err = r.reconcilePDBsForOSDs(clusterInfo, request, pdbConfig, "zone", []string{"zone-1", "zone-2"}, []string{})
	assert.NoError(t, err)
	// assert that pdb config map was not reset as the PG health check was not timed out
	assert.Equal(t, "zone-1", pdbConfig.Data[drainingFailureDomainKey])

	// update the drainingFailureDomain time by -9 minutes
	pdbConfig.Data[pgHealthCheckDurationKey] = time.Now().Add(time.Duration(-11) * time.Minute).Format(time.RFC3339)
	// reconcile OSD PDB with no active drains and unhealthy PGs
	err = r.reconcilePDBsForOSDs(clusterInfo, request, pdbConfig, "zone", []string{"zone-1", "zone-2"}, []string{})
	assert.NoError(t, err)
	// assert that pdb config map was reset as the PG health check was timed out
	assert.Equal(t, "", pdbConfig.Data[drainingFailureDomainKey])
}
