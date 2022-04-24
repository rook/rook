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
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephclientfake "github.com/rook/rook/pkg/daemon/ceph/client/fake"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

var (
	// global to allow creating helper functions that are not inline with test functions
	testIDGenerator osdIDGenerator
)

// TODO: look into: failed to calculate diff between current deployment and newly generated one.
//   Failed to generate strategic merge patch: map: map[] does not contain declared merge key: uid
//   appears in unit tests, but does it appear in real-world?

// Test integration between OSD creates and updates including on both nodes and PVCs.
// The definition for this test is a wrapper for the test function that adds a timeout.
func TestOSDIntegration(t *testing.T) {
	oldLogger := *logger
	defer func() { logger = &oldLogger }() // reset logger to default after this test
	logger.SetLevel(capnslog.TRACE)        // want more log info for this test if it fails

	oldOpportunisticDuration := osdOpportunisticUpdateDuration
	oldMinuteDuration := minuteTickerDuration
	defer func() {
		osdOpportunisticUpdateDuration = oldOpportunisticDuration
		minuteTickerDuration = oldMinuteDuration
	}()
	// lower the check durations for unit tests to speed them up
	osdOpportunisticUpdateDuration = 1 * time.Millisecond
	minuteTickerDuration = 3 * time.Millisecond

	done := make(chan bool)
	// runs in less than 650ms on 6-core, 16MB RAM system, but github CI can be much slower
	timeout := time.After(20 * 750 * time.Millisecond)

	go func() {
		// use defer because t.Fatal will kill this goroutine, and we always want done set if the
		// test func stops running
		defer func() { done <- true }()
		// run the actual test
		testOSDIntegration(t)
	}()

	select {
	case <-timeout:
		t.Fatal("Test timed out. This is a test failure.")
	case <-done:
	}
}

// This is the actual test. If it hangs, we should consider that an error.
func testOSDIntegration(t *testing.T) {
	ctx := context.TODO()
	contextCancel, cancel := context.WithCancel(ctx)
	namespace := "osd-integration"
	clusterName := "my-cluster"

	testIDGenerator = newOSDIDGenerator()

	// mock/stub functions as needed
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
	clientset := test.NewComplexClientset(t)
	test.AddSomeReadyNodes(t, clientset, 3)
	assignPodToNode := true
	test.PrependComplexJobReactor(t, clientset, assignPodToNode)
	test.SetFakeKubernetesVersion(clientset, "v1.13.2") // v1.13 or higher is required for OSDs on PVC

	os.Setenv(k8sutil.PodNamespaceEnvVar, namespace)
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	statusMapWatcher := watch.NewRaceFreeFake()
	clientset.PrependWatchReactor("configmaps", k8stesting.DefaultWatchReactor(statusMapWatcher, nil))

	failCreatingDeployments := []string{}
	failUpdatingDeployments := []string{}
	deploymentGeneration := int64(1) // mock deployment generation constantly increasing
	deploymentsCreated := []string{}
	deploymentsUpdated := []string{}
	var deploymentReactor k8stesting.ReactionFunc = func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		switch action := action.(type) {
		case k8stesting.CreateActionImpl:
			obj := action.GetObject()
			d, ok := obj.(*appsv1.Deployment)
			if !ok {
				panic(fmt.Sprintf("object not a deployment: %+v", obj))
			}
			t.Logf("deployment reactor: create event for deployment %q", d.Name)
			// (1) keep track of deployments which have been created
			deploymentsCreated = append(deploymentsCreated, d.Name)
			// (2) set deployments ready immediately. Don't have to test waiting for deployments to
			//     be ready b/c that is tested thoroughly in update_test.go
			d.Status.ObservedGeneration = deploymentGeneration
			d.Status.UpdatedReplicas = 1
			d.Status.ReadyReplicas = 1
			deploymentGeneration++
			// (3) return a failure if asked
			for _, match := range failCreatingDeployments {
				if strings.Contains(d.Name, match) {
					return true, nil, errors.Errorf("induced error creating deployment %q", d.Name)
				}
			}

		case k8stesting.UpdateActionImpl:
			obj := action.GetObject()
			d, ok := obj.(*appsv1.Deployment)
			if !ok {
				panic(fmt.Sprintf("object not a deployment: %+v", obj))
			}
			t.Logf("deployment reactor: update event for deployment %q", d.Name)
			// (1) keep track of deployments which have been created
			deploymentsUpdated = append(deploymentsUpdated, d.Name)
			// (2) set deployments ready immediately. Don't have to test waiting for deployments to
			//     be ready b/c that is tested thoroughly in update_test.go
			d.Status.ObservedGeneration = deploymentGeneration
			d.Status.UpdatedReplicas = 1
			d.Status.ReadyReplicas = 1
			deploymentGeneration++
			// (3) return a failure if asked
			for _, match := range failUpdatingDeployments {
				if strings.Contains(d.Name, match) {
					return true, nil, errors.Errorf("induced error creating deployment %q", d.Name)
				}
			}

		case k8stesting.DeleteActionImpl:
			panic(fmt.Sprintf("deployments should not be deleted: %+v", action))
		}

		// modify the object in-place so that the default reactor will create it with our
		// modifications, if we have made any
		return false, nil, nil
	}
	clientset.PrependReactor("*", "deployments", deploymentReactor)

	clusterInfo := cephclient.NewClusterInfo(namespace, clusterName)
	clusterInfo.CephVersion = cephver.Pacific
	clusterInfo.SetName("mycluster")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	clusterInfo.Context = ctx
	executor := osdIntegrationTestExecutor(t, clientset, namespace)

	rootCtx := &clusterd.Context{
		Clientset: clientset,
		ConfigDir: "/var/lib/rook",
		Executor:  executor,
	}
	spec := cephv1.ClusterSpec{
		CephVersion: cephv1.CephVersionSpec{
			Image: "quay.io/ceph/ceph:v16.2.0",
		},
		DataDirHostPath: rootCtx.ConfigDir,
		// This storage spec should... (see inline)
		Storage: cephv1.StorageScopeSpec{
			UseAllNodes: true,
			Selection: cephv1.Selection{
				// ... create 2 osd on each of the 3 nodes (6 total)
				DeviceFilter: "vd[ab]",
			},
			StorageClassDeviceSets: []cephv1.StorageClassDeviceSet{
				// ... create 6 portable osds
				newDummyStorageClassDeviceSet("portable-set", namespace, 6, true, "ec2"),
				// ... create 3 local, non-portable osds, one on each node
				newDummyStorageClassDeviceSet("local-set", namespace, 3, false, "local"),
			},
		},
	}
	osdsPerNode := 2 // vda and vdb

	c := New(rootCtx, clusterInfo, spec, "myversion")

	var startErr error
	var done bool
	runReconcile := func(ctx context.Context) {
		// reset environment
		c = New(rootCtx, clusterInfo, spec, "myversion")
		clusterInfo.Context = ctx
		statusMapWatcher.Reset()

		// reset counters
		deploymentsCreated = []string{}
		deploymentsUpdated = []string{}
		done = false

		startErr = c.Start()
		done = true
	}
	waitForDone := func() {
		for {
			if done == true {
				return
			}
			time.Sleep(1 * time.Millisecond)
		}
	}

	// NOTE: these tests all use the same environment
	t.Run("initial creation", func(t *testing.T) {
		go runReconcile(contextCancel)

		cms := waitForNumConfigMaps(clientset, namespace, 12) // 3 nodes + 9 new PVCs
		for _, cm := range cms {
			cpy := cm.DeepCopy()
			setStatusConfigMapToCompleted(t, cpy, osdsPerNode)
			updateStatusConfigmap(clientset, statusMapWatcher, cpy)
		}

		waitForDone()
		assert.NoError(t, startErr)
		assert.Len(t, deploymentsCreated, 15)
		assert.Len(t, deploymentsUpdated, 0)
	})

	t.Run("reconcile again with no changes", func(t *testing.T) {
		go runReconcile(contextCancel)

		cms := waitForNumConfigMaps(clientset, namespace, 3) // 3 nodes
		for _, cm := range cms {
			cpy := cm.DeepCopy()
			setStatusConfigMapToCompleted(t, cpy, osdsPerNode)
			updateStatusConfigmap(clientset, statusMapWatcher, cpy)
		}

		waitForDone()
		assert.NoError(t, startErr)
		assert.Len(t, deploymentsCreated, 0)
		assert.Len(t, deploymentsUpdated, 15)
	})

	t.Run("increase number of OSDs", func(t *testing.T) {
		spec.Storage.Selection.DeviceFilter = "/dev/vd[abc]" // 3 more (1 more per node)
		spec.Storage.StorageClassDeviceSets[0].Count = 8     // 2 more portable
		spec.Storage.StorageClassDeviceSets[1].Count = 6     // 3 more (1 more per node)
		osdsPerNode = 3                                      // vda, vdb, vdc

		go runReconcile(contextCancel)

		cms := waitForNumConfigMaps(clientset, namespace, 8) // 3 nodes + 5 new PVCs
		for _, cm := range cms {
			cpy := cm.DeepCopy()
			setStatusConfigMapToCompleted(t, cpy, osdsPerNode)
			updateStatusConfigmap(clientset, statusMapWatcher, cpy)
		}

		waitForDone()
		assert.NoError(t, startErr)
		assert.Len(t, deploymentsCreated, 8)
		assert.Len(t, deploymentsUpdated, 15)
	})

	t.Run("mixed create and update, cancel reconcile, and continue reconcile", func(t *testing.T) {
		spec.Storage.Selection.DeviceFilter = "/dev/vd[abcd]" // 3 more (1 more per node)
		spec.Storage.StorageClassDeviceSets[0].Count = 10     // 2 more portable
		osdsPerNode = 4                                       // vd[a-d]

		go runReconcile(contextCancel)
		cms := waitForNumConfigMaps(clientset, namespace, 5) // 3 nodes + 2 new PVCs
		i := 1
		for _, cm := range cms {
			if !strings.Contains(cm.Name, "node") {
				// only do node configmaps right now since those are always created and we want
				// a deterministic number of configmaps in the next step
				continue
			}
			cpy := cm.DeepCopy()
			setStatusConfigMapToCompleted(t, cpy, osdsPerNode)
			updateStatusConfigmap(clientset, statusMapWatcher, cpy)
			if i == 2 {
				t.Log("canceling orchestration")
				time.Sleep(10 * time.Millisecond)
				// after the second status map is made ready, cancel the orchestration. wait a short
				// while to make sure the watcher picks up the updated change
				cancel()
				// refresh the context so the rest of the test can continue
				contextCancel = context.TODO()
				break
			}
			i++
		}
		waitForDone()
		assert.Error(t, startErr)
		t.Logf("c.Start() error: %+v", startErr)
		// should have created 2 more OSDs for the configmaps we updated
		assert.Len(t, deploymentsCreated, 2)
		// we don't know exactly how many updates might have happened by this point
		numUpdates := len(deploymentsUpdated)
		t.Logf("deployments updated: %d", numUpdates)

		go runReconcile(contextCancel)
		cms = waitForNumConfigMaps(clientset, namespace, 5) // 3 nodes + 2 new PVCs
		for _, cm := range cms {
			cpy := cm.DeepCopy()
			setStatusConfigMapToCompleted(t, cpy, osdsPerNode)
			updateStatusConfigmap(clientset, statusMapWatcher, cpy)
		}
		waitForDone()
		assert.NoError(t, startErr)
		assert.Len(t, deploymentsCreated, 3)  // 5 less the 2 created in the previous step
		assert.Len(t, deploymentsUpdated, 25) // 23 + 2 created in previous step
	})

	t.Run("failures reported in status configmaps", func(t *testing.T) {
		spec.Storage.Selection.DeviceFilter = "/dev/vd[abcde]" // 3 more (1 more per node)
		osdsPerNode = 5                                        // vd[a-e]

		go runReconcile(contextCancel)
		cms := waitForNumConfigMaps(clientset, namespace, 3) // 3 nodes
		for _, cm := range cms {
			cpy := cm.DeepCopy()
			if strings.Contains(cm.Name, "node1") {
				setStatusConfigMapToCompleted(t, cpy, osdsPerNode)
			} else {
				setStatusConfigMapToFailed(t, cpy) // fail on node0 and node2
			}
			updateStatusConfigmap(clientset, statusMapWatcher, cpy)
		}
		waitForDone()
		assert.Error(t, startErr)
		t.Logf("c.Start() error: %+v", startErr)
		assert.Len(t, deploymentsCreated, 1)
		assert.Len(t, deploymentsUpdated, 28)

		// should get back to healthy after
		go runReconcile(contextCancel)
		cms = waitForNumConfigMaps(clientset, namespace, 3) // 3 nodes
		for _, cm := range cms {
			cpy := cm.DeepCopy()
			setStatusConfigMapToCompleted(t, cpy, osdsPerNode)
			updateStatusConfigmap(clientset, statusMapWatcher, cpy)
		}
		waitForDone()
		assert.NoError(t, startErr)
		assert.Len(t, deploymentsCreated, 2)
		assert.Len(t, deploymentsUpdated, 29) // 28 + 1 created in previous step
	})

	t.Run("failures during deployment updates", func(t *testing.T) {
		failUpdatingDeployments = []string{"osd-15", "osd-22"}
		go runReconcile(contextCancel)
		cms := waitForNumConfigMaps(clientset, namespace, 3) // 3 nodes
		for _, cm := range cms {
			cpy := cm.DeepCopy()
			setStatusConfigMapToCompleted(t, cpy, osdsPerNode)
			updateStatusConfigmap(clientset, statusMapWatcher, cpy)
		}
		waitForDone()
		assert.Error(t, startErr)
		t.Logf("c.Start() error: %+v", startErr)
		assert.Len(t, deploymentsCreated, 0)
		assert.Len(t, deploymentsUpdated, 31) // should attempt to update all deployments

		failUpdatingDeployments = []string{}
		go runReconcile(contextCancel)
		cms = waitForNumConfigMaps(clientset, namespace, 3) // 3 nodes
		for _, cm := range cms {
			cpy := cm.DeepCopy()
			setStatusConfigMapToCompleted(t, cpy, osdsPerNode)
			updateStatusConfigmap(clientset, statusMapWatcher, cpy)
		}
		waitForDone()
		assert.NoError(t, startErr)
		assert.Len(t, deploymentsCreated, 0)
		assert.Len(t, deploymentsUpdated, 31) // all deployments should be updated again
	})

	t.Run("failures during deployment creation", func(t *testing.T) {
		spec.Storage.Selection.DeviceFilter = "/dev/vd[abcdef]" // 3 more (1 more per node)
		osdsPerNode = 6                                         // vd[a-f]

		failCreatingDeployments = []string{"osd-31", "osd-33"}
		go runReconcile(contextCancel)
		cms := waitForNumConfigMaps(clientset, namespace, 3) // 3 nodes
		for _, cm := range cms {
			cpy := cm.DeepCopy()
			setStatusConfigMapToCompleted(t, cpy, osdsPerNode)
			updateStatusConfigmap(clientset, statusMapWatcher, cpy)
		}
		waitForDone()
		assert.Error(t, startErr)
		t.Logf("c.Start() error: %+v", startErr)
		assert.Len(t, deploymentsCreated, 3)
		assert.Len(t, deploymentsUpdated, 31)

		failCreatingDeployments = []string{}
		go runReconcile(contextCancel)
		cms = waitForNumConfigMaps(clientset, namespace, 3) // 3 nodes
		for _, cm := range cms {
			cpy := cm.DeepCopy()
			setStatusConfigMapToCompleted(t, cpy, osdsPerNode)
			updateStatusConfigmap(clientset, statusMapWatcher, cpy)
		}
		waitForDone()
		assert.NoError(t, startErr)
		assert.Len(t, deploymentsCreated, 2)  // the 2 deployments NOT created before should be created now
		assert.Len(t, deploymentsUpdated, 32) // 31 + 1 from previous step
	})

	t.Run("failures from improperly formatted StorageClassDeviceSet", func(t *testing.T) {
		newSCDS := cephv1.StorageClassDeviceSet{
			Name:                 "new",
			Count:                3,
			Portable:             true,
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
		}
		spec.Storage.StorageClassDeviceSets = append(spec.Storage.StorageClassDeviceSets, newSCDS)

		go runReconcile(contextCancel)
		cms := waitForNumConfigMaps(clientset, namespace, 3) // 3 nodes
		for _, cm := range cms {
			cpy := cm.DeepCopy()
			setStatusConfigMapToCompleted(t, cpy, osdsPerNode)
			updateStatusConfigmap(clientset, statusMapWatcher, cpy)
		}
		waitForDone()
		assert.Error(t, startErr)
		t.Logf("c.Start() error: %+v", startErr)
		assert.Len(t, deploymentsCreated, 0)
		assert.Len(t, deploymentsUpdated, 34)

		spec.Storage.StorageClassDeviceSets[2].VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			newDummyPVC("data", namespace, "100Gi", "ec2"),
			newDummyPVC("metadata", namespace, "10Gi", "uncle-rogers-secret-stuff"),
		}

		go runReconcile(contextCancel)
		cms = waitForNumConfigMaps(clientset, namespace, 6) // 3 nodes + 3 new PVCs
		for _, cm := range cms {
			cpy := cm.DeepCopy()
			setStatusConfigMapToCompleted(t, cpy, osdsPerNode)
			updateStatusConfigmap(clientset, statusMapWatcher, cpy)
		}
		waitForDone()
		assert.NoError(t, startErr)
		assert.Len(t, deploymentsCreated, 3)
		assert.Len(t, deploymentsUpdated, 34)
	})

	t.Run("clean up dangling configmaps", func(t *testing.T) {
		danglingCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dangling-status-configmap",
				Namespace: namespace,
				Labels:    statusConfigMapLabels("node0"),
			},
		}
		_, err := clientset.CoreV1().ConfigMaps(namespace).Create(ctx, danglingCM, metav1.CreateOptions{})
		assert.NoError(t, err)

		go runReconcile(contextCancel)
		cms := waitForNumConfigMaps(clientset, namespace, 4) // 3 nodes + dangling
		for _, cm := range cms {
			cpy := cm.DeepCopy()
			if cpy.Name == "dangling-status-configmap" {
				continue
			}
			setStatusConfigMapToCompleted(t, cpy, osdsPerNode)
			updateStatusConfigmap(clientset, statusMapWatcher, cpy)
		}
		waitForDone()
		assert.NoError(t, err)
		assert.Len(t, deploymentsCreated, 0)
		assert.Len(t, deploymentsUpdated, 37)

		cmList, err := clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Len(t, cmList.Items, 0)
	})
}

/*
 * mock executor to handle ceph commands
 */

func osdIntegrationTestExecutor(t *testing.T, clientset *fake.Clientset, namespace string) *exectest.MockExecutor {
	return &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			t.Logf("command: %s %v", command, args)
			if command != "ceph" {
				return "", errors.Errorf("unexpected command %q with args %v", command, args)
			}
			if args[0] == "auth" {
				if args[1] == "get-or-create-key" {
					return "{\"key\": \"does-not-matter\"}", nil
				}
			}
			if args[0] == "osd" {
				if args[1] == "ok-to-stop" {
					osdID := args[2]
					id, err := strconv.Atoi(osdID)
					if err != nil {
						panic(err)
					}
					t.Logf("returning ok for OSD %d", id)
					return cephclientfake.OsdOkToStopOutput(id, []int{id}, true), nil
				}
				if args[1] == "ls" {
					// ceph osd ls returns an array of osd IDs like [0,1,2]
					// build this based on the number of deployments since they should be equal
					// for this test
					l, err := clientset.AppsV1().Deployments(namespace).List(context.TODO(), metav1.ListOptions{})
					if err != nil {
						panic(fmt.Sprintf("failed to build 'ceph osd ls' output. %v", err))
					}
					return cephclientfake.OsdLsOutput(len(l.Items)), nil
				}
				if args[1] == "tree" {
					return cephclientfake.OsdTreeOutput(3, 3), nil // fake output for cluster with 3 nodes having 3 OSDs
				}
				if args[1] == "crush" {
					if args[2] == "get-device-class" {
						return cephclientfake.OSDDeviceClassOutput(args[3]), nil
					}
				}
			}
			if args[0] == "versions" {
				// the update deploy code only cares about the mons from the ceph version command results
				v := `{"mon":{"ceph version 16.2.2 (somehash) octopus (stable)":3}}`
				return v, nil
			}
			return "", errors.Errorf("unexpected ceph command %q", args)
		},
	}
}

/*
 * Unique and consistent OSD ID generator
 */

type osdIDGenerator struct {
	nextOSDID int
	osdIDMap  map[string]int
}

func newOSDIDGenerator() osdIDGenerator {
	return osdIDGenerator{
		nextOSDID: 0,
		osdIDMap:  map[string]int{},
	}
}

func (g *osdIDGenerator) osdID(t *testing.T, namedResource string) int {
	if id, ok := g.osdIDMap[namedResource]; ok {
		t.Logf("resource %q has existing OSD ID %d", namedResource, id)
		return id
	}
	id := g.nextOSDID
	g.osdIDMap[namedResource] = id
	g.nextOSDID++
	t.Logf("generated new OSD ID %d for resource %q", id, namedResource)
	return id
}

func newDummyStorageClassDeviceSet(
	name string, namespace string, count int, portable bool, storageClassName string,
) cephv1.StorageClassDeviceSet {
	return cephv1.StorageClassDeviceSet{
		Name:     name,
		Count:    count,
		Portable: portable,
		VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
			newDummyPVC("data", namespace, "10Gi", storageClassName),
		},
	}
}

func waitForNumConfigMaps(clientset kubernetes.Interface, namespace string, count int) []corev1.ConfigMap {
	for {
		cms, err := clientset.CoreV1().ConfigMaps(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			panic(err)
		}
		if len(cms.Items) >= count {
			return cms.Items
		}
		time.Sleep(1 * time.Microsecond)
	}
}

// Helper method to set "completed" status on "starting" ConfigMaps.
// If the configmap is a result for a provision on nodes, create numOSDsIfOnNode OSDs in the return
func setStatusConfigMapToCompleted(t *testing.T, cm *corev1.ConfigMap, numOSDsIfOnNode int) {
	status := parseOrchestrationStatus(cm.Data)
	t.Logf("updating configmap %q status to completed", cm.Name)
	// configmap names are deterministic can be mapped indirectly to an OSD ID, and since the
	// configmaps are used to report completion status of OSD provisioning, we use this property in
	// these unit tests
	status.Status = OrchestrationStatusCompleted
	if status.PvcBackedOSD {
		// only one OSD per PVC
		osdID := testIDGenerator.osdID(t, cm.Name)
		status.OSDs = []OSDInfo{
			{
				ID:        osdID,
				UUID:      fmt.Sprintf("%032d", osdID),
				BlockPath: "/dev/path/to/block",
				CVMode:    "raw",
			},
		}
	} else {
		status.OSDs = []OSDInfo{}
		for i := 0; i < numOSDsIfOnNode; i++ {
			// in order to generate multiple OSDs on a node, pretend they have different configmap
			// names (simply append the index). this is still deterministic.
			osdID := testIDGenerator.osdID(t, fmt.Sprintf("%s-%d", cm.Name, i))
			disk := k8sutil.IndexToName(i)
			status.OSDs = append(status.OSDs, OSDInfo{
				ID:        osdID,
				UUID:      fmt.Sprintf("%032d", osdID),
				BlockPath: fmt.Sprintf("/dev/vd%s", disk),
				CVMode:    "raw",
			})
		}
	}
	s, _ := json.Marshal(status)
	cm.Data[orchestrationStatusKey] = string(s)
}

func setStatusConfigMapToFailed(t *testing.T, cm *corev1.ConfigMap) {
	status := parseOrchestrationStatus(cm.Data)
	t.Logf("updating configmap %q status to failed", cm.Name)
	status.Status = OrchestrationStatusFailed
	s, _ := json.Marshal(status)
	cm.Data[orchestrationStatusKey] = string(s)
}

func updateStatusConfigmap(clientset kubernetes.Interface, statusMapWatcher *watch.RaceFreeFakeWatcher, cm *corev1.ConfigMap) {
	_, err := clientset.CoreV1().ConfigMaps(cm.Namespace).Update(context.TODO(), cm, metav1.UpdateOptions{})
	if err != nil {
		panic(err)
	}
	statusMapWatcher.Modify(cm)
}
