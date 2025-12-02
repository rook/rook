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
	"fmt"
	"strconv"
	"testing"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephclientfake "github.com/rook/rook/pkg/daemon/ceph/client/fake"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func Test_updateExistingOSDs(t *testing.T) {
	namespace := "my-namespace"

	logger.SetLevel(capnslog.DEBUG)

	oldUpdateFunc := updateMultipleDeploymentsAndWaitFunc
	oldNodeFunc := deploymentOnNodeFunc
	oldPVCFunc := deploymentOnPVCFunc
	oldConditionFunc := updateConditionFunc
	oldShouldCheckFunc := shouldCheckOkToStopFunc
	defer func() {
		updateMultipleDeploymentsAndWaitFunc = oldUpdateFunc
		deploymentOnNodeFunc = oldNodeFunc
		deploymentOnPVCFunc = oldPVCFunc
		updateConditionFunc = oldConditionFunc
		shouldCheckOkToStopFunc = oldShouldCheckFunc
	}()

	var executor *exectest.MockExecutor // will be defined later

	// inputs
	var (
		updateQueue         *updateQueue
		existingDeployments *existenceList
		clientset           *fake.Clientset
	)

	// behavior control
	var (
		updateInjectFailures    k8sutil.Failures // return failures from mocked updateDeploymentAndWaitFunc
		returnOkToStopIDs       []int            // return these IDs are ok-to-stop (or not ok to stop if empty)
		forceUpgradeIfUnhealthy bool
		requiresHealthyPGs      bool
		cephStatus              string
	)

	// intermediates (created from inputs)
	var (
		ctx          *clusterd.Context
		c            *Cluster
		updateConfig *updateConfig
	)

	// outputs
	var (
		osdToBeQueried     int      // this OSD ID should be queried
		deploymentsUpdated []string // updateDeploymentAndWaitFunc adds deployments to this list
		osdsOnPVCs         []int    // deploymentOnPVCFunc adds OSD IDs to this list
		osdsOnNodes        []int    // deploymentOnPVCFunc adds OSD IDs to this list
		errs               *provisionErrors
	)

	doSetup := func() {
		// set up intermediates
		ctx = &clusterd.Context{
			Clientset: clientset,
			Executor:  executor,
		}
		clusterInfo := &cephclient.ClusterInfo{
			Namespace:   namespace,
			CephVersion: cephver.Squid,
			Context:     context.TODO(),
		}
		clusterInfo.SetName("mycluster")
		clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
		spec := cephv1.ClusterSpec{
			ContinueUpgradeAfterChecksEvenIfNotHealthy: forceUpgradeIfUnhealthy,
			UpgradeOSDRequiresHealthyPGs:               requiresHealthyPGs,
		}
		c = New(ctx, clusterInfo, spec, "rook/rook:master")
		config := c.newProvisionConfig()
		updateConfig = c.newUpdateConfig(config, updateQueue, existingDeployments, sets.New[string]())

		// prepare outputs
		deploymentsUpdated = []string{}
		osdsOnPVCs = []int{}
		osdsOnNodes = []int{}
		errs = newProvisionErrors()
	}

	// stub out the conditionExportFunc to do nothing. we do not have a fake Rook interface that
	// allows us to interact with a CephCluster resource like the fake K8s clientset.
	updateConditionFunc = func(ctx context.Context, c *clusterd.Context, namespaceName types.NamespacedName, observedGeneration int64, conditionType cephv1.ConditionType, status corev1.ConditionStatus, reason cephv1.ConditionReason, message string) {
		// do nothing
	}
	shouldCheckOkToStopFunc = func(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo) bool {
		// always check. if shouldCheckOkToStop is not implemented correctly, single-node CI tests
		// will fail, which is a more thorough test than we could make in unit tests.
		return true
	}

	updateMultipleDeploymentsAndWaitFunc = func(
		ctx context.Context,
		clientset kubernetes.Interface,
		deployments []*appsv1.Deployment,
		listFunc func() (*appsv1.DeploymentList, error),
	) k8sutil.Failures {
		for _, d := range deployments {
			deploymentsUpdated = append(deploymentsUpdated, d.Name)
		}
		return updateInjectFailures
	}

	executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			t.Logf("command: %s %v", command, args)
			if args[0] == "osd" {
				if args[1] == "ok-to-stop" {
					queriedID := args[2]
					if strconv.Itoa(osdToBeQueried) != queriedID {
						err := errors.Errorf("OSD %d should have been queried, but %s was queried instead", osdToBeQueried, queriedID)
						t.Error(err)
						return "", err
					}
					if len(returnOkToStopIDs) > 0 {
						return cephclientfake.OsdOkToStopOutput(osdToBeQueried, returnOkToStopIDs), nil
					}
					return cephclientfake.OsdOkToStopOutput(osdToBeQueried, []int{}), errors.Errorf("induced error")
				}
				if args[1] == "crush" && args[2] == "get-device-class" {
					return cephclientfake.OSDDeviceClassOutput(args[3]), nil
				}
			}
			if args[0] == "status" {
				return cephStatus, nil
			}
			panic(fmt.Sprintf("unexpected command %q with args %v", command, args))
		},
	}

	// simple wrappers to allow us to count how many OSDs on nodes/PVCs are identified
	deploymentOnNodeFunc = func(c *Cluster, osd *OSDInfo, nodeName string, config *provisionConfig) (*appsv1.Deployment, error) {
		osdsOnNodes = append(osdsOnNodes, osd.ID)
		return deploymentOnNode(c, osd, nodeName, config)
	}
	deploymentOnPVCFunc = func(c *Cluster, osd *OSDInfo, pvcName string, config *provisionConfig) (*appsv1.Deployment, error) {
		osdsOnPVCs = append(osdsOnPVCs, osd.ID)
		return deploymentOnPVC(c, osd, pvcName, config)
	}

	addDeploymentOnNode := func(nodeName string, osdID int) {
		d := getDummyDeploymentOnNode(clientset, c, nodeName, osdID)
		_, err := clientset.AppsV1().Deployments(namespace).Create(context.TODO(), d, metav1.CreateOptions{})
		if err != nil {
			panic(err)
		}
	}

	addDeploymentOnPVC := func(pvcName string, osdID int) {
		d := getDummyDeploymentOnPVC(clientset, c, pvcName, osdID)
		_, err := clientset.AppsV1().Deployments(namespace).Create(context.TODO(), d, metav1.CreateOptions{})
		if err != nil {
			panic(err)
		}
	}

	t.Run("no items in the update queue should be a noop", func(t *testing.T) {
		clientset = fake.NewSimpleClientset()
		updateQueue = newUpdateQueueWithIDs()
		existingDeployments = newExistenceListWithIDs(0, 2, 4, 6)
		forceUpgradeIfUnhealthy = false
		updateInjectFailures = k8sutil.Failures{}
		doSetup()
		addDeploymentOnNode("node0", 0)
		addDeploymentOnPVC("pvc2", 2)
		addDeploymentOnNode("node1", 4)
		addDeploymentOnPVC("pvc6", 6)

		osdToBeQueried = -1 // this will make any OSD query fail
		updateConfig.updateExistingOSDs(errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, deploymentsUpdated, []string{})
	})

	t.Run("ok to stop one OSD at a time", func(t *testing.T) {
		clientset = fake.NewSimpleClientset()
		// reminder that updateQueue pops things off the queue from the front, so the leftmost item
		// will be the one queried
		updateQueue = newUpdateQueueWithIDs(0, 2, 4, 6)
		existingDeployments = newExistenceListWithIDs(0, 2, 4, 6)
		forceUpgradeIfUnhealthy = false
		updateInjectFailures = k8sutil.Failures{}
		doSetup()
		addDeploymentOnNode("node0", 0)
		addDeploymentOnPVC("pvc2", 2)
		addDeploymentOnNode("node1", 4)
		addDeploymentOnPVC("pvc6", 6)

		for _, i := range []int{0, 2, 4, 6} {
			osdToBeQueried = i
			returnOkToStopIDs = []int{i}
			deploymentsUpdated = []string{}
			updateConfig.updateExistingOSDs(errs)
			assert.Zero(t, errs.len())
			assert.ElementsMatch(t, deploymentsUpdated, []string{deploymentName(i)})
		}
		assert.ElementsMatch(t, osdsOnNodes, []int{0, 4})
		assert.ElementsMatch(t, osdsOnPVCs, []int{2, 6})

		assert.Equal(t, 0, updateQueue.Len()) // should be done with updates
	})

	t.Run("ok to stop 3 OSDs at a time", func(t *testing.T) {
		clientset = fake.NewSimpleClientset()
		updateQueue = newUpdateQueueWithIDs(0, 2, 4, 6)
		existingDeployments = newExistenceListWithIDs(0, 2, 4, 6)
		forceUpgradeIfUnhealthy = false
		updateInjectFailures = k8sutil.Failures{}
		doSetup()
		addDeploymentOnNode("node0", 0)
		addDeploymentOnPVC("pvc2", 2)
		addDeploymentOnNode("node1", 4)
		addDeploymentOnPVC("pvc6", 6)

		osdToBeQueried = 0
		returnOkToStopIDs = []int{0, 4, 6}
		updateConfig.updateExistingOSDs(errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, deploymentsUpdated,
			[]string{deploymentName(0), deploymentName(4), deploymentName(6)})

		// should NOT be done with updates
		// this also tests that updateQueue.Len() directly affects doneUpdating()
		assert.Equal(t, 1, updateQueue.Len())
		assert.False(t, updateConfig.doneUpdating())

		deploymentsUpdated = []string{}
		osdToBeQueried = 2
		returnOkToStopIDs = []int{2}
		updateConfig.updateExistingOSDs(errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, deploymentsUpdated,
			[]string{deploymentName(2)})

		// should be done with updates
		// this also tests that updateQueue.Len() directly affects doneUpdating()
		assert.Equal(t, 0, updateQueue.Len())
		assert.True(t, updateConfig.doneUpdating())
	})

	t.Run("ok to stop more OSDs than are in the update queue", func(t *testing.T) {
		clientset = fake.NewSimpleClientset()
		updateQueue = newUpdateQueueWithIDs(2, 0)
		existingDeployments = newExistenceListWithIDs(6, 4, 2, 0)
		forceUpgradeIfUnhealthy = false
		updateInjectFailures = k8sutil.Failures{}
		doSetup()
		addDeploymentOnNode("node0", 0)
		addDeploymentOnPVC("pvc2", 2)
		addDeploymentOnNode("node1", 4)
		addDeploymentOnPVC("pvc6", 6)

		osdToBeQueried = 2
		returnOkToStopIDs = []int{2, 4, 6}
		updateConfig.updateExistingOSDs(errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, deploymentsUpdated, []string{deploymentName(2)})

		deploymentsUpdated = []string{}
		osdToBeQueried = 0
		returnOkToStopIDs = []int{0, 6}
		updateConfig.updateExistingOSDs(errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, deploymentsUpdated, []string{deploymentName(0)})

		assert.Equal(t, 0, updateQueue.Len()) // should be done with updates
	})

	t.Run("ok to stop OSDS not in existence list (newly-created OSDs)", func(t *testing.T) {
		clientset = fake.NewSimpleClientset()
		updateQueue = newUpdateQueueWithIDs(2, 0)
		existingDeployments = newExistenceListWithIDs(2, 0)
		forceUpgradeIfUnhealthy = false
		updateInjectFailures = k8sutil.Failures{}
		doSetup()
		addDeploymentOnNode("node0", 0)
		addDeploymentOnPVC("pvc2", 2)
		addDeploymentOnNode("node1", 4)
		addDeploymentOnPVC("pvc6", 6)

		osdToBeQueried = 2
		returnOkToStopIDs = []int{2, 4, 6}
		updateConfig.updateExistingOSDs(errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, deploymentsUpdated, []string{deploymentName(2)})

		deploymentsUpdated = []string{}
		osdToBeQueried = 0
		returnOkToStopIDs = []int{0, 6}
		updateConfig.updateExistingOSDs(errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, deploymentsUpdated, []string{deploymentName(0)})

		assert.Equal(t, 0, updateQueue.Len()) // should be done with updates
	})

	t.Run("not ok to stop OSD", func(t *testing.T) {
		clientset = fake.NewSimpleClientset()
		updateQueue = newUpdateQueueWithIDs(2)
		existingDeployments = newExistenceListWithIDs(0, 2, 4, 6)
		forceUpgradeIfUnhealthy = false
		updateInjectFailures = k8sutil.Failures{}
		doSetup()
		addDeploymentOnNode("node0", 0)
		addDeploymentOnPVC("pvc2", 2)
		addDeploymentOnNode("node1", 4)
		addDeploymentOnPVC("pvc6", 6)

		osdToBeQueried = 2
		returnOkToStopIDs = []int{} // not ok to stop
		updateConfig.updateExistingOSDs(errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, deploymentsUpdated, []string{})
		assert.Equal(t, 1, updateQueue.Len()) // the OSD should have been requeued

		osdToBeQueried = 2
		returnOkToStopIDs = []int{2}
		updateConfig.updateExistingOSDs(errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, deploymentsUpdated, []string{deploymentName(2)})
		assert.Equal(t, 0, updateQueue.Len()) // the OSD should now have been removed from the queue
	})

	t.Run("PGs not clean to upgrade OSD", func(t *testing.T) {
		clientset = fake.NewSimpleClientset()
		updateQueue = newUpdateQueueWithIDs(2)
		existingDeployments = newExistenceListWithIDs(2)
		requiresHealthyPGs = true
		cephStatus = unHealthyCephStatus
		updateInjectFailures = k8sutil.Failures{}
		doSetup()

		osdToBeQueried = 2
		updateConfig.updateExistingOSDs(errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, deploymentsUpdated, []string{})
		assert.Equal(t, 1, updateQueue.Len()) // the OSD should remain
	})

	t.Run("PGs clean to upgrade OSD", func(t *testing.T) {
		clientset = fake.NewSimpleClientset()
		updateQueue = newUpdateQueueWithIDs(0)
		existingDeployments = newExistenceListWithIDs(0)
		requiresHealthyPGs = true
		cephStatus = healthyCephStatus
		forceUpgradeIfUnhealthy = true // FORCE UPDATES
		updateInjectFailures = k8sutil.Failures{}
		doSetup()
		addDeploymentOnNode("node0", 0)

		osdToBeQueried = 0
		returnOkToStopIDs = []int{0}
		updateConfig.updateExistingOSDs(errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, deploymentsUpdated, []string{deploymentName(0)})
		assert.Equal(t, 0, updateQueue.Len()) // should be done with updates
	})

	t.Run("continueUpgradesAfterChecksEvenIfUnhealthy = true", func(t *testing.T) {
		clientset = fake.NewSimpleClientset()
		updateQueue = newUpdateQueueWithIDs(2)
		existingDeployments = newExistenceListWithIDs(0, 2, 4, 6)
		forceUpgradeIfUnhealthy = true // FORCE UPDATES
		updateInjectFailures = k8sutil.Failures{}
		doSetup()
		addDeploymentOnNode("node0", 0)
		addDeploymentOnPVC("pvc2", 2)
		addDeploymentOnNode("node1", 4)
		addDeploymentOnPVC("pvc6", 6)

		osdToBeQueried = 2
		returnOkToStopIDs = []int{} // NOT ok-to-stop
		updateConfig.updateExistingOSDs(errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, deploymentsUpdated, []string{deploymentName(2)})

		assert.Equal(t, 0, updateQueue.Len()) // should be done with updates
	})

	t.Run("failures updating deployments", func(t *testing.T) {
		clientset = fake.NewSimpleClientset()
		updateQueue = newUpdateQueueWithIDs(0, 2, 4, 6)
		existingDeployments = newExistenceListWithIDs(0, 2, 4, 6)
		forceUpgradeIfUnhealthy = false
		updateInjectFailures = k8sutil.Failures{}
		doSetup()
		addDeploymentOnNode("node0", 0)
		addDeploymentOnPVC("pvc2", 2)
		addDeploymentOnNode("node1", 4)
		addDeploymentOnPVC("pvc6", 6)

		osdToBeQueried = 0
		returnOkToStopIDs = []int{0, 6}
		updateInjectFailures = k8sutil.Failures{
			{ResourceName: deploymentName(6), Error: errors.Errorf("induced failure updating OSD 6")},
		}
		updateConfig.updateExistingOSDs(errs)
		assert.Equal(t, 1, errs.len())
		assert.ElementsMatch(t, deploymentsUpdated,
			[]string{deploymentName(0), deploymentName(6)})

		deploymentsUpdated = []string{}
		osdToBeQueried = 2
		returnOkToStopIDs = []int{2, 4}
		updateInjectFailures = k8sutil.Failures{
			{ResourceName: deploymentName(2), Error: errors.Errorf("induced failure updating OSD 2")},
			{ResourceName: deploymentName(4), Error: errors.Errorf("induced failure waiting for OSD 4")},
		}
		updateConfig.updateExistingOSDs(errs)
		assert.Equal(t, 3, errs.len()) // errors should be appended to the same provisionErrors struct
		assert.ElementsMatch(t, deploymentsUpdated,
			[]string{deploymentName(2), deploymentName(4)})

		assert.Zero(t, updateQueue.Len()) // errors should not be requeued
	})

	t.Run("failure due to OSD deployment with bad info", func(t *testing.T) {
		clientset = fake.NewSimpleClientset()
		updateQueue = newUpdateQueueWithIDs(0, 6)
		existingDeployments = newExistenceListWithIDs(0, 2, 4, 6)
		forceUpgradeIfUnhealthy = false
		updateInjectFailures = k8sutil.Failures{}
		doSetup()
		addDeploymentOnNode("node0", 0)
		addDeploymentOnPVC("pvc2", 2)
		addDeploymentOnNode("node1", 4)
		addDeploymentOnPVC("pvc6", 6)
		// give OSD 6 bad info by removing env vars from primary container
		deploymentClient := clientset.AppsV1().Deployments(namespace)
		d, err := deploymentClient.Get(context.TODO(), deploymentName(6), metav1.GetOptions{})
		if err != nil {
			panic(err)
		}
		d.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{}
		_, err = deploymentClient.Update(context.TODO(), d, metav1.UpdateOptions{})
		if err != nil {
			panic(err)
		}
		deploymentsUpdated = []string{}

		osdToBeQueried = 0
		returnOkToStopIDs = []int{0, 6}
		updateConfig.updateExistingOSDs(errs)
		assert.Equal(t, 1, errs.len())
		assert.ElementsMatch(t, deploymentsUpdated,
			[]string{deploymentName(0)})

		assert.Zero(t, updateQueue.Len()) // errors should not be requeued
	})

	t.Run("do not update OSDs on nodes removed from the storage spec", func(t *testing.T) {
		clientset = fake.NewSimpleClientset()
		// reminder that updateQueue pops things off the queue from the front, so the leftmost item
		// will be the one queried
		updateQueue = newUpdateQueueWithIDs(0, 4)
		existingDeployments = newExistenceListWithIDs(0, 4)
		forceUpgradeIfUnhealthy = false
		updateInjectFailures = k8sutil.Failures{}
		doSetup()
		addDeploymentOnNode("node0", 0)
		addDeploymentOnNode("node1", 4)

		// Remove "node0" from valid storage (user no longer wants it)
		assert.Equal(t, "node0", c.ValidStorage.Nodes[0].Name)
		c.ValidStorage.Nodes = c.ValidStorage.Nodes[1:]
		t.Logf("valid storage nodes: %+v", c.ValidStorage.Nodes)

		osdToBeQueried = 0
		returnOkToStopIDs = []int{0, 4}
		updateConfig.updateExistingOSDs(errs)
		assert.Zero(t, errs.len())
		assert.ElementsMatch(t, deploymentsUpdated, []string{deploymentName(4)})

		assert.ElementsMatch(t, osdsOnNodes, []int{4})
		assert.ElementsMatch(t, osdsOnPVCs, []int{})

		assert.Equal(t, 0, updateQueue.Len()) // should be done with updates
	})

	t.Run("skip osd reconcile", func(t *testing.T) {
		clientset = fake.NewSimpleClientset()
		updateQueue = newUpdateQueueWithIDs(0, 1)
		existingDeployments = newExistenceListWithIDs(0)
		forceUpgradeIfUnhealthy = true
		updateInjectFailures = k8sutil.Failures{}
		doSetup()
		addDeploymentOnNode("node0", 0)

		osdToBeQueried = 0
		updateConfig.osdsToSkipReconcile.Insert("0")
		updateConfig.updateExistingOSDs(errs)
		assert.Zero(t, errs.len())
		assert.Equal(t, 1, updateQueue.Len())
		osdIDUpdated, ok := updateQueue.Pop()
		assert.True(t, ok)
		assert.Equal(t, 1, osdIDUpdated)
		updateConfig.osdsToSkipReconcile.Delete("0")
	})
}

func Test_getOSDUpdateInfo(t *testing.T) {
	namespace := "rook-ceph"
	cephImage := "quay.io/ceph/ceph:v15"

	// NOTE: all tests share the same clientset
	clientset := fake.NewSimpleClientset()
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   namespace,
		CephVersion: cephver.Squid,
	}
	clusterInfo.SetName("mycluster")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	spec := cephv1.ClusterSpec{
		CephVersion: cephv1.CephVersionSpec{Image: cephImage},
	}
	c := New(ctx, clusterInfo, spec, "rook/rook:master")

	var errs *provisionErrors
	var d *appsv1.Deployment

	t.Run("cluster with no existing deployments", func(t *testing.T) {
		errs = newProvisionErrors()
		updateQueue, existenceList, err := c.getOSDUpdateInfo(errs)
		assert.NoError(t, err)
		assert.Zero(t, errs.len())
		assert.Zero(t, updateQueue.Len())
		assert.Zero(t, existenceList.Len())
	})

	t.Run("cluster in namespace with existing deployments, but none are OSDs", func(t *testing.T) {
		// random deployment in this namespace
		addTestDeployment(clientset, "non-rook-deployment", namespace, map[string]string{})

		// mon.a in this namespace
		l := controller.CephDaemonAppLabels("rook-ceph-mon", namespace, "mon", "a", "rook-ceph-operator", "cephclusters.ceph.rook.io", true)
		addTestDeployment(clientset, "rook-ceph-mon-a", namespace, l)

		// osd.1 and 3 in another namespace (another Rook cluster)
		clusterInfo2 := &cephclient.ClusterInfo{
			Namespace:   "other-namespace",
			CephVersion: cephver.Squid,
		}
		clusterInfo2.SetName("other-cluster")
		clusterInfo2.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
		c2 := New(ctx, clusterInfo2, spec, "rook/rook:master")

		// osd.1 on PVC in "other-namespace"
		d = getDummyDeploymentOnPVC(clientset, c2, "pvc1", 1)
		createDeploymentOrPanic(clientset, d)

		// osd.3 on Node in "other-namespace"
		d = getDummyDeploymentOnNode(clientset, c2, "node3", 3)
		createDeploymentOrPanic(clientset, d)

		errs = newProvisionErrors()
		updateQueue, existenceList, err := c.getOSDUpdateInfo(errs)
		assert.NoError(t, err)
		assert.Zero(t, errs.len())
		assert.Zero(t, updateQueue.Len())
		assert.Zero(t, existenceList.Len())
	})

	t.Run("cluster in namespace with existing OSD deployments", func(t *testing.T) {
		// osd.0 on PVC in this namespace
		d = getDummyDeploymentOnPVC(clientset, c, "pvc0", 0)
		createDeploymentOrPanic(clientset, d)

		// osd.2 on Node in this namespace
		d = getDummyDeploymentOnNode(clientset, c, "node2", 2)
		createDeploymentOrPanic(clientset, d)

		errs = newProvisionErrors()
		updateQueue, existenceList, err := c.getOSDUpdateInfo(errs)
		assert.NoError(t, err)
		assert.Zero(t, errs.len())
		assert.Equal(t, 2, updateQueue.Len())
		assert.True(t, updateQueue.Exists(0))
		assert.True(t, updateQueue.Exists(2))
		assert.Equal(t, 2, existenceList.Len())
		assert.True(t, existenceList.Exists(0))
		assert.True(t, existenceList.Exists(2))
	})

	t.Run("existing OSD deployment with no OSD ID", func(t *testing.T) {
		l := map[string]string{k8sutil.AppAttr: AppName}
		addTestDeployment(clientset, "rook-ceph-osd-NOID", namespace, l)

		errs = newProvisionErrors()
		updateQueue, existenceList, err := c.getOSDUpdateInfo(errs)
		assert.NoError(t, err)
		assert.Equal(t, 1, errs.len())
		// should have same update queue and existence list as last test
		assert.Equal(t, 2, updateQueue.Len())
		assert.Equal(t, 2, existenceList.Len())
	})

	t.Run("failure to list OSD deployments", func(t *testing.T) {
		// reset the test to check that an error is reported if listing OSD deployments fails
		test.PrependFailReactor(t, clientset, "list", "deployments")
		ctx = &clusterd.Context{
			Clientset: clientset,
		}
		c = New(ctx, clusterInfo, spec, "rook/rook:master")

		errs = newProvisionErrors()
		_, _, err := c.getOSDUpdateInfo(errs)
		fmt.Println(err)
		assert.Error(t, err)
	})
}

func addTestDeployment(clientset *fake.Clientset, name, namespace string, labels map[string]string) {
	d := &appsv1.Deployment{}
	d.SetName(name)
	d.SetNamespace(namespace)
	d.SetLabels(labels)
	createDeploymentOrPanic(clientset, d)
}

func createDeploymentOrPanic(clientset *fake.Clientset, d *appsv1.Deployment) {
	_, err := clientset.AppsV1().Deployments(d.Namespace).Create(context.TODO(), d, metav1.CreateOptions{})
	if err != nil {
		panic(err)
	}
}

func Test_updateQueue(t *testing.T) {
	q := newUpdateQueueWithCapacity(2)
	assert.Equal(t, 2, cap(q.q))
	assert.Zero(t, q.Len())

	testPop := func(osdID int) {
		t.Helper()
		id, ok := q.Pop()
		assert.Equal(t, osdID, id)
		assert.True(t, ok)
	}

	assertEmpty := func() {
		t.Helper()
		id, ok := q.Pop()
		assert.Equal(t, -1, id)
		assert.False(t, ok)
	}

	// assert empty behavior initially
	assertEmpty()

	// test basic functionality
	q.Push(0)
	assert.Equal(t, 1, q.Len())
	testPop(0)
	assertEmpty()

	// test that queue can hold more items than initial capacity
	// and that items are Pushed/Popped in FIFO order
	q.Push(1)
	q.Push(2)
	q.Push(3)
	assert.Equal(t, 3, q.Len())
	assert.True(t, q.Exists(1))
	assert.True(t, q.Exists(2))
	assert.True(t, q.Exists(3))
	testPop(1)
	testPop(2)
	testPop(3)
	assertEmpty()

	// Test removing queue items via q.Remove
	for _, i := range []int{4, 5, 6, 7, 8} {
		q.Push(i)
		assert.True(t, q.Exists(i))
	}
	assert.Equal(t, 5, q.Len())
	q.Remove([]int{
		1, 2, 3, // non-existent items shouldn't affect queue
		4, // remove first item
		6, // remove a middle item
		8, // remove last item
	})
	assert.Equal(t, 2, q.Len())
	assert.False(t, q.Exists(1))
	assert.False(t, q.Exists(2))
	assert.False(t, q.Exists(3))
	assert.True(t, q.Exists(5))
	assert.False(t, q.Exists(6))
	assert.True(t, q.Exists(7))
	assert.False(t, q.Exists(8))
	testPop(5)
	// items pushed back onto the queue after removal should not get old values
	q.Push(9)
	q.Push(10)
	assert.Equal(t, 3, q.Len())
	assert.False(t, q.Exists(5))
	assert.True(t, q.Exists(7))
	assert.True(t, q.Exists(9))
	assert.True(t, q.Exists(10))
	testPop(7)
	testPop(9)
	testPop(10)
	assertEmpty()
}

func Test_existenceList(t *testing.T) {
	l := newExistenceListWithCapacity(2)

	// Assert zero item does not exist initially
	assert.False(t, l.Exists(0))
	assert.Zero(t, l.Len())

	// Assert basic functionality
	l.Add(1)
	assert.True(t, l.Exists(1))
	assert.False(t, l.Exists(0))
	assert.False(t, l.Exists(2))
	assert.Equal(t, 1, l.Len())

	// assert that more items can be added than initial capacity
	l.Add(0)
	l.Add(2)
	l.Add(3)
	assert.True(t, l.Exists(0))
	assert.True(t, l.Exists(1)) // 1 should still exist from before
	assert.True(t, l.Exists(2))
	assert.True(t, l.Exists(3))
	assert.False(t, l.Exists(4))
	assert.Equal(t, 4, l.Len())

	// assert that the same item can be added twice (though this should never happen for OSDs IRL)
	l.Add(1)
	assert.True(t, l.Exists(1))
	assert.Equal(t, 4, l.Len())
}

func TestCluster_rotateCephxKey(t *testing.T) {
	// auth rotate returns an array instead of a single object
	rotatedKeyJson := `[{"key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="}]`
	rotateCalledForEntity := ""
	sharedExecutor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			t.Logf("command: %s %v", command, args)
			if command == "ceph" && args[0] == "auth" && args[1] == "rotate" {
				rotateCalledForEntity = args[2]
				return rotatedKeyJson, nil
			}
			panic(fmt.Sprintf("unexpected command %q %v", command, args))
		},
	}

	newTest := func(daemonCephxConfig cephv1.CephxConfig) *Cluster {
		rotateCalledForEntity = "" // reset to empty each test

		clusterd := clusterd.Context{
			Executor: sharedExecutor,
		}
		clusterInfo := cephclient.ClusterInfo{
			Context:     context.TODO(),
			Namespace:   "ns",
			CephVersion: cephver.CephVersion{Major: 20, Minor: 2},
		}
		return &Cluster{
			context:     &clusterd,
			clusterInfo: &clusterInfo,
			spec: cephv1.ClusterSpec{
				Security: cephv1.ClusterSecuritySpec{
					CephX: cephv1.ClusterCephxConfig{
						Daemon: daemonCephxConfig,
					},
				},
			},
		}
	}

	t.Run("empty status and config", func(t *testing.T) {
		c := newTest(cephv1.CephxConfig{})
		osdInfo := OSDInfo{
			ID:          1,
			CephxStatus: cephv1.CephxStatus{},
		}

		cephxStatus, err := c.rotateCephxKey(osdInfo)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.CephxStatus{}, cephxStatus)
		assert.Equal(t, rotateCalledForEntity, "")
	})

	t.Run("empty status, disabled config", func(t *testing.T) { // brownfield, no rotation
		c := newTest(cephv1.CephxConfig{
			KeyRotationPolicy: "Disabled",
			KeyGeneration:     3,
		})
		osdInfo := OSDInfo{
			ID:          1,
			CephxStatus: cephv1.CephxStatus{},
		}

		cephxStatus, err := c.rotateCephxKey(osdInfo)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.CephxStatus{}, cephxStatus)
		assert.Equal(t, rotateCalledForEntity, "")
	})

	t.Run("empty status, enabled config", func(t *testing.T) { // brownfield with rotation
		c := newTest(cephv1.CephxConfig{
			KeyRotationPolicy: "KeyGeneration",
			KeyGeneration:     3,
		})
		osdInfo := OSDInfo{
			ID:          1,
			CephxStatus: cephv1.CephxStatus{},
		}

		cephxStatus, err := c.rotateCephxKey(osdInfo)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.CephxStatus{KeyCephVersion: "20.2.0-0", KeyGeneration: 3}, cephxStatus)
		assert.Equal(t, rotateCalledForEntity, "osd.1")
	})

	t.Run("undefined status, empty config", func(t *testing.T) { // greenfield, no rotation
		c := newTest(cephv1.CephxConfig{})
		osdInfo := OSDInfo{
			ID:          1,
			CephxStatus: keyring.UninitializedCephxStatus(), // shouldn't happen in reality
		}

		cephxStatus, err := c.rotateCephxKey(osdInfo)
		assert.NoError(t, err)
		// results in unnecessary OSD restart, but ensures that future rotations aren't blocked
		// in the unlikely event the uninitialized status is erroneously applied to osd deployment
		assert.Equal(t, cephv1.CephxStatus{KeyCephVersion: "20.2.0-0", KeyGeneration: 1}, cephxStatus)
		assert.Equal(t, rotateCalledForEntity, "")
	})

	t.Run("set status, empty config", func(t *testing.T) {
		c := newTest(cephv1.CephxConfig{})
		osdInfo := OSDInfo{
			ID:          1,
			CephxStatus: cephv1.CephxStatus{KeyGeneration: 1, KeyCephVersion: "19.2.6-0"},
		}

		cephxStatus, err := c.rotateCephxKey(osdInfo)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.CephxStatus{KeyCephVersion: "19.2.6-0", KeyGeneration: 1}, cephxStatus)
		assert.Equal(t, rotateCalledForEntity, "")
	})

	t.Run("set status, enabled config", func(t *testing.T) {
		c := newTest(cephv1.CephxConfig{
			KeyRotationPolicy: "KeyGeneration",
			KeyGeneration:     4,
		})
		osdInfo := OSDInfo{
			ID:          2,
			CephxStatus: cephv1.CephxStatus{KeyGeneration: 1, KeyCephVersion: "19.2.6-0"},
		}

		cephxStatus, err := c.rotateCephxKey(osdInfo)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.CephxStatus{KeyCephVersion: "20.2.0-0", KeyGeneration: 4}, cephxStatus)
		assert.Equal(t, rotateCalledForEntity, "osd.2")
	})

	t.Run("auth rotate failure", func(t *testing.T) {
		// this case shouldn't happen in code, but it could repair a botched greenfield deploy
		c := newTest(cephv1.CephxConfig{
			KeyRotationPolicy: "KeyGeneration",
			KeyGeneration:     4,
		})
		c.context.Executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
				rotateCalledForEntity = arg[2]
				return "", fmt.Errorf("mock error")
			},
		}
		osdInfo := OSDInfo{
			ID:          2,
			CephxStatus: cephv1.CephxStatus{KeyGeneration: 1, KeyCephVersion: "19.2.6-0"},
		}

		cephxStatus, err := c.rotateCephxKey(osdInfo)
		assert.Error(t, err)
		assert.Equal(t, cephv1.CephxStatus{KeyCephVersion: "19.2.6-0", KeyGeneration: 1}, cephxStatus)
		assert.Equal(t, rotateCalledForEntity, "osd.2")
	})
}
