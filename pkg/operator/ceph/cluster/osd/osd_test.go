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

package osd

import (
	"context"
	"strings"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	fakeclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephclientfake "github.com/rook/rook/pkg/daemon/ceph/client/fake"
	discoverDaemon "github.com/rook/rook/pkg/daemon/discover"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testexec "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	healthyCephStatus   = `{"fsid":"877a47e0-7f6c-435e-891a-76983ab8c509","health":{"checks":{},"status":"HEALTH_OK"},"election_epoch":12,"quorum":[0,1,2],"quorum_names":["a","b","c"],"monmap":{"epoch":3,"fsid":"877a47e0-7f6c-435e-891a-76983ab8c509","modified":"2020-11-02 09:58:23.015313","created":"2020-11-02 09:57:37.719235","min_mon_release":14,"min_mon_release_name":"nautilus","features":{"persistent":["kraken","luminous","mimic","osdmap-prune","nautilus"],"optional":[]},"mons":[{"rank":0,"name":"a","public_addrs":{"addrvec":[{"type":"v2","addr":"172.30.74.42:3300","nonce":0},{"type":"v1","addr":"172.30.74.42:6789","nonce":0}]},"addr":"172.30.74.42:6789/0","public_addr":"172.30.74.42:6789/0"},{"rank":1,"name":"b","public_addrs":{"addrvec":[{"type":"v2","addr":"172.30.101.61:3300","nonce":0},{"type":"v1","addr":"172.30.101.61:6789","nonce":0}]},"addr":"172.30.101.61:6789/0","public_addr":"172.30.101.61:6789/0"},{"rank":2,"name":"c","public_addrs":{"addrvec":[{"type":"v2","addr":"172.30.250.55:3300","nonce":0},{"type":"v1","addr":"172.30.250.55:6789","nonce":0}]},"addr":"172.30.250.55:6789/0","public_addr":"172.30.250.55:6789/0"}]},"osdmap":{"osdmap":{"epoch":19,"num_osds":3,"num_up_osds":3,"num_in_osds":3,"num_remapped_pgs":0}},"pgmap":{"pgs_by_state":[{"state_name":"active+clean","count":96}],"num_pgs":96,"num_pools":3,"num_objects":79,"data_bytes":81553681,"bytes_used":3255447552,"bytes_avail":1646011994112,"bytes_total":1649267441664,"read_bytes_sec":853,"write_bytes_sec":5118,"read_op_per_sec":1,"write_op_per_sec":0},"fsmap":{"epoch":9,"id":1,"up":1,"in":1,"max":1,"by_rank":[{"filesystem_id":1,"rank":0,"name":"ocs-storagecluster-cephfilesystem-b","status":"up:active","gid":14161},{"filesystem_id":1,"rank":0,"name":"ocs-storagecluster-cephfilesystem-a","status":"up:standby-replay","gid":24146}],"up:standby":0},"mgrmap":{"epoch":10,"active_gid":14122,"active_name":"a","active_addrs":{"addrvec":[{"type":"v2","addr":"10.131.0.28:6800","nonce":1},{"type":"v1","addr":"10.131.0.28:6801","nonce":1}]}}}`
	unHealthyCephStatus = `{"fsid":"613975f3-3025-4802-9de1-a2280b950e75","health":{"checks":{"OSD_DOWN":{"severity":"HEALTH_WARN","summary":{"message":"1 osds down"}},"OSD_HOST_DOWN":{"severity":"HEALTH_WARN","summary":{"message":"1 host (1 osds) down"}},"PG_AVAILABILITY":{"severity":"HEALTH_WARN","summary":{"message":"Reduced data availability: 101 pgs stale"}},"POOL_APP_NOT_ENABLED":{"severity":"HEALTH_WARN","summary":{"message":"application not enabled on 1 pool(s)"}}},"status":"HEALTH_WARN","overall_status":"HEALTH_WARN"},"election_epoch":12,"quorum":[0,1,2],"quorum_names":["rook-ceph-mon0","rook-ceph-mon2","rook-ceph-mon1"],"monmap":{"epoch":3,"fsid":"613975f3-3025-4802-9de1-a2280b950e75","modified":"2017-08-11 20:13:02.075679","created":"2017-08-11 20:12:35.314510","features":{"persistent":["kraken","luminous"],"optional":[]},"mons":[{"rank":0,"name":"rook-ceph-mon0","addr":"10.3.0.45:6789/0","public_addr":"10.3.0.45:6789/0"},{"rank":1,"name":"rook-ceph-mon2","addr":"10.3.0.249:6789/0","public_addr":"10.3.0.249:6789/0"},{"rank":2,"name":"rook-ceph-mon1","addr":"10.3.0.252:6789/0","public_addr":"10.3.0.252:6789/0"}]},"osdmap":{"osdmap":{"epoch":17,"num_osds":2,"num_up_osds":1,"num_in_osds":2,"full":false,"nearfull":true,"num_remapped_pgs":0}},"pgmap":{"pgs_by_state":[{"state_name":"stale+active+clean","count":101},{"state_name":"active+clean","count":99}],"num_pgs":200,"num_pools":2,"num_objects":243,"data_bytes":976793635,"bytes_used":13611479040,"bytes_avail":19825307648,"bytes_total":33436786688},"fsmap":{"epoch":1,"by_rank":[]},"mgrmap":{"epoch":3,"active_gid":14111,"active_name":"rook-ceph-mgr0","active_addr":"10.2.73.6:6800/9","available":true,"standbys":[],"modules":["restful","status"],"available_modules":["dashboard","prometheus","restful","status","zabbix"]},"servicemap":{"epoch":1,"modified":"0.000000","services":{}}}`
	// osdDFResults is a JSON representation of the output of `ceph osd df` command
	// which has 5 osds with different storage usage
	// Testing the resize of crush weight for OSDs based on the utilization
	// 1) `ceph osd df`, kb size(in Tib) < crush_weight size -> no reweight
	// 2) `ceph osd df`, kb size(in Tib) = 0 -> no reweight
	// 3) `ceph osd df`, kb size(in Tib)  and crush_weight size has 0.085% difference -> no reweight
	// 4) & 5) `ceph osd df`, kb size(in Tib) and crush_weight size has more than 1% difference -> reweight
	osdDFResults = `
	{"nodes":[
	{"id":0,"device_class":"hdd","name":"osd.0","type":"osd","type_id":0,"crush_weight":0.039093017578125,"depth":2,"pool_weights":{},"reweight":1,"kb":41943040,"kb_used":27640,"kb_used_data":432,"kb_used_omap":1,"kb_used_meta":27198,"kb_avail":41915400,"utilization":0.065898895263671875,"var":0.99448308946989694,"pgs":9,"status":"up"},
	{"id":1,"device_class":"hdd","name":"osd.1","type":"osd","type_id":0,"crush_weight":0.039093017578125,"depth":2,"pool_weights":{},"reweight":1,"kb":0,"kb_used":27960,"kb_used_data":752,"kb_used_omap":1,"kb_used_meta":27198,"kb_avail":41915080,"utilization":0.066661834716796875,"var":1.005996641880547,"pgs":15,"status":"up"},
	{"id":2,"device_class":"hdd","name":"osd.1","type":"osd","type_id":0,"crush_weight":0.039093017578125,"depth":2,"pool_weights":{},"reweight":1,"kb":42333872,"kb_used":27960,"kb_used_data":752,"kb_used_omap":1,"kb_used_meta":27198,"kb_avail":41915080,"utilization":0.066661834716796875,"var":1.005996641880547,"pgs":15,"status":"up"},
	{"id":3,"device_class":"hdd","name":"osd.1","type":"osd","type_id":0,"crush_weight":0.039093017578125,"depth":2,"pool_weights":{},"reweight":1,"kb":9841943040,"kb_used":27960,"kb_used_data":752,"kb_used_omap":1,"kb_used_meta":27198,"kb_avail":41915080,"utilization":0.066661834716796875,"var":1.005996641880547,"pgs":15,"status":"up"},
	{"id":4,"device_class":"hdd","name":"osd.2","type":"osd","type_id":0,"crush_weight":0.039093017578125,"depth":2,"pool_weights":{},"reweight":1,"kb":9991943040,"kb_used":27780,"kb_used_data":564,"kb_used_omap":1,"kb_used_meta":27198,"kb_avail":41915260,"utilization":0.066232681274414062,"var":0.99952026864955634,"pgs":8,"status":"up"}],
	"stray":[],"summary":{"total_kb":125829120,"total_kb_used":83380,"total_kb_used_data":1748,"total_kb_used_omap":3,"total_kb_used_meta":81596,"total_kb_avail":125745740,"average_utilization":0.066264470418294266,"min_var":0.99448308946989694,"max_var":1.005996641880547,"dev":0.00031227879054369131}}`
)

func TestOSDProperties(t *testing.T) {
	osdProps := []osdProperties{
		{
			pvc:         corev1.PersistentVolumeClaimVolumeSource{ClaimName: "claim"},
			metadataPVC: corev1.PersistentVolumeClaimVolumeSource{ClaimName: "claim"},
		},
		{
			pvc:         corev1.PersistentVolumeClaimVolumeSource{ClaimName: ""},
			metadataPVC: corev1.PersistentVolumeClaimVolumeSource{ClaimName: ""},
		},
	}
	expected := [][2]bool{
		{true, true},
		{false, false},
	}
	for i, p := range osdProps {
		actual := [2]bool{p.onPVC(), p.onPVCWithMetadata()}
		assert.Equal(t, expected[i], actual, "detected a problem in `expected[%d]`", i)
	}
}

func TestStart(t *testing.T) {
	namespace := "ns"
	clientset := fake.NewClientset()
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("ExecuteCommandWithOutputFile: %s %v", command, args)
			if args[1] == "crush" && args[2] == "class" && args[3] == "ls" {
				// Mock executor for OSD crush class list command, returning ssd as available device class
				return `["ssd"]`, nil
			}
			if args[0] == "osd" && args[1] == "df" {
				return osdDFResults, nil
			}
			return "", nil
		},
	}

	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testing",
			Namespace: namespace,
		},
		Spec: cephv1.ClusterSpec{},
	}
	// Objects to track in the fake client.
	object := []runtime.Object{
		cephCluster,
	}
	s := scheme.Scheme
	// Create a fake client to mock API calls.
	client := clientfake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   namespace,
		CephVersion: cephver.Squid,
		Context:     context.TODO(),
	}
	clusterInfo.SetName("rook-ceph-test")
	context := &clusterd.Context{Clientset: clientset, Client: client, ConfigDir: "/var/lib/rook", Executor: executor}
	spec := cephv1.ClusterSpec{}
	c := New(context, clusterInfo, spec, "myversion")

	// Start the first time
	err := c.Start()
	assert.Nil(t, err)

	// Should not fail if it already exists
	err = c.Start()
	assert.Nil(t, err)
}

func createDiscoverConfigmap(nodeName, ns string, clientset *fake.Clientset) error {
	ctx := context.TODO()
	data := make(map[string]string, 1)
	data[discoverDaemon.LocalDiskCMData] = `[{"name":"sdx","parent":"","hasChildren":false,"devLinks":"/dev/disk/by-id/scsi-36001405f826bd553d8c4dbf9f41c18be    /dev/disk/by-id/wwn-0x6001405f826bd553d8c4dbf9f41c18be /dev/disk/by-path/ip-127.0.0.1:3260-iscsi-iqn.2016-06.world.srv:storage.target01-lun-1","size":10737418240,"uuid":"","serial":"36001405f826bd553d8c4dbf9f41c18be","type":"disk","rotational":true,"readOnly":false,"ownPartition":true,"filesystem":"","vendor":"LIO-ORG","model":"disk02","wwn":"0x6001405f826bd553","wwnVendorExtension":"0x6001405f826bd553d8c4dbf9f41c18be","empty":true}]`
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "local-device-" + nodeName,
			Namespace: ns,
			Labels: map[string]string{
				k8sutil.AppAttr:         discoverDaemon.AppName,
				discoverDaemon.NodeAttr: nodeName,
			},
		},
		Data: data,
	}
	_, err := clientset.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{})
	return err
}

func createNode(nodeName string, condition corev1.NodeConditionType, clientset *fake.Clientset) error {
	ctx := context.TODO()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type: condition, Status: corev1.ConditionTrue,
				},
			},
		},
	}
	_, err := clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
	return err
}

func TestAddRemoveNode(t *testing.T) {
	ctx := context.TODO()
	namespace := "ns-add-remove"
	// create a storage spec with the given nodes/devices/dirs
	nodeName := "node23"

	oldConditionExportFunc := updateConditionFunc
	defer func() {
		updateConditionFunc = oldConditionExportFunc
	}()
	// stub out the conditionExportFunc to do nothing. we do not have a fake Rook interface that
	// allows us to interact with a CephCluster resource like the fake K8s clientset.
	updateConditionFunc = func(ctx context.Context, c *clusterd.Context, namespaceName types.NamespacedName, observedGeneration int64, conditionType cephv1.ConditionType, status corev1.ConditionStatus, reason cephv1.ConditionReason, message string) {
		// do nothing
	}

	// set up a fake k8s client set and watcher to generate events that the operator will listen to
	clientset := fake.NewClientset()
	t.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")

	testexec.AddReadyNode(t, clientset, nodeName, "23.23.23.23")
	cmErr := createDiscoverConfigmap(nodeName, "rook-system", clientset)
	assert.Nil(t, cmErr)

	statusMapWatcher := watch.NewFake()
	clientset.PrependWatchReactor("configmaps", k8stesting.DefaultWatchReactor(statusMapWatcher, nil))

	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   namespace,
		CephVersion: cephver.Squid,
		Context:     ctx,
	}
	clusterInfo.SetName("rook-ceph-test")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	generateKey := "expected key"
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[1] == "crush" && args[2] == "class" && args[3] == "ls" {
				// Mock executor for OSD crush class list command, returning ssd as available device class
				return `["ssd"]`, nil
			}
			return "{\"key\": \"" + generateKey + "\"}", nil
		},
	}

	context := &clusterd.Context{
		Clientset:     clientset,
		ConfigDir:     "/var/lib/rook",
		Executor:      executor,
		RookClientset: fakeclient.NewSimpleClientset(),
	}

	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testing",
			Namespace: namespace,
		},
		Spec: cephv1.ClusterSpec{
			DataDirHostPath: context.ConfigDir,
			Storage: cephv1.StorageScopeSpec{
				Nodes: []cephv1.Node{
					{
						Name: nodeName,
						Selection: cephv1.Selection{
							Devices: []cephv1.Device{{Name: "sdx"}},
						},
					},
				},
			},
		},
	}

	// Objects to track in the fake client.
	object := []runtime.Object{
		cephCluster,
	}
	s := scheme.Scheme
	// Create a fake client to mock API calls.
	client := clientfake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	context.Client = client

	c := New(context, clusterInfo, cephCluster.Spec, "myversion")

	// kick off the start of the orchestration in a goroutine
	var startErr error
	startCompleted := false
	go func() {
		startErr = c.Start()
		startCompleted = true
	}()

	mockNodeOrchestrationCompletion(c, nodeName, statusMapWatcher)
	waitForOrchestrationCompletion(c, nodeName, &startCompleted)

	// verify orchestration for adding the node succeeded
	assert.True(t, startCompleted)
	assert.NoError(t, startErr)
	_, err := clientset.AppsV1().Deployments(namespace).Get(ctx, deploymentName(1), metav1.GetOptions{})
	assert.NoError(t, err)

	// simulate the OSD pod having been created
	osdPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:   "osdPod",
		Labels: map[string]string{k8sutil.AppAttr: AppName},
	}}
	_, err = c.context.Clientset.CoreV1().Pods(c.clusterInfo.Namespace).Create(ctx, osdPod, metav1.CreateOptions{})
	assert.NoError(t, err)

	// mock the ceph calls that will be called during remove node
	context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "status" {
				return `{"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			if args[0] == "osd" {
				if args[1] == "df" {
					return `{"nodes":[{"id":1,"name":"osd.1","kb_used":0}]}`, nil
				}
				if args[1] == "dump" {
					// OSD 1 is down and out
					return `{"OSDs": [{"OSD": 1, "Up": 0, "In": 0}]}`, nil
				}
				if args[1] == "safe-to-destroy" {
					return `{"safe_to_destroy":[1],"active":[],"missing_stats":[],"stored_pgs":[]}`, nil
				}
				if args[1] == "set" {
					return "", nil
				}
				if args[1] == "unset" {
					return "", nil
				}
				if args[1] == "crush" {
					if args[2] == "reweight" {
						return "", nil
					}
					if args[2] == "rm" {
						return "", nil
					}
					if args[2] == "get-device-class" {
						return cephclientfake.OSDDeviceClassOutput(args[3]), nil
					}
					if args[2] == "class" {
						return `["ssd"]`, nil
					}
				}
				if args[1] == "out" {
					return "", nil
				}
				if args[1] == "rm" {
					assert.Equal(t, "1", args[2])
					return "", nil
				}
				if args[1] == "find" {
					return `{"crush_location":{"host":"my-host"}}`, nil
				}
				if args[1] == "ok-to-stop" {
					return cephclientfake.OsdOkToStopOutput(1, []int{1}), nil
				}
			}
			if args[0] == "df" && args[1] == "detail" {
				return `{"stats":{"total_bytes":0,"total_used_bytes":0,"total_avail_bytes":3072}}`, nil
			}
			if args[0] == "pg" && args[1] == "dump" {
				return `{}`, nil
			}
			if args[0] == "auth" && args[1] == "del" {
				if strings.HasPrefix(args[2], "osd.") {
					assert.Equal(t, "osd.1", args[2])
				} else if strings.HasPrefix(args[2], "client.") {
					assert.Equal(t, "client.bootstrap-osd", args[2])
				}
				return "", nil
			}
			return "", errors.Errorf("unexpected ceph command %q", args)
		},
	}

	// modify the storage spec to remove the node from the cluster
	cephCluster.Spec.Storage.Nodes = []cephv1.Node{}
	c = New(context, clusterInfo, cephCluster.Spec, "myversion")

	// reset the orchestration status watcher
	statusMapWatcher = watch.NewFake()
	clientset.PrependWatchReactor("configmaps", k8stesting.DefaultWatchReactor(statusMapWatcher, nil))

	startErr = nil
	startCompleted = false
	go func() {
		startErr = c.Start()
		startCompleted = true
	}()

	mockNodeOrchestrationCompletion(c, nodeName, statusMapWatcher)
	waitForOrchestrationCompletion(c, nodeName, &startCompleted)

	// verify orchestration for removing the node succeeded
	assert.True(t, startCompleted)
	assert.NoError(t, startErr)
	// deployment should still exist; OSDs are removed by health monitor code only if they are down,
	// out, and the user has set removeOSDsIfOutAndSafeToRemove
	_, err = clientset.AppsV1().Deployments(namespace).Get(ctx, deploymentName(1), metav1.GetOptions{})
	assert.NoError(t, err)

	removeIfOutAndSafeToRemove := true
	healthMon := NewOSDHealthMonitor(context, cephclient.AdminTestClusterInfo(namespace), removeIfOutAndSafeToRemove, cephv1.CephClusterHealthCheckSpec{})
	healthMon.checkOSDHealth()
	_, err = clientset.AppsV1().Deployments(namespace).Get(ctx, deploymentName(1), metav1.GetOptions{})
	assert.True(t, k8serrors.IsNotFound(err))
}

func TestPostReconcileUpdateOSDProperties(t *testing.T) {
	namespace := "ns"
	clientset := fake.NewClientset()
	removedDeviceClassOSD := ""
	setDeviceClassOSD := ""
	setDeviceClass := ""
	var crushWeight []string
	var osdID []string
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("ExecuteCommandWithOutput: %s %v", command, args)
			if args[0] == "osd" {
				if args[1] == "df" {
					return osdDFResults, nil
				}
				if args[1] == "crush" {
					switch args[2] {
					case "rm-device-class":
						removedDeviceClassOSD = args[3]
					case "set-device-class":
						setDeviceClass = args[3]
						setDeviceClassOSD = args[4]
					case "reweight":
						osdID = append(osdID, args[3])
						crushWeight = append(crushWeight, args[4])
					}
				}
			}
			return "", nil
		},
	}

	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testing",
			Namespace: namespace,
		},
	}
	// Objects to track in the fake client.
	object := []runtime.Object{
		cephCluster,
	}
	s := scheme.Scheme
	// Create a fake client to mock API calls.
	client := clientfake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   namespace,
		CephVersion: cephver.Squid,
		Context:     context.TODO(),
	}
	clusterInfo.SetName("rook-ceph-test")
	context := &clusterd.Context{Clientset: clientset, Client: client, ConfigDir: "/var/lib/rook", Executor: executor}
	c := New(context, clusterInfo, cephCluster.Spec, "myversion")

	// Start the first time
	desiredOSDs := map[int]*OSDInfo{
		0: {ID: 0, DeviceClass: "hdd"},
		1: {ID: 1, DeviceClass: "hdd"},
		2: {ID: 2, DeviceClass: "newclass"},
	}
	t.Run("test device class change", func(t *testing.T) {
		c.spec.Storage = cephv1.StorageScopeSpec{AllowDeviceClassUpdate: true}
		err := c.postReconcileUpdateOSDProperties(desiredOSDs)
		assert.Nil(t, err)
		assert.Equal(t, "newclass", setDeviceClass)
		assert.Equal(t, "osd.2", setDeviceClassOSD)
		assert.Equal(t, "osd.2", removedDeviceClassOSD)
	})
	t.Run("test resize Osd Crush Weight", func(t *testing.T) {
		c.spec.Storage = cephv1.StorageScopeSpec{AllowOsdCrushWeightUpdate: true}
		err := c.postReconcileUpdateOSDProperties(desiredOSDs)
		assert.Nil(t, err)
		// only osds with more than 1% change in utilization should be reweighted
		assert.Equal(t, []string([]string{"osd.3", "osd.4"}), osdID)
		assert.Equal(t, []string([]string{"9.166024", "9.305722"}), crushWeight)
	})
}

func TestAddNodeFailure(t *testing.T) {
	// create a storage spec with the given nodes/devices/dirs
	nodeName := "node1672"

	// create a fake clientset that will return an error when the operator tries to create a job
	clientset := fake.NewClientset()
	clientset.PrependReactor("create", "jobs", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("mock failed to create jobs")
	})
	nodeErr := createNode(nodeName, corev1.NodeReady, clientset)
	assert.Nil(t, nodeErr)

	t.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")

	cmErr := createDiscoverConfigmap(nodeName, "rook-system", clientset)
	assert.Nil(t, cmErr)

	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   "ns-add-remove",
		CephVersion: cephver.Squid,
		Context:     context.TODO(),
	}
	clusterInfo.SetName("testcluster")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	context := &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}
	spec := cephv1.ClusterSpec{
		DataDirHostPath: context.ConfigDir,
		Storage: cephv1.StorageScopeSpec{
			Nodes: []cephv1.Node{
				{
					Name: nodeName,
					Selection: cephv1.Selection{
						Devices: []cephv1.Device{{Name: "sdx"}},
					},
				},
			},
		},
	}
	c := New(context, clusterInfo, spec, "myversion")

	// kick off the start of the orchestration in a goroutine
	var startErr error
	startCompleted := false
	go func() {
		startErr = c.Start()
		startCompleted = true
	}()

	// wait for orchestration to complete
	waitForOrchestrationCompletion(c, nodeName, &startCompleted)

	// verify orchestration failed (because the operator failed to create a job)
	assert.True(t, startCompleted)
	assert.NotNil(t, startErr)
}

func TestOSDProvisionCleanup(t *testing.T) {
	// Test that we cleanup orphaned provision status configmaps and provision jobs during reconciliation.
	missingNodeName := "missingNode"
	existingNodeName := "existingNode"
	namespace := "ns"

	clientset := fake.NewClientset()

	// Create orphaned status configmap
	cmName := statusConfigMapName(missingNodeName)
	cmLabels := statusConfigMapLabels(missingNodeName)
	_, err := clientset.CoreV1().ConfigMaps(namespace).Create(context.TODO(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:   cmName,
			Labels: cmLabels,
		},
	}, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create orphaned provision job
	orphanedJobName := provisionJobName(missingNodeName)
	orphanedJobLabels := provisionJobLabels(namespace)
	_, err = clientset.BatchV1().Jobs(namespace).Create(context.TODO(), &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:   orphanedJobName,
			Labels: orphanedJobLabels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						k8sutil.LabelHostname(): missingNodeName,
					},
				},
			},
		},
	}, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create a non-orphaned provision job
	nonOrphanedJobName := provisionJobName(existingNodeName)
	nonOrphanedJobLabels := provisionJobLabels(namespace)
	_, err = clientset.BatchV1().Jobs(namespace).Create(context.TODO(), &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:   nonOrphanedJobName,
			Labels: nonOrphanedJobLabels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						k8sutil.LabelHostname(): existingNodeName,
					},
				},
			},
		},
	}, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create the existing node
	_, err = clientset.CoreV1().Nodes().Create(context.TODO(), &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: existingNodeName,
			Labels: map[string]string{
				k8sutil.LabelHostname(): existingNodeName,
			},
		},
	}, metav1.CreateOptions{})
	assert.NoError(t, err)

	t.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")

	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testing",
			Namespace: namespace,
		},
	}
	// Objects to track in the fake client.
	object := []runtime.Object{
		cephCluster,
	}
	s := scheme.Scheme
	// Create a fake client to mock API calls.
	client := clientfake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   namespace,
		CephVersion: cephver.Squid,
		Context:     context.TODO(),
	}
	clusterInfo.SetName("testcluster")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("ExecuteCommandWithOutput: %s %v", command, args)
			if args[1] == "crush" && args[2] == "class" && args[3] == "ls" {
				// Mock executor for OSD crush class list command, returning ssd as available device class
				return `["ssd"]`, nil
			}
			if args[0] == "osd" && args[1] == "df" {
				return osdDFResults, nil
			}
			return "", nil
		},
	}
	ctx := &clusterd.Context{Clientset: clientset, Client: client, ConfigDir: "/var/lib/rook", Executor: executor}
	spec := cephv1.ClusterSpec{
		DataDirHostPath: ctx.ConfigDir,
		Storage: cephv1.StorageScopeSpec{
			Nodes: []cephv1.Node{},
		},
	}

	c := New(ctx, clusterInfo, spec, "myversion")

	// run the reconciliation
	err = c.Start()
	assert.Nil(t, err)

	// validate that orphaned status configmap was deleted
	_, err = clientset.CoreV1().ConfigMaps(namespace).Get(context.TODO(), cmName, metav1.GetOptions{})
	assert.True(t, k8serrors.IsNotFound(err), "Orphaned status configmap was not deleted.")

	// validate that orphaned provision job was deleted
	_, err = clientset.BatchV1().Jobs(namespace).Get(context.TODO(), orphanedJobName, metav1.GetOptions{})
	assert.True(t, k8serrors.IsNotFound(err), "Orphaned provision job was not deleted.")

	// validate that the non-orphaned provision job was not deleted
	_, err = clientset.BatchV1().Jobs(namespace).Get(context.TODO(), nonOrphanedJobName, metav1.GetOptions{})
	assert.NoError(t, err, "non-orphaned provision job was deleted.")
}

func TestGetPVCHostName(t *testing.T) {
	ctx := context.TODO()
	clientset := fake.NewClientset()
	clusterInfo := &cephclient.ClusterInfo{Namespace: "ns"}
	clusterInfo.SetName("mycluster")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	c := &Cluster{context: &clusterd.Context{Clientset: clientset}, clusterInfo: clusterInfo}
	osdInfo := OSDInfo{ID: 23}
	pvcName := "test-pvc"

	// fail to get the host name when there is no pod or deployment
	name, err := c.getPVCHostName(pvcName)
	assert.Error(t, err)
	assert.Equal(t, "", name)

	// Create a sample osd deployment
	osdDeployment := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "osd-23",
			Namespace: c.clusterInfo.Namespace,
			Labels:    c.getOSDLabels(osdInfo, "", true),
		},
	}
	k8sutil.AddLabelToDeployment(OSDOverPVCLabelKey, pvcName, osdDeployment)
	osdDeployment.Spec.Template.Spec.NodeSelector = map[string]string{corev1.LabelHostname: "testnode"}

	_, err = clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Create(ctx, osdDeployment, metav1.CreateOptions{})
	assert.NoError(t, err)

	// get the host name based on the deployment
	name, err = c.getPVCHostName(pvcName)
	assert.NoError(t, err)
	assert.Equal(t, "testnode", name)

	// delete the deployment and get the host name based on the pod
	err = clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Delete(ctx, osdDeployment.Name, metav1.DeleteOptions{})
	assert.NoError(t, err)
	osdPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "osd-23",
			Namespace: c.clusterInfo.Namespace,
			Labels:    c.getOSDLabels(osdInfo, "", true),
		},
	}
	osdPod.Labels = map[string]string{OSDOverPVCLabelKey: pvcName}
	osdPod.Spec.NodeName = "testnode"
	_, err = clientset.CoreV1().Pods(c.clusterInfo.Namespace).Create(ctx, osdPod, metav1.CreateOptions{})
	assert.NoError(t, err)

	name, err = c.getPVCHostName(pvcName)
	assert.NoError(t, err)
	assert.Equal(t, "testnode", name)
}

func TestGetOSDInfo(t *testing.T) {
	clusterInfo := &cephclient.ClusterInfo{Namespace: "ns"}
	clusterInfo.SetName("test")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	context := &clusterd.Context{}
	spec := cephv1.ClusterSpec{DataDirHostPath: "/rook"}
	c := New(context, clusterInfo, spec, "myversion")

	node := "n1"
	location := "root=default host=myhost zone=myzone"
	osd1 := &OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "dev/logical-volume-path", CVMode: "raw", Location: location, TopologyAffinity: "topology.rook.io/rack=rack0"}
	osd2 := &OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "vg1/lv1", CVMode: "lvm", LVBackedPV: true}
	osd3 := &OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "", CVMode: "raw"}
	osdProp := osdProperties{
		crushHostname: node,
		pvc:           corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc"},
		selection:     cephv1.Selection{},
		resources:     corev1.ResourceRequirements{},
		storeConfig:   config.StoreConfig{},
		portable:      true,
	}
	dataPathMap := &provisionConfig{
		DataPathMap: opconfig.NewDatalessDaemonDataPathMap(c.clusterInfo.Namespace, c.spec.DataDirHostPath),
	}

	t.Run("version labels are on deployment and not pod spec", func(t *testing.T) {
		d1, _ := c.makeDeployment(osdProp, osd1, dataPathMap)
		assert.NotEqual(t, "", d1.Labels["rook-version"])
		assert.Equal(t, "", d1.Spec.Template.Labels["rook-version"])
		assert.NotEqual(t, "", d1.Labels["ceph-version"])
		assert.Equal(t, "", d1.Spec.Template.Labels["ceph-version"])
	})

	t.Run("get info from PVC-based OSDs", func(t *testing.T) {
		d1, _ := c.makeDeployment(osdProp, osd1, dataPathMap)
		osdInfo1, _ := c.getOSDInfo(d1)
		assert.Equal(t, osd1.ID, osdInfo1.ID)
		assert.Equal(t, osd1.BlockPath, osdInfo1.BlockPath)
		assert.Equal(t, osd1.CVMode, osdInfo1.CVMode)
		assert.Equal(t, location, osdInfo1.Location)
		assert.Equal(t, osd1.TopologyAffinity, osdInfo1.TopologyAffinity)
		osdProp.portable = false

		d2, _ := c.makeDeployment(osdProp, osd2, dataPathMap)
		osdInfo2, _ := c.getOSDInfo(d2)
		assert.Equal(t, osd2.ID, osdInfo2.ID)
		assert.Equal(t, osd2.BlockPath, osdInfo2.BlockPath)
		assert.Equal(t, osd2.CVMode, osdInfo2.CVMode)
		assert.Equal(t, osd2.LVBackedPV, osdInfo2.LVBackedPV)

		// make deployment fails if block path is empty. allow it to create a valid deployment, then
		// set the deployment to have bad info
		d3, err := c.makeDeployment(osdProp, osd3, dataPathMap)
		assert.NoError(t, err)
		container, err := findOSDContainer(d3.Spec.Template.Spec.Containers)
		assert.NoError(t, err)
		container.Env = append(container.Env, corev1.EnvVar{Name: blockPathVarName, Value: ""})
		_, err = c.getOSDInfo(d3)
		assert.Error(t, err)
	})

	t.Run("get info from node-based OSDs", func(t *testing.T) {
		useAllDevices := true
		osd4 := &OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "", CVMode: "lvm", Location: location}
		osd5 := &OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "vg1/lv1", CVMode: "lvm"}
		osdProp = osdProperties{
			crushHostname: node,
			devices:       []cephv1.Device{},
			pvc:           corev1.PersistentVolumeClaimVolumeSource{},
			selection: cephv1.Selection{
				UseAllDevices: &useAllDevices,
			},
			resources:      corev1.ResourceRequirements{},
			storeConfig:    config.StoreConfig{},
			metadataDevice: "",
		}

		d4, _ := c.makeDeployment(osdProp, osd4, dataPathMap)
		osdInfo4, _ := c.getOSDInfo(d4)
		assert.Equal(t, osd4.ID, osdInfo4.ID)
		assert.Equal(t, location, osdInfo4.Location)

		d5, _ := c.makeDeployment(osdProp, osd5, dataPathMap)
		osdInfo5, _ := c.getOSDInfo(d5)
		assert.Equal(t, osd5.ID, osdInfo5.ID)
		assert.Equal(t, osd5.CVMode, osdInfo5.CVMode)
	})
}

func TestGetPreparePlacement(t *testing.T) {
	// no placement
	prop := osdProperties{}
	result := prop.getPreparePlacement()
	assert.Nil(t, result.NodeAffinity)

	// the osd daemon placement is specified
	prop.placement = cephv1.Placement{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "label1",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"bar", "baz"},
							},
						},
					},
				},
			},
		},
	}

	result = prop.getPreparePlacement()
	assert.Equal(t, 1, len(result.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms))
	assert.Equal(t, "label1", result.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key)

	// The prepare placement is specified and takes precedence over the osd placement
	prop.preparePlacement = &cephv1.Placement{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "label2",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"foo", "bar"},
							},
						},
					},
				},
			},
		},
	}
	result = prop.getPreparePlacement()
	assert.Equal(t, 1, len(result.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms))
	assert.Equal(t, "label2", result.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key)
}

func TestDetectCrushLocation(t *testing.T) {
	location := []string{"host=foo"}
	nodeLabels := map[string]string{}

	// no change to the location if there are no labels
	updateLocationWithNodeLabels(&location, nodeLabels)
	assert.Equal(t, 1, len(location))
	assert.Equal(t, "host=foo", location[0])

	// no change to the location if an invalid label or invalid topology
	nodeLabels = map[string]string{
		"topology.rook.io/foo":          "bar",
		"invalid.topology.rook.io/rack": "r1",
		"topology.rook.io/zone":         "z1",
	}
	updateLocationWithNodeLabels(&location, nodeLabels)
	assert.Equal(t, 1, len(location))
	assert.Equal(t, "host=foo", location[0])

	// update the location with valid topology labels
	nodeLabels = map[string]string{
		"topology.kubernetes.io/region": "region1",
		"topology.kubernetes.io/zone":   "zone",
		"topology.rook.io/rack":         "rack1",
		"topology.rook.io/row":          "row1",
	}
	// sorted in alphabetical order
	expected := []string{
		"host=foo",
		"rack=rack1",
		"region=region1",
		"row=row1",
		"zone=zone",
	}
	updateLocationWithNodeLabels(&location, nodeLabels)

	assert.Equal(t, 5, len(location))
	for i, locString := range location {
		assert.Equal(t, locString, expected[i])
	}
}

func TestGetOSDInfoWithCustomRoot(t *testing.T) {
	clusterInfo := &cephclient.ClusterInfo{Namespace: "ns"}
	clusterInfo.SetName("test")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	context := &clusterd.Context{}
	spec := cephv1.ClusterSpec{
		DataDirHostPath: "/rook",
		Storage: cephv1.StorageScopeSpec{
			Config: map[string]string{
				"crushRoot": "custom-root",
			},
		},
	}
	c := New(context, clusterInfo, spec, "myversion")

	node := "n1"
	location := "root=custom-root host=myhost zone=myzone"
	osd1 := &OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "dev/logical-volume-path", CVMode: "raw", Location: location}
	osd2 := &OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "vg1/lv1", CVMode: "lvm", LVBackedPV: true, Location: location}
	osd3 := &OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "", CVMode: "lvm", Location: location}
	osdProp := osdProperties{
		crushHostname: node,
		pvc:           corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc"},
		selection:     cephv1.Selection{},
		resources:     corev1.ResourceRequirements{},
		storeConfig:   config.StoreConfig{},
	}
	dataPathMap := &provisionConfig{
		DataPathMap: opconfig.NewDatalessDaemonDataPathMap(c.clusterInfo.Namespace, c.spec.DataDirHostPath),
	}
	d1, _ := c.makeDeployment(osdProp, osd1, dataPathMap)
	osdInfo1, _ := c.getOSDInfo(d1)
	assert.Equal(t, osd1.ID, osdInfo1.ID)
	assert.Equal(t, osd1.BlockPath, osdInfo1.BlockPath)
	assert.Equal(t, osd1.CVMode, osdInfo1.CVMode)
	assert.Equal(t, location, osdInfo1.Location)

	d2, _ := c.makeDeployment(osdProp, osd2, dataPathMap)
	osdInfo2, _ := c.getOSDInfo(d2)
	assert.Equal(t, osd2.ID, osdInfo2.ID)
	assert.Equal(t, osd2.BlockPath, osdInfo2.BlockPath)
	assert.Equal(t, osd2.CVMode, osdInfo2.CVMode)
	assert.Equal(t, osd2.LVBackedPV, osdInfo2.LVBackedPV)
	assert.Equal(t, location, osdInfo2.Location)

	d3, _ := c.makeDeployment(osdProp, osd3, dataPathMap)
	_, err := c.getOSDInfo(d3)
	assert.Error(t, err)
}

func TestUpdateCephStorageStatus(t *testing.T) {
	ctx := context.TODO()
	clusterInfo := cephclient.AdminTestClusterInfo("fake")
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("ExecuteCommandWithOutputFile: %s %v", command, args)
			if args[1] == "crush" && args[2] == "class" && args[3] == "ls" {
				// Mock executor for OSD crush class list command, returning ssd as available device class
				return `["ssd"]`, nil
			}
			return "", nil
		},
	}

	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testing",
			Namespace: "fake",
		},
		Spec: cephv1.ClusterSpec{},
	}
	// Objects to track in the fake client.
	object := []runtime.Object{
		cephCluster,
	}
	s := scheme.Scheme
	// Create a fake client to mock API calls.
	client := clientfake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	context := &clusterd.Context{
		Executor:  executor,
		Client:    client,
		Clientset: testexec.New(t, 2),
	}

	// Initializing an OSD monitoring
	c := New(context, clusterInfo, cephCluster.Spec, "myversion")

	t.Run("verify ssd device class added to storage status", func(t *testing.T) {
		err := c.updateCephOsdStorageStatus()
		assert.NoError(t, err)
		err = context.Client.Get(clusterInfo.Context, clusterInfo.NamespacedName(), cephCluster)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cephCluster.Status.CephStorage.DeviceClasses))
		assert.Equal(t, "ssd", cephCluster.Status.CephStorage.DeviceClasses[0].Name)
	})

	t.Run("verify bluestore OSD count in storage status", func(t *testing.T) {
		labels := map[string]string{
			k8sutil.AppAttr:     AppName,
			k8sutil.ClusterAttr: clusterInfo.Namespace,
			OsdIdLabelKey:       "0",
			osdStore:            "bluestore",
		}

		deployment := &apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "osd0",
				Namespace: clusterInfo.Namespace,
				Labels:    labels,
			},
		}
		if _, err := context.Clientset.AppsV1().Deployments(clusterInfo.Namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
			logger.Errorf("Error creating fake deployment: %v", err)
		}
		err := c.updateCephOsdStorageStatus()
		assert.NoError(t, err)
		err = context.Client.Get(clusterInfo.Context, clusterInfo.NamespacedName(), cephCluster)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cephCluster.Status.CephStorage.DeviceClasses))
		assert.Equal(t, "ssd", cephCluster.Status.CephStorage.DeviceClasses[0].Name)
		assert.Equal(t, 1, cephCluster.Status.CephStorage.OSD.StoreType["bluestore"])
		assert.Equal(t, 0, cephCluster.Status.CephStorage.OSD.StoreType["bluestore-rdr"])
	})

	t.Run("verify bluestoreRDR OSD count in storage status", func(t *testing.T) {
		labels := map[string]string{
			k8sutil.AppAttr:     AppName,
			k8sutil.ClusterAttr: clusterInfo.Namespace,
			OsdIdLabelKey:       "1",
			osdStore:            "bluestore-rdr",
		}

		deployment := &apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "osd1",
				Namespace: clusterInfo.Namespace,
				Labels:    labels,
			},
		}
		if _, err := context.Clientset.AppsV1().Deployments(clusterInfo.Namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
			logger.Errorf("Error creating fake deployment: %v", err)
		}
		err := c.updateCephOsdStorageStatus()
		assert.NoError(t, err)
		err = context.Client.Get(clusterInfo.Context, clusterInfo.NamespacedName(), cephCluster)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cephCluster.Status.CephStorage.DeviceClasses))
		assert.Equal(t, "ssd", cephCluster.Status.CephStorage.DeviceClasses[0].Name)
		assert.Equal(t, 1, cephCluster.Status.CephStorage.OSD.StoreType["bluestore-rdr"])
		assert.Equal(t, 1, cephCluster.Status.CephStorage.OSD.StoreType["bluestore"])
	})
}

func Test_updateCephOsdStorageStatus_cephx(t *testing.T) {
	ctx := context.TODO()
	clusterInfo := cephclient.AdminTestClusterInfo("fake")
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("ExecuteCommandWithOutputFile: %s %v", command, args)
			if args[1] == "crush" && args[2] == "class" && args[3] == "ls" {
				// Mock executor for OSD crush class list command, returning ssd as available device class
				return `["ssd"]`, nil
			}
			return "", nil
		},
	}

	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testing",
			Namespace: "fake",
		},
		Spec: cephv1.ClusterSpec{},
	}
	// Objects to track in the fake client.
	object := []runtime.Object{
		cephCluster,
	}
	s := scheme.Scheme
	// Create a fake client to mock API calls.
	client := clientfake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	context := &clusterd.Context{
		Executor:  executor,
		Client:    client,
		Clientset: testexec.New(t, 2),
	}

	// Initializing an OSD monitoring
	c := New(context, clusterInfo, cephCluster.Spec, "myversion")

	// Tests are not independent and rely on prior tests's changes

	t.Run("no osd deployments, no cephcluster status", func(t *testing.T) {
		err := c.updateCephOsdStorageStatus()
		assert.NoError(t, err)
		err = context.Client.Get(clusterInfo.Context, clusterInfo.NamespacedName(), cephCluster)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.ClusterCephxStatus{}, cephCluster.Status.Cephx)
	})

	t.Run("unset cephx status, no cephcluster status", func(t *testing.T) {
		deployment := &apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "osd0",
				Namespace: clusterInfo.Namespace,
				Labels: map[string]string{
					k8sutil.AppAttr:     AppName,
					k8sutil.ClusterAttr: clusterInfo.Namespace,
					OsdIdLabelKey:       "0",
				},
			},
			Spec: apps.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
				},
			},
		}
		_, err := context.Clientset.AppsV1().Deployments(clusterInfo.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
		assert.NoError(t, err)

		err = c.updateCephOsdStorageStatus()
		assert.NoError(t, err)
		err = context.Client.Get(clusterInfo.Context, clusterInfo.NamespacedName(), cephCluster)
		assert.NoError(t, err)
		assert.NotNil(t, cephCluster.Status.Cephx)
		assert.Empty(t, cephCluster.Status.Cephx.OSD)
	})

	t.Run("empty string cephx status, no cephcluster status", func(t *testing.T) {
		deployment := &apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "osd0",
				Namespace: clusterInfo.Namespace,
				Labels: map[string]string{
					k8sutil.AppAttr:     AppName,
					k8sutil.ClusterAttr: clusterInfo.Namespace,
					OsdIdLabelKey:       "0",
				},
			},
			Spec: apps.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							cephxStatusAnnotationKey: ``,
						},
					},
				},
			},
		}
		_, err := context.Clientset.AppsV1().Deployments(clusterInfo.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
		assert.NoError(t, err)

		err = c.updateCephOsdStorageStatus()
		assert.NoError(t, err)
		err = context.Client.Get(clusterInfo.Context, clusterInfo.NamespacedName(), cephCluster)
		assert.NoError(t, err)
		assert.NotNil(t, cephCluster.Status.Cephx)
		assert.Empty(t, cephCluster.Status.Cephx.OSD)
	})

	t.Run("gen 1 cephx status", func(t *testing.T) {
		deployment := &apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "osd0",
				Namespace: clusterInfo.Namespace,
				Labels: map[string]string{
					k8sutil.AppAttr:     AppName,
					k8sutil.ClusterAttr: clusterInfo.Namespace,
					OsdIdLabelKey:       "0",
				},
			},
			Spec: apps.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							cephxStatusAnnotationKey: `{"keyGeneration": 1, "keyCephVersion": "v20"}`,
						},
					},
				},
			},
		}
		_, err := context.Clientset.AppsV1().Deployments(clusterInfo.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
		assert.NoError(t, err)

		err = c.updateCephOsdStorageStatus()
		assert.NoError(t, err)
		err = context.Client.Get(clusterInfo.Context, clusterInfo.NamespacedName(), cephCluster)
		assert.NoError(t, err)
		assert.NotNil(t, cephCluster.Status.Cephx)
		assert.Equal(t, cephv1.CephxStatus{KeyGeneration: 1, KeyCephVersion: "v20"}, cephCluster.Status.Cephx.OSD)
	})

	t.Run("gen 1 and unset cephx status", func(t *testing.T) {
		deployment := &apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "osd1",
				Namespace: clusterInfo.Namespace,
				Labels: map[string]string{
					k8sutil.AppAttr:     AppName,
					k8sutil.ClusterAttr: clusterInfo.Namespace,
					OsdIdLabelKey:       "1",
				},
			},
			Spec: apps.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
				},
			},
		}
		_, err := context.Clientset.AppsV1().Deployments(clusterInfo.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
		assert.NoError(t, err)

		err = c.updateCephOsdStorageStatus()
		assert.NoError(t, err)
		err = context.Client.Get(clusterInfo.Context, clusterInfo.NamespacedName(), cephCluster)
		assert.NoError(t, err)
		assert.NotNil(t, cephCluster.Status.Cephx)
		assert.Equal(t, cephv1.CephxStatus{KeyGeneration: 0, KeyCephVersion: ""}, cephCluster.Status.Cephx.OSD)
	})

	t.Run("gen 1 and gen 2 cephx status", func(t *testing.T) {
		deployment := &apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "osd1",
				Namespace: clusterInfo.Namespace,
				Labels: map[string]string{
					k8sutil.AppAttr:     AppName,
					k8sutil.ClusterAttr: clusterInfo.Namespace,
					OsdIdLabelKey:       "1",
				},
			},
			Spec: apps.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							cephxStatusAnnotationKey: `{"keyGeneration": 2, "keyCephVersion": "v19"}`,
						},
					},
				},
			},
		}
		_, err := context.Clientset.AppsV1().Deployments(clusterInfo.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
		assert.NoError(t, err)

		err = c.updateCephOsdStorageStatus()
		assert.NoError(t, err)
		err = context.Client.Get(clusterInfo.Context, clusterInfo.NamespacedName(), cephCluster)
		assert.NoError(t, err)
		assert.NotNil(t, cephCluster.Status.Cephx)
		assert.Equal(t, cephv1.CephxStatus{KeyGeneration: 1, KeyCephVersion: "v20"}, cephCluster.Status.Cephx.OSD)
	})
}

func TestGetOSDLocationFromArgs(t *testing.T) {
	args := []string{"--id", "2", "--crush-location=root=default host=minikube"}
	osdLocaiton, locationFound := getOSDLocationFromArgs(args)
	assert.Equal(t, osdLocaiton, "root=default host=minikube")
	assert.Equal(t, locationFound, true)

	args = []string{"--id", "2"}
	osdLocaiton, locationFound = getOSDLocationFromArgs(args)
	assert.Equal(t, osdLocaiton, "")
	assert.Equal(t, locationFound, false)
}

func TestValidateOSDSettings(t *testing.T) {
	namespace := "ns"
	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   namespace,
		CephVersion: cephver.Squid,
		Context:     context.TODO(),
	}
	clusterInfo.SetName("rook-ceph-test")
	c := New(&clusterd.Context{}, clusterInfo, cephv1.ClusterSpec{}, "version")

	t.Run("validate with no settings", func(t *testing.T) {
		assert.NoError(t, c.validateOSDSettings())
	})

	t.Run("valid device sets", func(t *testing.T) {
		c.spec.Storage.StorageClassDeviceSets = []cephv1.StorageClassDeviceSet{
			{Name: "set1"},
			{Name: "set2"},
			{Name: "set3"},
		}
		assert.NoError(t, c.validateOSDSettings())
	})

	t.Run("duplicate device sets", func(t *testing.T) {
		c.spec.Storage.StorageClassDeviceSets = []cephv1.StorageClassDeviceSet{
			{Name: "set1"},
			{Name: "set2"},
			{Name: "set1"},
		}
		assert.Error(t, c.validateOSDSettings())
	})
}
