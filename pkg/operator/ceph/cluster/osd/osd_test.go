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
	"os"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	fakeclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephclientfake "github.com/rook/rook/pkg/daemon/ceph/client/fake"
	discoverDaemon "github.com/rook/rook/pkg/daemon/discover"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestOSDProperties(t *testing.T) {
	osdProps := []osdProperties{
		{pvc: corev1.PersistentVolumeClaimVolumeSource{ClaimName: "claim"},
			metadataPVC: corev1.PersistentVolumeClaimVolumeSource{ClaimName: "claim"}},
		{pvc: corev1.PersistentVolumeClaimVolumeSource{ClaimName: ""},
			metadataPVC: corev1.PersistentVolumeClaimVolumeSource{ClaimName: ""}},
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
	clientset := fake.NewSimpleClientset()
	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   "ns",
		CephVersion: cephver.Octopus,
		Context:     context.TODO(),
	}
	context := &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}
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
	updateConditionFunc = func(ctx context.Context, c *clusterd.Context, namespaceName types.NamespacedName, conditionType cephv1.ConditionType, status corev1.ConditionStatus, reason cephv1.ConditionReason, message string) {
		// do nothing
	}

	// set up a fake k8s client set and watcher to generate events that the operator will listen to
	clientset := fake.NewSimpleClientset()
	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	test.AddReadyNode(t, clientset, nodeName, "23.23.23.23")
	cmErr := createDiscoverConfigmap(nodeName, "rook-system", clientset)
	assert.Nil(t, cmErr)

	statusMapWatcher := watch.NewFake()
	clientset.PrependWatchReactor("configmaps", k8stesting.DefaultWatchReactor(statusMapWatcher, nil))

	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   namespace,
		CephVersion: cephver.Octopus,
		Context:     ctx,
	}
	clusterInfo.SetName("rook-ceph-test")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	generateKey := "expected key"
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			return "{\"key\": \"" + generateKey + "\"}", nil
		},
	}

	context := &clusterd.Context{
		Clientset:     clientset,
		ConfigDir:     "/var/lib/rook",
		Executor:      executor,
		RookClientset: fakeclient.NewSimpleClientset(),
	}
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
		Labels: map[string]string{k8sutil.AppAttr: AppName}}}
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
					return cephclientfake.OsdOkToStopOutput(1, []int{1}, true), nil
				}
			}
			if args[0] == "df" && args[1] == "detail" {
				return `{"stats":{"total_bytes":0,"total_used_bytes":0,"total_avail_bytes":3072}}`, nil
			}
			if args[0] == "pg" && args[1] == "dump" {
				return `{}`, nil
			}
			if args[0] == "auth" && args[1] == "del" {
				assert.Equal(t, "osd.1", args[2])
				return "", nil
			}
			return "", errors.Errorf("unexpected ceph command %q", args)
		},
	}

	// modify the storage spec to remove the node from the cluster
	spec.Storage.Nodes = []cephv1.Node{}
	c = New(context, clusterInfo, spec, "myversion")

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

func TestAddNodeFailure(t *testing.T) {
	// create a storage spec with the given nodes/devices/dirs
	nodeName := "node1672"

	// create a fake clientset that will return an error when the operator tries to create a job
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("create", "jobs", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("mock failed to create jobs")
	})
	nodeErr := createNode(nodeName, corev1.NodeReady, clientset)
	assert.Nil(t, nodeErr)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	cmErr := createDiscoverConfigmap(nodeName, "rook-system", clientset)
	assert.Nil(t, cmErr)

	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   "ns-add-remove",
		CephVersion: cephver.Octopus,
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

func TestGetPVCHostName(t *testing.T) {
	ctx := context.TODO()
	clientset := fake.NewSimpleClientset()
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
	osd1 := OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "dev/logical-volume-path", CVMode: "raw", Location: location, TopologyAffinity: "topology.rook.io/rack=rack0"}
	osd2 := OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "vg1/lv1", CVMode: "lvm", LVBackedPV: true}
	osd3 := OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "", CVMode: "raw"}
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
		d3.Spec.Template.Spec.Containers[0].Env = append(d3.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{Name: blockPathVarName, Value: ""})
		_, err = c.getOSDInfo(d3)
		assert.Error(t, err)
	})

	t.Run("get info from node-based OSDs", func(t *testing.T) {
		useAllDevices := true
		osd4 := OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "", CVMode: "lvm", Location: location}
		osd5 := OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "vg1/lv1", CVMode: "lvm"}
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
	prop.placement = cephv1.Placement{NodeAffinity: &corev1.NodeAffinity{
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
	prop.preparePlacement = &cephv1.Placement{NodeAffinity: &corev1.NodeAffinity{
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
		"failure-domain.beta.kubernetes.io/region": "region1",
		"failure-domain.beta.kubernetes.io/zone":   "zone1",
		"topology.rook.io/rack":                    "rack1",
		"topology.rook.io/row":                     "row1",
	}

	expected := []string{
		"host=foo",
		"rack=rack1",
		"region=region1",
		"row=row1",
		"zone=zone1",
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
	osd1 := OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "dev/logical-volume-path", CVMode: "raw", Location: location}
	osd2 := OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "vg1/lv1", CVMode: "lvm", LVBackedPV: true, Location: location}
	osd3 := OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "", CVMode: "lvm", Location: location}
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
