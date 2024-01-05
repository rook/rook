/*
Copyright 2021 The Rook Authors. All rights reserved.

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
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func Test_createNewOSDsFromStatus(t *testing.T) {
	namespace := "my-namespace"

	clientset := fake.NewSimpleClientset()
	// clusterd.Context is created in doSetup()

	// names of status configmaps for nodes
	statusNameNode0 := statusConfigMapName("node0")
	statusNameNode2 := statusConfigMapName("node2")
	statusNamePVC1 := statusConfigMapName("pvc1")
	statusNamePVC2 := statusConfigMapName("pvc2")

	clusterInfo := &cephclient.ClusterInfo{
		Namespace: namespace,
	}
	clusterInfo.SetName("mycluster")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)

	var oldCreateDaemonOnNodeFunc = createDaemonOnNodeFunc
	defer func() {
		createDaemonOnNodeFunc = oldCreateDaemonOnNodeFunc
	}()
	createCallsOnNode := []int{}
	induceFailureCreatingOSD := -1 // allow causing the create call to fail for a given OSD ID
	createDaemonOnNodeFunc = func(c *Cluster, osd OSDInfo, nodeName string, config *provisionConfig) error {
		createCallsOnNode = append(createCallsOnNode, osd.ID)
		if induceFailureCreatingOSD == osd.ID {
			return errors.Errorf("createOSDDaemonOnNode: induced failure on OSD %d", osd.ID)
		}
		return nil
	}

	var oldCreateDaemonOnPVCFunc = createDaemonOnPVCFunc
	defer func() {
		createDaemonOnPVCFunc = oldCreateDaemonOnPVCFunc
	}()
	createCallsOnPVC := []int{}
	// reuse induceFailureCreatingOSD from above
	createDaemonOnPVCFunc = func(c *Cluster, osd OSDInfo, pvcName string, config *provisionConfig) error {
		createCallsOnPVC = append(createCallsOnPVC, osd.ID)
		if induceFailureCreatingOSD == osd.ID {
			return errors.Errorf("createOSDDaemonOnNode: induced failure on OSD %d", osd.ID)
		}
		return nil
	}

	// Simulate an environment where deployments exist for OSDs 3, 4, and 6
	deployments := newExistenceListWithCapacity(5)
	deployments.Add(3)
	deployments.Add(4)
	deployments.Add(6)

	spec := cephv1.ClusterSpec{}
	var status *OrchestrationStatus
	awaitingStatusConfigMaps := sets.New[string]()

	var c *Cluster
	var createConfig *createConfig
	var errs *provisionErrors
	doSetup := func() {
		// none of this code should ever add or remove deployments from the existence list
		assert.Equal(t, 3, deployments.Len())
		// Simulate environment where provision jobs were created for node0, node2, pvc1, and pvc2
		awaitingStatusConfigMaps = sets.New[string]()
		awaitingStatusConfigMaps.Insert(
			statusNameNode0, statusNameNode2,
			statusNamePVC1, statusNamePVC2)
		createCallsOnNode = createCallsOnNode[:0]
		createCallsOnPVC = createCallsOnPVC[:0]
		errs = newProvisionErrors()
		ctx := &clusterd.Context{
			Clientset: clientset,
		}
		c = New(ctx, clusterInfo, spec, "rook/rook:master")
		config := c.newProvisionConfig()
		createConfig = c.newCreateConfig(config, awaitingStatusConfigMaps, deployments)
	}

	t.Run("node: create no OSDs when none are returned from node", func(t *testing.T) {
		doSetup()
		status = &OrchestrationStatus{
			OSDs:         []OSDInfo{},
			PvcBackedOSD: false,
		}
		createConfig.createNewOSDsFromStatus(status, "node0", errs)
		assert.Zero(t, errs.len())
		assert.Len(t, createCallsOnNode, 0)
		assert.Len(t, createCallsOnPVC, 0)
		// status map should have been marked completed
		assert.Equal(t, 4, awaitingStatusConfigMaps.Len())
		assert.Equal(t, 1, createConfig.finishedStatusConfigMaps.Len())
		assert.True(t, createConfig.finishedStatusConfigMaps.Has(statusNameNode0))
	})

	t.Run("test: node: create all OSDs on node when all do not exist", func(t *testing.T) {
		doSetup()
		status = &OrchestrationStatus{
			OSDs: []OSDInfo{
				{ID: 0}, {ID: 1}, {ID: 2},
			},
			PvcBackedOSD: false,
		}
		createConfig.createNewOSDsFromStatus(status, "node2", errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, createCallsOnNode, []int{0, 1, 2})
		assert.Len(t, createCallsOnPVC, 0)
		// status map should have been marked completed
		assert.Equal(t, 4, awaitingStatusConfigMaps.Len())
		assert.Equal(t, 1, createConfig.finishedStatusConfigMaps.Len())
		assert.True(t, createConfig.finishedStatusConfigMaps.Has(statusNameNode2))
	})

	t.Run("node: create only nonexistent OSDs on node when some already exist", func(t *testing.T) {
		doSetup()
		status = &OrchestrationStatus{
			OSDs: []OSDInfo{
				{ID: 3}, {ID: 4}, // already exist
				{ID: 5}, // does not exist
				{ID: 6}, // already exists
				{ID: 7}, // does not exist
			},
			PvcBackedOSD: false,
		}
		createConfig.createNewOSDsFromStatus(status, "node0", errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, createCallsOnNode, []int{5, 7})
		assert.Len(t, createCallsOnPVC, 0)
		// status map should have been marked completed
		assert.Equal(t, 4, awaitingStatusConfigMaps.Len())
		assert.Equal(t, 1, createConfig.finishedStatusConfigMaps.Len())
		assert.True(t, createConfig.finishedStatusConfigMaps.Has(statusNameNode0))
	})

	t.Run("node: skip creating OSDs for status configmaps that weren't created for this reconcile", func(t *testing.T) {
		doSetup()
		status = &OrchestrationStatus{
			OSDs: []OSDInfo{
				{ID: 0}, {ID: 1}, {ID: 2},
			},
			PvcBackedOSD: false,
		}
		createConfig.createNewOSDsFromStatus(status, "node1", errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, createCallsOnNode, []int{})
		assert.Len(t, createCallsOnPVC, 0)
		// status map should NOT have been marked completed
		assert.Equal(t, 4, awaitingStatusConfigMaps.Len())
		assert.Equal(t, 0, createConfig.finishedStatusConfigMaps.Len())
	})

	t.Run("node: errors reported if OSDs fail to create", func(t *testing.T) {
		induceFailureCreatingOSD = 1 // fail when creating OSD 1
		doSetup()
		status = &OrchestrationStatus{
			OSDs: []OSDInfo{
				{ID: 0}, {ID: 1}, {ID: 2},
			},
			PvcBackedOSD: false,
		}
		createConfig.createNewOSDsFromStatus(status, "node0", errs)
		assert.Equal(t, 1, errs.len())
		assert.ElementsMatch(t, createCallsOnNode, []int{0, 1, 2})
		assert.Len(t, createCallsOnPVC, 0)
		// status map should have been marked completed
		assert.Equal(t, 4, awaitingStatusConfigMaps.Len())
		assert.Equal(t, 1, createConfig.finishedStatusConfigMaps.Len())
		assert.True(t, createConfig.finishedStatusConfigMaps.Has(statusNameNode0))
		induceFailureCreatingOSD = -1 // off
	})

	t.Run("pvc: create no OSDs when none are returned from PVC", func(t *testing.T) {
		doSetup()
		status = &OrchestrationStatus{
			OSDs:         []OSDInfo{},
			PvcBackedOSD: true,
		}
		createConfig.createNewOSDsFromStatus(status, "pvc1", errs)
		assert.Zero(t, errs.len())
		assert.Len(t, createCallsOnNode, 0)
		assert.Len(t, createCallsOnPVC, 0)
		// status map should have been marked completed
		assert.Equal(t, 4, awaitingStatusConfigMaps.Len())
		assert.Equal(t, 1, createConfig.finishedStatusConfigMaps.Len())
		assert.True(t, createConfig.finishedStatusConfigMaps.Has(statusNamePVC1))
	})

	t.Run("pvc: create all OSDs on pvc when all do not exist", func(t *testing.T) {
		doSetup()
		status = &OrchestrationStatus{
			OSDs: []OSDInfo{
				{ID: 0}, {ID: 1}, {ID: 2},
			},
			PvcBackedOSD: true,
		}
		createConfig.createNewOSDsFromStatus(status, "pvc2", errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, createCallsOnPVC, []int{0, 1, 2})
		assert.Len(t, createCallsOnNode, 0)
		// status map should have been marked completed
		assert.Equal(t, 4, awaitingStatusConfigMaps.Len())
		assert.Equal(t, 1, createConfig.finishedStatusConfigMaps.Len())
		assert.True(t, createConfig.finishedStatusConfigMaps.Has(statusNamePVC2))
	})

	t.Run("pvc: create only nonexistent OSDs on pvc when some already exist", func(t *testing.T) {
		doSetup()
		status = &OrchestrationStatus{
			OSDs: []OSDInfo{
				{ID: 3}, {ID: 4}, // already exist
				{ID: 5}, // does not exist
				{ID: 6}, // already exists
				{ID: 7}, // does not exist
			},
			PvcBackedOSD: true,
		}
		createConfig.createNewOSDsFromStatus(status, "pvc1", errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, createCallsOnPVC, []int{5, 7})
		assert.Len(t, createCallsOnNode, 0)
		// status map should have been marked completed
		assert.Equal(t, 4, awaitingStatusConfigMaps.Len())
		assert.Equal(t, 1, createConfig.finishedStatusConfigMaps.Len())
		assert.True(t, createConfig.finishedStatusConfigMaps.Has(statusNamePVC1))
	})

	t.Run("pvc: skip creating OSDs for status configmaps that weren't created for this reconcile", func(t *testing.T) {
		doSetup()
		status = &OrchestrationStatus{
			OSDs: []OSDInfo{
				{ID: 0}, {ID: 1}, {ID: 2},
			},
			PvcBackedOSD: true,
		}
		createConfig.createNewOSDsFromStatus(status, "pvc0", errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, createCallsOnPVC, []int{})
		assert.Len(t, createCallsOnNode, 0)
		// no status maps should have been marked completed
		assert.Equal(t, 4, awaitingStatusConfigMaps.Len())
		assert.Equal(t, 0, createConfig.finishedStatusConfigMaps.Len())
	})

	t.Run("pvc: errors reported if OSDs fail to create", func(t *testing.T) {
		induceFailureCreatingOSD = 1 // fail when creating OSD 1
		doSetup()
		status = &OrchestrationStatus{
			OSDs: []OSDInfo{
				{ID: 0}, {ID: 1}, {ID: 2},
			},
			PvcBackedOSD: true,
		}
		createConfig.createNewOSDsFromStatus(status, "pvc1", errs)
		assert.Equal(t, 1, errs.len())
		assert.ElementsMatch(t, createCallsOnPVC, []int{0, 1, 2})
		assert.Len(t, createCallsOnNode, 0)
		// status map should have been marked completed
		assert.Equal(t, 4, awaitingStatusConfigMaps.Len())
		assert.Equal(t, 1, createConfig.finishedStatusConfigMaps.Len())
		assert.True(t, createConfig.finishedStatusConfigMaps.Has(statusNamePVC1))
		induceFailureCreatingOSD = -1 // off
	})
}

func Test_startProvisioningOverPVCs(t *testing.T) {
	namespace := "rook-ceph"

	clientset := test.NewComplexClientset(t) // fake clientset with generate name functionality

	// clusterd.Context is created in doSetup()

	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   namespace,
		CephVersion: cephver.Quincy,
	}
	clusterInfo.SetName("mycluster")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	clusterInfo.Context = context.TODO()

	spec := cephv1.ClusterSpec{}

	var errs *provisionErrors
	var c *Cluster
	var config *provisionConfig
	var awaitingStatusConfigMaps sets.Set[string]
	var err error
	doSetup := func() {
		errs = newProvisionErrors()
		ctx := &clusterd.Context{
			Clientset: clientset,
		}
		c = New(ctx, clusterInfo, spec, "rook/rook:master")
		config = c.newProvisionConfig()
	}

	t.Run("do nothing if no storage spec is given", func(t *testing.T) {
		spec = cephv1.ClusterSpec{}
		doSetup()
		awaitingStatusConfigMaps, err = c.startProvisioningOverPVCs(config, errs)
		assert.NoError(t, err)
		assert.Zero(t, awaitingStatusConfigMaps.Len())
		assert.Zero(t, errs.len())
		// no result configmaps should have been created
		cms, err := clientset.CoreV1().ConfigMaps(namespace).List(context.TODO(), metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Len(t, cms.Items, 0)
	})

	t.Run("do nothing if device set count is zero", func(t *testing.T) {
		spec = cephv1.ClusterSpec{
			Storage: cephv1.StorageScopeSpec{
				StorageClassDeviceSets: []cephv1.StorageClassDeviceSet{
					{
						Name:  "set1",
						Count: 0,
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							newDummyPVC("data", namespace, "10Gi", "gp2"),
						},
					},
				},
			},
		}
		doSetup()
		awaitingStatusConfigMaps, err = c.startProvisioningOverPVCs(config, errs)
		assert.NoError(t, err)
		assert.Zero(t, awaitingStatusConfigMaps.Len())
		assert.Zero(t, errs.len()) // this was not a problem with a single job but with ALL jobs
		// no result configmaps should have been created
		cms, err := clientset.CoreV1().ConfigMaps(namespace).List(context.TODO(), metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Len(t, cms.Items, 0)
	})

	t.Run("one device set with 2 PVCs", func(t *testing.T) {
		spec = cephv1.ClusterSpec{
			Storage: cephv1.StorageScopeSpec{
				StorageClassDeviceSets: []cephv1.StorageClassDeviceSet{
					{
						Name:  "set1",
						Count: 2,
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							newDummyPVC("data", namespace, "10Gi", "gp2"),
						},
					},
				},
			},
		}
		doSetup()
		awaitingStatusConfigMaps, err = c.startProvisioningOverPVCs(config, errs)
		assert.NoError(t, err)
		assert.Equal(t, 2, awaitingStatusConfigMaps.Len())
		assert.Zero(t, errs.len())
		cms, err := clientset.CoreV1().ConfigMaps(namespace).List(context.TODO(), metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Len(t, cms.Items, 2)
	})

	t.Run("repeat same device set with 2 PVCs (before provisioning jobs are done and before OSD deployments are created)", func(t *testing.T) {
		// spec = <working spec from prior test>
		doSetup()
		awaitingStatusConfigMaps, err = c.startProvisioningOverPVCs(config, errs)
		assert.NoError(t, err)
		assert.Equal(t, 2, awaitingStatusConfigMaps.Len())
		assert.Zero(t, errs.len())
		cms, err := clientset.CoreV1().ConfigMaps(namespace).List(context.TODO(), metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Len(t, cms.Items, 2) // still just 2 configmaps should exist (the same 2 from before)
	})

	t.Run("error if no volume claim template", func(t *testing.T) {
		spec = cephv1.ClusterSpec{
			Storage: cephv1.StorageScopeSpec{
				StorageClassDeviceSets: []cephv1.StorageClassDeviceSet{
					{
						Name:                 "set1",
						Count:                2,
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
					},
				},
			},
		}
		clientset = test.NewComplexClientset(t) // reset to empty fake k8s environment
		doSetup()
		awaitingStatusConfigMaps, err = c.startProvisioningOverPVCs(config, errs)
		assert.NoError(t, err)
		assert.Equal(t, 0, awaitingStatusConfigMaps.Len())
		assert.Equal(t, 1, errs.len())
		cms, err := clientset.CoreV1().ConfigMaps(namespace).List(context.TODO(), metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Len(t, cms.Items, 0)
	})

	// TODO: should we verify the osdProps set on the job?
}

func Test_startProvisioningOverNodes(t *testing.T) {
	namespace := "rook-ceph"
	dataDirHostPath := "/var/lib/mycluster"

	clientset := test.New(t, 3) // fake clientset with 3 nodes
	// clusterd.Context is created in doSetup()

	// names of status configmaps for nodes
	statusNameNode0 := statusConfigMapName("node0")
	statusNameNode1 := statusConfigMapName("node1")
	statusNameNode2 := statusConfigMapName("node2")
	// statusNameNode3 := statusConfigMapName("node3")

	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   namespace,
		CephVersion: cephver.Quincy,
	}
	clusterInfo.SetName("mycluster")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	clusterInfo.Context = context.TODO()

	var useAllDevices bool
	spec := cephv1.ClusterSpec{}

	var errs *provisionErrors
	var c *Cluster
	var config *provisionConfig
	var prepareJobsRun sets.Set[string]
	var err error
	var cms *corev1.ConfigMapList
	doSetup := func() {
		errs = newProvisionErrors()
		ctx := &clusterd.Context{
			Clientset: clientset,
		}
		c = New(ctx, clusterInfo, spec, "rook/rook:master")
		config = c.newProvisionConfig()
	}

	t.Run("do nothing if no storage spec is given", func(t *testing.T) {
		spec = cephv1.ClusterSpec{}
		doSetup()
		prepareJobsRun, err = c.startProvisioningOverNodes(config, errs)
		assert.NoError(t, err)
		assert.Zero(t, prepareJobsRun.Len())
		assert.Zero(t, errs.len())
		// no result configmaps should have been created
		cms, err = clientset.CoreV1().ConfigMaps(namespace).List(context.TODO(), metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Len(t, cms.Items, 0)
	})

	t.Run("error on empty dataDirHostPath", func(t *testing.T) {
		useAllDevices = true
		spec = cephv1.ClusterSpec{
			// this storage spec should cause prepare jobs to run on all (3) nodes
			Storage: cephv1.StorageScopeSpec{
				UseAllNodes: true,
				Selection: cephv1.Selection{
					UseAllDevices: &useAllDevices,
				},
			},
			// BUT empty should not allow any jobs to be created
			DataDirHostPath: "",
		}
		doSetup()
		prepareJobsRun, err = c.startProvisioningOverNodes(config, errs)
		assert.NoError(t, err)
		assert.Zero(t, prepareJobsRun.Len())
		assert.Equal(t, 1, errs.len()) // this was not a problem with a single job but with ALL jobs
		// no result configmaps should have been created
		cms, err = clientset.CoreV1().ConfigMaps(namespace).List(context.TODO(), metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Len(t, cms.Items, 0)
	})

	t.Run("use all nodes and devices", func(t *testing.T) {
		// Setting dataDirHostPath non-empty on the previous config should have jobs run for all nodes
		spec.DataDirHostPath = dataDirHostPath
		doSetup()
		prepareJobsRun, err = c.startProvisioningOverNodes(config, errs)
		assert.NoError(t, err)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t,
			[]string{statusNameNode0, statusNameNode1, statusNameNode2},
			sets.List(prepareJobsRun),
		)
		// all result configmaps should have been created
		cms, err = clientset.CoreV1().ConfigMaps(namespace).List(context.TODO(), metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Len(t, cms.Items, 3)
	})

	t.Run("use all nodes and devices when useAllNodes and individual nodes are both set", func(t *testing.T) {
		// this also tests that jobs that currently exist (created in previous test) are handled
		spec.Storage.Nodes = []cephv1.Node{
			{Name: "node0"}, {Name: "node2"},
		}
		doSetup()
		prepareJobsRun, err = c.startProvisioningOverNodes(config, errs)
		assert.NoError(t, err)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t,
			[]string{statusNameNode0, statusNameNode1, statusNameNode2},
			sets.List(prepareJobsRun),
		)
	})

	t.Run("use individual nodes", func(t *testing.T) {
		// this also tests that existing status configmaps (from the previous tests) don't affect the
		// reported status configmaps from this run
		spec = cephv1.ClusterSpec{
			Storage: cephv1.StorageScopeSpec{
				UseAllNodes: false,
				Nodes: []cephv1.Node{
					// run on only node0 and node2
					{Name: "node0"},
					{Name: "node2"},
				},
				Selection: cephv1.Selection{
					UseAllDevices: &useAllDevices,
				},
			},
			DataDirHostPath: dataDirHostPath,
		}
		doSetup()
		prepareJobsRun, err = c.startProvisioningOverNodes(config, errs)
		assert.NoError(t, err)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t,
			[]string{statusNameNode0, statusNameNode2},
			sets.List(prepareJobsRun),
		)
	})

	t.Run("use no nodes", func(t *testing.T) {
		spec = cephv1.ClusterSpec{
			Storage: cephv1.StorageScopeSpec{
				UseAllNodes: false,
				Nodes:       []cephv1.Node{
					// empty
				},
				Selection: cephv1.Selection{
					UseAllDevices: &useAllDevices,
				},
			},
			DataDirHostPath: dataDirHostPath,
		}
		doSetup()
		prepareJobsRun, err = c.startProvisioningOverNodes(config, errs)
		assert.NoError(t, err)
		assert.Zero(t, errs.len())
		assert.Zero(t, prepareJobsRun.Len())
	})

	t.Run("failures running prepare jobs", func(t *testing.T) {
		spec = cephv1.ClusterSpec{
			Storage: cephv1.StorageScopeSpec{
				UseAllNodes: false,
				Nodes: []cephv1.Node{
					// run on only node0 and node2
					{Name: "node0"},
					{Name: "node2"},
				},
				Selection: cephv1.Selection{
					UseAllDevices: &useAllDevices,
				},
			},
			DataDirHostPath: dataDirHostPath,
		}
		// re-initialize an empty test clientset with 3 nodes
		clientset = test.New(t, 3)
		// add a job reactor that will cause the node2 job to fail
		var jobReactor k8stesting.ReactionFunc = func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
			switch action := action.(type) {
			case k8stesting.CreateActionImpl:
				obj := action.GetObject()
				objMeta, err := meta.Accessor(obj)
				if err != nil {
					panic(err)
				}
				objName := objMeta.GetName()
				if strings.Contains(objName, "node2") {
					return true, nil, errors.Errorf("induced error")
				}
			default:
				panic("this should not happen")
			}
			return false, nil, nil
		}
		clientset.PrependReactor("create", "jobs", jobReactor)
		doSetup()
		prepareJobsRun, err = c.startProvisioningOverNodes(config, errs)
		assert.NoError(t, err)
		assert.Equal(t, 1, errs.len())
		assert.ElementsMatch(t,
			[]string{statusNameNode0},
			sets.List(prepareJobsRun),
		)
		// with a fresh clientset, only the one results ConfigMap should exist
		cms, err = clientset.CoreV1().ConfigMaps(namespace).List(context.TODO(), metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Len(t, cms.Items, 1)
		assert.Equal(t, sets.List(prepareJobsRun)[0], cms.Items[0].Name)
	})
}

func newDummyPVC(name, namespace string, capacity string, storageClassName string) corev1.PersistentVolumeClaim {
	volMode := corev1.PersistentVolumeBlock
	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName(corev1.ResourceStorage): apiresource.MustParse(capacity),
				},
			},
			StorageClassName: &storageClassName,
			VolumeMode:       &volMode,
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
		},
	}
}
