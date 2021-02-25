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
	"sync"
	"testing"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/tevino/abool"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// The definition for this test is a wrapper for the test function that adds a timeout
func TestOSDsOnPVC(t *testing.T) {
	oldLogger := *logger
	defer func() { logger = &oldLogger }() // reset logger to default after this test
	logger.SetLevel(capnslog.TRACE)        // want more log info for this test if it fails

	oldOpportunisticDuration := osdOpportunisticUpdateDuration
	defer func() { osdOpportunisticUpdateDuration = oldOpportunisticDuration }()
	// lower the opportunistic update check duration for unit tests to speed them up
	osdOpportunisticUpdateDuration = 1 * time.Millisecond

	// This test runs in less than 150 milliseconds on a 6-core CPU w/ 16GB RAM
	// GitHub CI runner can be a lot slower (~2.5 seconds)
	done := make(chan bool)
	timeout := time.After(50 * 150 * time.Millisecond)

	go func() {
		// use defer because t.Fatal will kill this goroutine, and we always want done set if the
		// func stops running
		defer func() { done <- true }()
		// run the actual test
		testOSDsOnPVC(t)
	}()

	select {
	case <-timeout:
		t.Fatal("Test timed out. This is a test failure.")
	case <-done:
	}
}

// This is the actual test. If it hangs, we should consider that an error. Writing a timeout in
// tests requires running the test in a goroutine, so there is a timeout wrapper above
func testOSDsOnPVC(t *testing.T) {
	namespace := "osd-on-pvc"
	ctx := context.TODO()
	// we don't need to create actual nodes for this test, but this is the set of nodes which should
	// we will use to create fake placements for OSD prepare job pods
	osdIDGenerator := newOSDIDGenerator()

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

	// Helper methods to set "completed" status on "starting" ConfigMaps.
	setStatusConfigMapToCompleted := func(cm *corev1.ConfigMap) {
		// can't use mockNodeOrchestrationCompleted here b/c it uses a context.CoreV1() method that
		// is mutex locked when a reactor is processing
		status := parseOrchestrationStatus(cm.Data)
		t.Logf("configmap reactor: updating configmap %q status to completed", cm.Name)
		// configmap names are deterministic can be mapped indirectly to an OSD ID, and since the
		// configmaps are used to report completion status of OSD provisioning, we use this property in
		// thse unit tests
		osdID := osdIDGenerator.osdID(t, cm.Name)
		status.Status = OrchestrationStatusCompleted
		status.PvcBackedOSD = true
		status.OSDs = []OSDInfo{
			{
				ID:        osdID,
				UUID:      fmt.Sprintf("%032d", osdID),
				BlockPath: "/dev/path/to/block",
			},
		}
		s, _ := json.Marshal(status)
		cm.Data[orchestrationStatusKey] = string(s)
	}

	createConfigMapWithStatusStartingCallback := func(cm *corev1.ConfigMap) {
		// placeholder to be defined later in tests
	}
	deleteConfigMapWithStatusAlreadyExistsCallback := func(cm *corev1.ConfigMap, action k8stesting.DeleteActionImpl) {
		// placeholder to be defined later in tests
	}

	var cmReactor k8stesting.ReactionFunc = func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		switch action := action.(type) {
		case k8stesting.CreateActionImpl:
			obj := action.GetObject()
			cm, ok := obj.(*corev1.ConfigMap)
			if !ok {
				t.Fatal("err! action not a configmap")
			}

			status := parseOrchestrationStatus(cm.Data)
			if status.Status == OrchestrationStatusStarting {
				// allow tests to specify a custom callback for this case
				createConfigMapWithStatusStartingCallback(cm)
			}

		case k8stesting.DeleteActionImpl:
			// get the CM being deleted to figure out some info about it
			obj, err := clientset.Tracker().Get(action.GetResource(), action.GetNamespace(), action.Name)
			if err != nil {
				t.Fatalf("err! could not get configmap %q", action.Name)
			}
			cm, ok := obj.(*corev1.ConfigMap)
			if !ok {
				t.Fatal("err! action not a configmap")
			}

			status := parseOrchestrationStatus(cm.Data)
			if status.Status == OrchestrationStatusAlreadyExists {
				t.Logf("configmap reactor: delete: OSD for configmap %q was updated", cm.Name)
				// allow tests to specify a custom callback for this case
				deleteConfigMapWithStatusAlreadyExistsCallback(cm, action)
			} else if status.Status == OrchestrationStatusCompleted {
				t.Logf("configmap reactor: delete: OSD for configmap %q was created", cm.Name)
			}
		}

		// modify it in-place and allow it to be created later with these changes
		return false, nil, nil
	}
	clientset.PrependReactor("*", "configmaps", cmReactor)

	deploymentOps := newResourceOperationList()

	// make a very simple reactor to record when deployments were created
	var deploymentReactor k8stesting.ReactionFunc = func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction, ok := action.(k8stesting.CreateAction)
		if !ok {
			t.Fatal("err! action is not a create action")
			return false, nil, nil
		}
		obj := createAction.GetObject()
		d, ok := obj.(*appsv1.Deployment)
		if !ok {
			t.Fatal("err! action not a deployment")
			return false, nil, nil
		}
		if o, _ := clientset.Tracker().Get(action.GetResource(), d.Namespace, d.Name); o != nil {
			// deployment already exists, so this isn't be a valid create
			return false, nil, nil
		}
		t.Logf("creating deployment %q", d.Name)
		deploymentOps.Add(d.Name, "create")
		return false, nil, nil
	}
	clientset.PrependReactor("create", "deployments", deploymentReactor)

	// patch the updateDeploymentAndWait function to always report success and record when
	// deployments are updated
	oldUDAW := updateDeploymentAndWait
	defer func() {
		updateDeploymentAndWait = oldUDAW
	}()
	updateDeploymentAndWait = func(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, deployment *appsv1.Deployment, daemonType string, daemonName string, skipUpgradeChecks bool, continueUpgradeAfterChecksEvenIfNotHealthy bool) error {
		t.Logf("updating deployment %q", deployment.Name)
		deploymentOps.Add(deployment.Name, "update")
		return nil
	}

	// wait for a number of deployments to be updated
	waitForDeploymentOps := func(count int) {
		for {
			if deploymentOps.Len() >= count {
				return
			}
			<-time.After(1 * time.Millisecond)
		}
	}

	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   namespace,
		CephVersion: cephver.Nautilus,
	}
	executor := osdPVCTestExecutor(t, clientset, namespace)

	context := &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: executor, RequestCancelOrchestration: abool.New()}
	storageClassName := "test-storage-class"
	volumeMode := corev1.PersistentVolumeBlock
	spec := cephv1.ClusterSpec{
		CephVersion: cephv1.CephVersionSpec{
			Image: "ceph/ceph:v14.2.2",
		},
		DataDirHostPath: context.ConfigDir,
		Storage: rookv1.StorageScopeSpec{
			StorageClassDeviceSets: []rookv1.StorageClassDeviceSet{
				{
					Name:  "set1",
					Count: 5,
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "data",
							},
							Spec: corev1.PersistentVolumeClaimSpec{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("10Gi"),
									},
								},
								StorageClassName: &storageClassName,
								VolumeMode:       &volumeMode,
								AccessModes: []corev1.PersistentVolumeAccessMode{
									corev1.ReadWriteOnce,
								},
							},
						},
					},
				},
			},
		},
	}

	// =============================================================================================
	t.Log("Step 1: create new PVCs")

	// when creating new configmaps with status "starting", simulate them becoming "completed"
	// before any opportunistic updates can happen by changing the status to "completed" before
	// the config map gets created by the fake k8s clientset.
	createConfigMapWithStatusStartingCallback = func(cm *corev1.ConfigMap) {
		setStatusConfigMapToCompleted(cm)
	}
	deleteConfigMapWithStatusAlreadyExistsCallback = func(cm *corev1.ConfigMap, action k8stesting.DeleteActionImpl) {
		// do nothing on CM deletes
	}

	var c *Cluster
	var provisionDone bool = false
	waitForDone := func() {
		for {
			if provisionDone {
				t.Log("provisioning done")
				break
			}
			t.Log("provisioning not done. waiting...")
			time.Sleep(time.Millisecond)
		}
	}
	run := func() {
		// reset
		deploymentOps = newResourceOperationList()
		statusMapWatcher.Reset()

		// kick off the start of the orchestration in a goroutine so we can watch the results
		// and manipulate confimaps in the test if needed
		c = New(context, clusterInfo, spec, "myversion")
		provisionDone = false
		go func() {
			provisionConfig := c.newProvisionConfig()
			c.startProvisioningOverPVCs(provisionConfig)
			provisionDone = true
		}()
	}
	run()

	numExpectedPVCs := 5
	waitForNumPVCs(t, clientset, namespace, numExpectedPVCs)

	waitForNumDeployments(t, clientset, namespace, numExpectedPVCs)
	// 5 deployments should have been created
	assert.Equal(t, 5, deploymentOps.Len())
	// all 5 should be create operations
	assert.Len(t, deploymentOps.ResourcesWithOperation("create"), 5)
	t.Log("deployments successfully created for new PVCs")

	waitForDone()

	// =============================================================================================
	t.Log("Step 2: verify deployments are updated when run again")
	// clean the create times maps
	reset := func() {
		deploymentOps = newResourceOperationList()
		// fake 'watcher' can close the channel for long tests, so reset when we can
		statusMapWatcher.Reset()
	}
	reset()

	run()

	waitForNumPVCs(t, clientset, namespace, numExpectedPVCs)

	// 5 deployments should have been operated on
	waitForDeploymentOps(numExpectedPVCs)
	// all 5 should be update operations
	updatedDeployments := deploymentOps.ResourcesWithOperation("update")
	assert.Len(t, updatedDeployments, 5)

	// use later to ensure existing deployments are updated
	existingDeployments := updatedDeployments

	waitForDone()

	// =============================================================================================
	t.Log("Step 3: verify new deployments are created before existing ones are updated")
	reset()

	spec.Storage.StorageClassDeviceSets[0].Count = 8
	numExpectedPVCs = 8

	run()

	waitForNumPVCs(t, clientset, namespace, numExpectedPVCs)
	waitForNumDeployments(t, clientset, namespace, numExpectedPVCs)
	waitForDeploymentOps(numExpectedPVCs)

	// the same deployments from before should be updated here also
	updatedDeployments = deploymentOps.ResourcesWithOperation("update")
	assert.Len(t, updatedDeployments, 5)
	assert.ElementsMatch(t, existingDeployments, updatedDeployments)

	createdDeployments := deploymentOps.ResourcesWithOperation("create")
	assert.Len(t, createdDeployments, 3)

	for i, do := range deploymentOps.List() {
		if i < 3 {
			// first 3 ops should be create ops
			assert.Equal(t, "create", do.operation)
		} else {
			// final 5 ops should be update ops
			assert.Equal(t, "update", do.operation)
		}
	}

	existingDeployments = append(createdDeployments, updatedDeployments...)

	waitForDone()

	// =============================================================================================
	t.Log("Step 4: verify updates can happen opportunistically")
	reset()

	spec.Storage.StorageClassDeviceSets[0].Count = 10
	numExpectedPVCs = 10

	// In this test we carefully control the configmaps. When a configmap with status
	// "alreadyExisting" is deleted, we know an OSD deployment just finished updating. We then
	// immediately set one of the configmaps in "starting" state to "completed" so that it should
	// be the next status configmap to be processed; a new OSD should be created for it. We
	// therefore know that the first operation should be an update and the second a create. Then on
	// in update-then-create fashion until all creates are done, followed by all updates.
	configMapsThatNeedUpdatedToCompleted := []string{}
	createConfigMapWithStatusStartingCallback = func(cm *corev1.ConfigMap) {
		t.Logf("configmap reactor: create: marking that configmap %q needs to be completed later", cm.Name)
		configMapsThatNeedUpdatedToCompleted = append(configMapsThatNeedUpdatedToCompleted, cm.Name)
	}
	deleteConfigMapWithStatusAlreadyExistsCallback = func(cm *corev1.ConfigMap, action k8stesting.DeleteActionImpl) {
		if len(configMapsThatNeedUpdatedToCompleted) > 0 {
			cmName := configMapsThatNeedUpdatedToCompleted[0]
			obj, err := clientset.Tracker().Get(action.GetResource(), action.GetNamespace(), cmName)
			if err != nil {
				t.Fatalf("err! could not get configmap %q", cmName)
			}
			cm, ok := obj.(*corev1.ConfigMap)
			if !ok {
				t.Fatal("err! action not a configmap")
			}
			setStatusConfigMapToCompleted(cm)
			err = clientset.Tracker().Update(action.GetResource(), cm, action.GetNamespace())
			if err != nil {
				t.Fatalf("err! failed to update configmap to completed. %v", err)
			}
			statusMapWatcher.Modify(cm) // MUST inform the fake watcher we made a change
			configMapsThatNeedUpdatedToCompleted = configMapsThatNeedUpdatedToCompleted[1:]
		}
	}

	run()

	waitForNumPVCs(t, clientset, namespace, numExpectedPVCs)
	waitForNumDeployments(t, clientset, namespace, numExpectedPVCs)
	waitForDeploymentOps(numExpectedPVCs)

	updatedDeployments = deploymentOps.ResourcesWithOperation("update")
	assert.Len(t, updatedDeployments, 8)
	assert.ElementsMatch(t, existingDeployments, updatedDeployments)

	createdDeployments = deploymentOps.ResourcesWithOperation("create")
	assert.Len(t, createdDeployments, 2)

	assert.Equal(t,
		[]string{
			"update",
			"create",
			"update",
			"create",
			"update",
			"update",
			"update",
			"update",
			"update",
			"update",
		}, deploymentOps.OperationsInOrder())

	existingDeployments = append(createdDeployments, updatedDeployments...)

	waitForDone()

	// =============================================================================================
	t.Log("Step 5: verify opportunistic updates can all happen before creates")
	reset()

	spec.Storage.StorageClassDeviceSets[0].Count = 12
	numExpectedPVCs = 12

	// In this test, we stop all configmaps from being moved from "starting" to "completed" status
	// in the configmap reactor so that all opportunistic updates should happen before new OSDs
	// get created.
	configMapsThatNeedUpdatedToCompleted = []string{}
	createConfigMapWithStatusStartingCallback = func(cm *corev1.ConfigMap) {
		// re-define this behavior as a reminder for readers of the test
		t.Logf("configmap reactor: create: marking that configmap %q needs to be completed later", cm.Name)
		configMapsThatNeedUpdatedToCompleted = append(configMapsThatNeedUpdatedToCompleted, cm.Name)
	}
	deleteConfigMapWithStatusAlreadyExistsCallback = func(cm *corev1.ConfigMap, action k8stesting.DeleteActionImpl) {
		// do NOT automatically move configmaps from "starting" to "completed"
	}

	run()

	waitForDeploymentOps(10) // wait for 10 updates

	updatedDeployments = deploymentOps.ResourcesWithOperation("update")
	assert.Len(t, updatedDeployments, 10)
	assert.ElementsMatch(t, existingDeployments, updatedDeployments)

	// update configmaps from "starting" to "completed"
	for _, cmName := range configMapsThatNeedUpdatedToCompleted {
		cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, cmName, metav1.GetOptions{})
		assert.NoError(t, err)
		setStatusConfigMapToCompleted(cm)
		cm, err = clientset.CoreV1().ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{})
		assert.NoError(t, err)
		statusMapWatcher.Modify(cm) // MUST inform the fake watcher we made a change
	}

	waitForNumPVCs(t, clientset, namespace, numExpectedPVCs)
	waitForNumDeployments(t, clientset, namespace, numExpectedPVCs)
	waitForDeploymentOps(numExpectedPVCs)

	// should be 2 more create operations
	createdDeployments = deploymentOps.ResourcesWithOperation("create")
	assert.Len(t, createdDeployments, 2)

	for i, do := range deploymentOps.List() {
		if i < 10 {
			// first 10 ops should be update ops
			assert.Equal(t, "update", do.operation)
		} else {
			// final 2 ops should be update ops
			assert.Equal(t, "create", do.operation)
		}
	}

	waitForDone()
	t.Log("success")
}

/*
 * mock executor to handle ceph commands
 */

func osdPVCTestExecutor(t *testing.T, clientset *fake.Clientset, namespace string) *exectest.MockExecutor {
	return &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command string, outFileArg string, args ...string) (string, error) {
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
					return "", nil // no need to return text, only output status is used
				}
				if args[1] == "ls" {
					// ceph osd ls returns an array of osd IDs like [0,1,2]
					// build this based on the number of deployments since they should be equal
					// for this test
					l, err := clientset.AppsV1().Deployments(namespace).List(context.TODO(), metav1.ListOptions{})
					if err != nil {
						t.Fatalf("failed to build 'ceph osd ls' output. %v", err)
					}
					num := len(l.Items)
					a := []string{}
					for i := 0; i < num; i++ {
						a = append(a, strconv.Itoa(i))
					}
					return fmt.Sprintf("[%s]", strings.Join(a, ",")), nil
				}
				if args[1] == "tree" {
					return `{"nodes":[{"id":-1,"name":"default","type":"root","type_id":11,"children":[-3]},{"id":-3,"name":"master","type":"host","type_id":1,"pool_weights":{},"children":[2,1,0]},{"id":0,"device_class":"hdd","name":"osd.0","type":"osd","type_id":0,"crush_weight":0.009796142578125,"depth":2,"pool_weights":{},"exists":1,"status":"up","reweight":1,"primary_affinity":1},{"id":1,"device_class":"hdd","name":"osd.1","type":"osd","type_id":0,"crush_weight":0.009796142578125,"depth":2,"pool_weights":{},"exists":1,"status":"up","reweight":1,"primary_affinity":1},{"id":2,"device_class":"hdd","name":"osd.2","type":"osd","type_id":0,"crush_weight":0.009796142578125,"depth":2,"pool_weights":{},"exists":1,"status":"up","reweight":1,"primary_affinity":1}],"stray":[]}`, nil
				}
			}
			if args[0] == "versions" {
				// the update deploy code only cares about the mons from the ceph version command results
				v := `{"mon":{"ceph version 14.2.2 (somehash) nautilus (stable)":3}}`
				return v, nil
			}
			return "", errors.Errorf("unexpected ceph command %q", args)
		},
	}
}

/*
 * basic helper functions
 */

// node names for OSDs on PVC end up being the name of the PVC
func waitForNumPVCs(t *testing.T, clientset *fake.Clientset, namespace string, count int) {
	for {
		l, err := clientset.CoreV1().PersistentVolumeClaims(namespace).List(context.TODO(), metav1.ListOptions{})
		assert.NoError(t, err)
		if len(l.Items) >= count {
			t.Log("PVCs for OSDs on PVC all exist")
			break
		}
		<-time.After(1 * time.Millisecond)
	}
}

func waitForNumDeployments(t *testing.T, clientset *fake.Clientset, namespace string, count int) {
	for {
		l, err := clientset.AppsV1().Deployments(namespace).List(context.TODO(), metav1.ListOptions{})
		assert.NoError(t, err)
		if len(l.Items) >= count {
			t.Log("Deployments for OSDs on PVC all exist")
			break
		}
		<-time.After(1 * time.Millisecond)
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

/*
 * resourceOperationList
 * We want to keep track of the order in which some resources (notably deployments) are created and
 * updated, and the tracker should be thread safe since there could be operations occurring in
 * parallel.
 */

type resourceOperation struct {
	resourceName string
	operation    string // e.g., "create", "update"
}

func newResourceOperation(resourceName, operation string) resourceOperation {
	return resourceOperation{resourceName, operation}
}

type resourceOperationList struct {
	sync.Mutex
	resourceOps []resourceOperation
}

func newResourceOperationList() *resourceOperationList {
	return &resourceOperationList{
		sync.Mutex{},
		[]resourceOperation{},
	}
}

func (r *resourceOperationList) Add(resourceName, operation string) {
	r.Lock()
	defer r.Unlock()
	r.resourceOps = append(r.resourceOps, newResourceOperation(resourceName, operation))
}

func (r *resourceOperationList) Len() int {
	r.Lock()
	defer r.Unlock()
	return len(r.resourceOps)
}

func (r *resourceOperationList) List() []resourceOperation {
	return r.resourceOps
}

// Return only the resources which have a given operation
func (r *resourceOperationList) ResourcesWithOperation(operation string) []string {
	resources := []string{}
	for _, ro := range r.List() {
		if ro.operation == operation {
			resources = append(resources, ro.resourceName)
		}
	}
	return resources
}

// Return all operations in order without resource names
func (r *resourceOperationList) OperationsInOrder() []string {
	ops := []string{}
	for _, ro := range r.List() {
		ops = append(ops, ro.operation)
	}
	return ops
}
