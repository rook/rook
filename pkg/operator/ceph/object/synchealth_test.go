/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package object

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/rook/rook/pkg/util/exec"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	syncCheckerZone      = "zone-a"
	syncCheckerPeerZone  = "zone-b"
	syncCheckerZoneGroup = "zonegroup-a"
	syncCheckerRealm     = "realm-a"

	syncCheckerZoneID     = "6cb39d2c-3005-49da-9be3-c1a92a97d28a"
	syncCheckerPeerZoneID = "9c1b1d0a-1a8c-4a1a-9c1a-1a8c4a1a9c1b"
)

func syncCheckerZoneGroupJSON(masterZoneID string) string {
	return fmt.Sprintf(`{
		"id": "fd8ff110-d3fd-49b4-b24f-f6cd3dddfedf",
		"name": %q,
		"is_master": true,
		"master_zone": %q,
		"zones": [
			{"id": %q, "name": %q, "endpoints": ["http://rook-ceph-rgw-my-store.rook-ceph.svc:80"]},
			{"id": %q, "name": %q, "endpoints": ["http://rook-ceph-rgw-my-store.rook-ceph-secondary.svc:80"]}
		],
		"realm_id": "237e6250-5f7d-4b85-9359-8cb2b1848507"
	}`, syncCheckerZoneGroup, masterZoneID, syncCheckerZoneID, syncCheckerZone, syncCheckerPeerZoneID, syncCheckerPeerZone)
}

func syncStatusJSON(status string) string {
	return fmt.Sprintf(`{
		"sync_status": {
			"info": {"status": %q, "num_shards": 64, "period": "df665ecb-1762-47a9-9c66-f938d251c02a"},
			"markers": []
		},
		"full_sync": {"total": 0, "complete": 0}
	}`, status)
}

func syncStatusENOENT(t *testing.T) (string, error) {
	message := "ERROR: sync.read_sync_status() returned ret=-2"
	return message, exectest.MockExecCommandReturns(t, "", message, int(syscall.ENOENT))
}

func syncStatusTimeout() (string, error) {
	return "", errors.Errorf("%s the command radosgw-admin to return", exec.TimeoutWaitingForMessage)
}

func syncStatusProxyNotFound() (string, error) {
	return "", kerrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "rook-ceph-mgr-a")
}

// syncCheckerCalls counts the radosgw-admin commands a test's checker ran
type syncCheckerCalls struct {
	metadataStatus int
	dataStatus     map[string]int
	periodPull     int
}

type syncCheckerFixture struct {
	checker   *multisiteSyncChecker
	recorder  *events.FakeRecorder
	client    client.Client
	calls     *syncCheckerCalls
	restarted *[]string
}

// tick runs the given number of health check iterations
func (f *syncCheckerFixture) tick(t *testing.T, count int) {
	t.Helper()
	for range count {
		f.checker.checkSyncStatus(context.TODO())
	}
}

// recoveryAnnotation returns the recovery annotation the restart patch left on the gateway
// deployment's pod template
func (f *syncCheckerFixture) recoveryAnnotation(t *testing.T) string {
	t.Helper()
	deployment, err := f.checker.context.Clientset.AppsV1().Deployments(namespace).Get(
		context.TODO(), fmt.Sprintf("%s-%s-a", AppName, store), metav1.GetOptions{})
	require.NoError(t, err)
	return deployment.Spec.Template.Annotations[multisiteSyncRecoveryAnnotation]
}

func (f *syncCheckerFixture) events() []string {
	collected := []string{}
	for {
		select {
		case event := <-f.recorder.Events:
			collected = append(collected, event)
		default:
			return collected
		}
	}
}

func (f *syncCheckerFixture) condition(t *testing.T) *cephv1.Condition {
	t.Helper()
	objectStore := &cephv1.CephObjectStore{}
	require.NoError(t, f.client.Get(context.TODO(), f.checker.namespacedName, objectStore))
	require.NotNil(t, objectStore.Status)
	return cephv1.FindStatusCondition(objectStore.Status.Conditions, cephv1.ConditionMultisiteSyncHealthy)
}

// syncCheckerResponses lets each test control what the store's zone reports
type syncCheckerResponses struct {
	zoneIsMaster  bool
	metadataSync  func() (string, error)
	dataSync      func(peer string) (string, error)
	skipReconcile bool
}

func newSyncCheckerFixture(t *testing.T, responses syncCheckerResponses) *syncCheckerFixture {
	t.Helper()

	ctx := context.TODO()
	calls := &syncCheckerCalls{dataStatus: map[string]int{}}

	masterZoneID := syncCheckerPeerZoneID
	if responses.zoneIsMaster {
		masterZoneID = syncCheckerZoneID
	}

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			switch {
			case args[0] == "zonegroup" && args[1] == "get":
				return syncCheckerZoneGroupJSON(masterZoneID), nil
			case args[0] == "zone" && args[1] == "get":
				return fmt.Sprintf(`{"id": %q, "name": %q}`, syncCheckerZoneID, syncCheckerZone), nil
			case args[0] == "metadata" && args[1] == "sync" && args[2] == "status":
				calls.metadataStatus++
				return responses.metadataSync()
			case args[0] == "data" && args[1] == "sync" && args[2] == "status":
				peer := sourceZoneArg(t, args)
				calls.dataStatus[peer]++
				return responses.dataSync(peer)
			case args[0] == "period" && args[1] == "pull":
				calls.periodPull++
				return "", nil
			}
			t.Fatalf("unhandled command: %s %v", command, args)
			panic("unhandled command")
		},
	}

	objectStore := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: store, Namespace: namespace},
		TypeMeta:   metav1.TypeMeta{Kind: "CephObjectStore"},
		Spec: cephv1.ObjectStoreSpec{
			Gateway: cephv1.GatewaySpec{Port: 80, Instances: 1},
			Zone:    cephv1.ZoneSpec{Name: syncCheckerZone},
		},
		Status: &cephv1.ObjectStoreStatus{Phase: cephv1.ConditionReady},
	}

	objectZone := &cephv1.CephObjectZone{
		ObjectMeta: metav1.ObjectMeta{Name: syncCheckerZone, Namespace: namespace},
		Spec:       cephv1.ObjectZoneSpec{ZoneGroup: syncCheckerZoneGroup},
	}
	objectZoneGroup := &cephv1.CephObjectZoneGroup{
		ObjectMeta: metav1.ObjectMeta{Name: syncCheckerZoneGroup, Namespace: namespace},
		Spec:       cephv1.ObjectZoneGroupSpec{Realm: syncCheckerRealm},
	}
	objectRealm := &cephv1.CephObjectRealm{
		ObjectMeta: metav1.ObjectMeta{Name: syncCheckerRealm, Namespace: namespace},
	}

	clientset := test.New(t, 1)
	deploymentLabels := getLabels(store, namespace, true)
	if responses.skipReconcile {
		deploymentLabels[cephv1.SkipReconcileLabelKey] = "true"
	}
	_, err := clientset.AppsV1().Deployments(namespace).Create(ctx, &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-a", AppName, store),
			Namespace: namespace,
			Labels:    deploymentLabels,
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStore{}, &cephv1.CephObjectStoreList{})
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objectStore).Build()

	recorder := events.NewFakeRecorder(50)
	checker := newMultisiteSyncChecker(
		&clusterd.Context{
			Executor:      executor,
			Clientset:     clientset,
			RookClientset: rookfake.NewSimpleClientset(objectRealm, objectZoneGroup, objectZone, objectStore),
		},
		cl,
		recorder,
		&cephclient.ClusterInfo{Namespace: namespace, Context: ctx},
		&cephv1.ClusterSpec{},
		objectStore,
		types.NamespacedName{Namespace: namespace, Name: store},
	)

	waitForDeploymentToStartOrig := waitForDeploymentToStart
	t.Cleanup(func() { waitForDeploymentToStart = waitForDeploymentToStartOrig })
	restarted := &[]string{}
	waitForDeploymentToStart = func(_ context.Context, _ *clusterd.Context, deployment *apps.Deployment) error {
		*restarted = append(*restarted, deployment.Name)
		return nil
	}

	return &syncCheckerFixture{
		checker:   checker,
		recorder:  recorder,
		client:    cl,
		calls:     calls,
		restarted: restarted,
	}
}

func sourceZoneArg(t *testing.T, args []string) string {
	t.Helper()
	for _, arg := range args {
		if strings.HasPrefix(arg, "--source-zone=") {
			return strings.TrimPrefix(arg, "--source-zone=")
		}
	}
	t.Fatalf("no --source-zone argument in %v", args)
	return ""
}

func countEvents(recorded []string, eventType, reason string) int {
	count := 0
	for _, event := range recorded {
		if strings.HasPrefix(event, eventType+" "+reason+" ") {
			count++
		}
	}
	return count
}

func healthySyncStatus() (string, error) {
	return syncStatusJSON("sync"), nil
}

func initSyncStatus() (string, error) {
	return syncStatusJSON(syncStatusInit), nil
}

func TestShouldCheckMultisiteSync(t *testing.T) {
	multisiteStore := func() *cephv1.CephObjectStore {
		return &cephv1.CephObjectStore{
			Spec: cephv1.ObjectStoreSpec{Zone: cephv1.ZoneSpec{Name: syncCheckerZone}},
		}
	}

	t.Run("multisite store", func(t *testing.T) {
		assert.True(t, shouldCheckMultisiteSync(multisiteStore()))
	})

	t.Run("store without a zone", func(t *testing.T) {
		assert.False(t, shouldCheckMultisiteSync(&cephv1.CephObjectStore{}))
	})

	t.Run("external store", func(t *testing.T) {
		objectStore := multisiteStore()
		objectStore.Spec.Gateway.ExternalRgwEndpoints = []cephv1.EndpointAddress{{IP: "192.168.0.1"}}
		assert.False(t, shouldCheckMultisiteSync(objectStore))
	})

	t.Run("store with multisite sync traffic disabled", func(t *testing.T) {
		objectStore := multisiteStore()
		objectStore.Spec.Gateway.DisableMultisiteSyncTraffic = true
		assert.False(t, shouldCheckMultisiteSync(objectStore))
	})
}

func TestMultisiteSyncCheckerHealthy(t *testing.T) {
	f := newSyncCheckerFixture(t, syncCheckerResponses{
		metadataSync: healthySyncStatus,
		dataSync:     func(string) (string, error) { return healthySyncStatus() },
	})

	f.tick(t, 5)

	assert.Empty(t, *f.restarted)
	assert.Equal(t, 0, f.calls.periodPull)
	assert.Empty(t, f.events())
	assert.Empty(t, f.checker.streaks)
	assert.Equal(t, 5, f.calls.metadataStatus)
	assert.Equal(t, 5, f.calls.dataStatus[syncCheckerPeerZone])
}

func TestMultisiteSyncCheckerWedgedMetadata(t *testing.T) {
	f := newSyncCheckerFixture(t, syncCheckerResponses{
		metadataSync: func() (string, error) { return syncStatusENOENT(t) },
		dataSync:     func(string) (string, error) { return healthySyncStatus() },
	})

	f.tick(t, 2)
	assert.Empty(t, *f.restarted)
	assert.Equal(t, 2, f.checker.streaks[syncSignal{metadata: true}.key()])

	f.tick(t, 1)
	assert.Equal(t, 1, f.calls.periodPull)
	require.Len(t, *f.restarted, 1)
	assert.NotEmpty(t, f.recoveryAnnotation(t))
	assert.Empty(t, f.checker.streaks)

	recoveryEvents := f.events()
	require.Len(t, recoveryEvents, 1)
	assert.Contains(t, recoveryEvents[0], corev1.EventTypeNormal+" "+multisiteSyncRecoveryEvent)
	assert.Contains(t, recoveryEvents[0], "metadata sync")
}

func TestMultisiteSyncCheckerWedgedDataSync(t *testing.T) {
	f := newSyncCheckerFixture(t, syncCheckerResponses{
		metadataSync: healthySyncStatus,
		dataSync:     func(string) (string, error) { return initSyncStatus() },
	})

	f.tick(t, 3)

	require.Len(t, *f.restarted, 1)
	assert.Equal(t, 0, f.calls.periodPull, "data sync recovery must not pull the period")

	recoveryEvents := f.events()
	require.Len(t, recoveryEvents, 1)
	assert.Contains(t, recoveryEvents[0], fmt.Sprintf("data sync from zone %q", syncCheckerPeerZone))
}

func TestMultisiteSyncCheckerTransientInit(t *testing.T) {
	wedged := true
	f := newSyncCheckerFixture(t, syncCheckerResponses{
		metadataSync: func() (string, error) {
			if wedged {
				return initSyncStatus()
			}
			return healthySyncStatus()
		},
		dataSync: func(string) (string, error) { return healthySyncStatus() },
	})

	f.tick(t, 2)
	wedged = false
	f.tick(t, 2)

	assert.Empty(t, *f.restarted)
	assert.Equal(t, 0, f.calls.periodPull)
	assert.Empty(t, f.checker.streaks)
	assert.Empty(t, f.events())
}

func TestMultisiteSyncCheckerUnknownProbes(t *testing.T) {
	tests := []struct {
		name     string
		response func() (string, error)
	}{
		{"command timed out", syncStatusTimeout},
		{"command proxy not found", syncStatusProxyNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newSyncCheckerFixture(t, syncCheckerResponses{
				metadataSync: tc.response,
				dataSync:     func(string) (string, error) { return healthySyncStatus() },
			})

			f.tick(t, 5)

			assert.Empty(t, f.checker.streaks)
			assert.Empty(t, *f.restarted)
			assert.Equal(t, 0, f.calls.periodPull)
			assert.Empty(t, f.events())
		})
	}
}

func TestMultisiteSyncCheckerRecoveryBudget(t *testing.T) {
	wedged := true
	f := newSyncCheckerFixture(t, syncCheckerResponses{
		metadataSync: func() (string, error) {
			if wedged {
				return initSyncStatus()
			}
			return healthySyncStatus()
		},
		dataSync: func(string) (string, error) { return healthySyncStatus() },
	})

	f.tick(t, 3*maxRecoveriesPerSignal)
	require.Len(t, *f.restarted, maxRecoveriesPerSignal)
	assert.Nil(t, f.condition(t))

	f.tick(t, 3)
	assert.Len(t, *f.restarted, maxRecoveriesPerSignal, "the budget is exhausted")

	wedgedCondition := f.condition(t)
	require.NotNil(t, wedgedCondition)
	assert.Equal(t, corev1.ConditionFalse, wedgedCondition.Status)
	assert.Equal(t, cephv1.MultisiteSyncWedgedReason, wedgedCondition.Reason)
	assert.Equal(t, 1, countEvents(f.events(), corev1.EventTypeWarning, string(cephv1.MultisiteSyncWedgedReason)))

	f.tick(t, 3)
	assert.Len(t, *f.restarted, maxRecoveriesPerSignal)
	assert.Equal(t, 0, countEvents(f.events(), corev1.EventTypeWarning, string(cephv1.MultisiteSyncWedgedReason)),
		"the wedge is only reported once")

	wedged = false
	f.tick(t, 1)

	healthyCondition := f.condition(t)
	require.NotNil(t, healthyCondition)
	assert.Equal(t, corev1.ConditionTrue, healthyCondition.Status)
	assert.Equal(t, cephv1.MultisiteSyncHealthyReason, healthyCondition.Reason)
	assert.Empty(t, f.checker.recoveries)

	wedged = true
	f.tick(t, 3)
	assert.Len(t, *f.restarted, maxRecoveriesPerSignal+1, "the budget is reset once sync is healthy again")
}

func TestMultisiteSyncCheckerBothSignalsWedged(t *testing.T) {
	f := newSyncCheckerFixture(t, syncCheckerResponses{
		metadataSync: func() (string, error) { return syncStatusENOENT(t) },
		dataSync:     func(string) (string, error) { return initSyncStatus() },
	})

	f.tick(t, 3)

	require.Len(t, *f.restarted, 1, "signals wedging in the same probe cycle share one restart")
	assert.Equal(t, 1, f.calls.periodPull)
	recoveryEvents := f.events()
	require.Len(t, recoveryEvents, 1)
	assert.Contains(t, recoveryEvents[0], "metadata sync")
	assert.Contains(t, recoveryEvents[0], fmt.Sprintf("data sync from zone %q", syncCheckerPeerZone))

	// the shared restart was charged to both signals, so both budgets exhaust together
	f.tick(t, 3)
	require.Len(t, *f.restarted, 2)
	f.tick(t, 3)
	assert.Len(t, *f.restarted, 2, "both budgets are spent")
	wedgedCondition := f.condition(t)
	require.NotNil(t, wedgedCondition)
	assert.Equal(t, corev1.ConditionFalse, wedgedCondition.Status)
}

func TestClassifySyncStatus(t *testing.T) {
	enoent := func(message string) error {
		return exectest.MockExecCommandReturns(t, "", message, int(syscall.ENOENT))
	}

	tests := []struct {
		name   string
		output string
		err    error
		want   syncProbeResult
	}{
		{"initialized and syncing", syncStatusJSON("sync"), nil, syncHealthy},
		{"building full sync maps", syncStatusJSON("building-full-sync-maps"), nil, syncHealthy},
		{"stuck in init", syncStatusJSON("init"), nil, syncWedged},
		{"metadata sync status ENOENT", "ERROR: sync.read_sync_status() returned ret=-2", enoent("ERROR: sync.read_sync_status() returned ret=-2"), syncWedged},
		{"sync status summary ENOENT", "failed to read sync status: (2) No such file or directory", enoent("failed to read sync status: (2) No such file or directory"), syncWedged},
		{"bare ENOENT exit", "", enoent(""), syncWedged},
		{"other error", "some other failure", errors.New("exit status 1"), syncUnknown},
		{"timeout", "", errors.Errorf("%s the command radosgw-admin to return", exec.TimeoutWaitingForMessage), syncUnknown},
		{"unparsable output", "not json", nil, syncUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, classifySyncStatus(tc.output, tc.err))
		})
	}
}

func TestMultisiteSyncCheckerMasterZone(t *testing.T) {
	f := newSyncCheckerFixture(t, syncCheckerResponses{
		zoneIsMaster: true,
		metadataSync: func() (string, error) {
			t.Fatal("the master zone must not be probed for metadata sync")
			return "", nil
		},
		dataSync: func(string) (string, error) { return initSyncStatus() },
	})

	f.tick(t, 3)

	assert.Equal(t, 0, f.calls.metadataStatus)
	assert.Equal(t, 3, f.calls.dataStatus[syncCheckerPeerZone])
	require.Len(t, *f.restarted, 1)
	assert.Equal(t, 0, f.calls.periodPull)
}

func TestMultisiteSyncCheckerSkipReconcile(t *testing.T) {
	f := newSyncCheckerFixture(t, syncCheckerResponses{
		skipReconcile: true,
		metadataSync:  func() (string, error) { return syncStatusENOENT(t) },
		dataSync:      func(string) (string, error) { return healthySyncStatus() },
	})

	f.tick(t, 3)

	assert.Empty(t, *f.restarted)
	assert.Equal(t, 1, countEvents(f.events(), corev1.EventTypeNormal, multisiteSyncRecoverySkippedEvent))
}

func TestMultisiteSyncCheckerLifecycle(t *testing.T) {
	ctx := context.TODO()
	probes := atomic.Int32{}

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			if args[0] == "zonegroup" && args[1] == "get" {
				return syncCheckerZoneGroupJSON(syncCheckerZoneID), nil
			}
			// exactly one zone get per health check iteration
			if args[0] == "zone" && args[1] == "get" {
				probes.Add(1)
				return fmt.Sprintf(`{"id": %q, "name": %q}`, syncCheckerZoneID, syncCheckerZone), nil
			}
			return syncStatusJSON("sync"), nil
		},
	}

	objectStore := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: store, Namespace: namespace},
		Spec: cephv1.ObjectStoreSpec{
			Gateway: cephv1.GatewaySpec{Port: 80},
			Zone:    cephv1.ZoneSpec{Name: syncCheckerZone},
		},
		Status: &cephv1.ObjectStoreStatus{Phase: cephv1.ConditionReady},
	}
	objectZone := &cephv1.CephObjectZone{
		ObjectMeta: metav1.ObjectMeta{Name: syncCheckerZone, Namespace: namespace},
		Spec:       cephv1.ObjectZoneSpec{ZoneGroup: syncCheckerZoneGroup},
	}
	objectZoneGroup := &cephv1.CephObjectZoneGroup{
		ObjectMeta: metav1.ObjectMeta{Name: syncCheckerZoneGroup, Namespace: namespace},
		Spec:       cephv1.ObjectZoneGroupSpec{Realm: syncCheckerRealm},
	}
	objectRealm := &cephv1.CephObjectRealm{
		ObjectMeta: metav1.ObjectMeta{Name: syncCheckerRealm, Namespace: namespace},
	}

	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStore{}, &cephv1.CephObjectStoreList{})

	r := &ReconcileCephObjectStore{
		client: fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objectStore).Build(),
		context: &clusterd.Context{
			Executor:      executor,
			Clientset:     test.New(t, 1),
			RookClientset: rookfake.NewSimpleClientset(objectRealm, objectZoneGroup, objectZone, objectStore),
		},
		clusterInfo:      &cephclient.ClusterInfo{Namespace: namespace, Context: ctx},
		clusterSpec:      &cephv1.ClusterSpec{},
		recorder:         events.NewFakeRecorder(50),
		opManagerContext: ctx,
		storeContexts:    map[string]*objectStoreHealth{},
	}
	namespacedName := types.NamespacedName{Namespace: namespace, Name: store}

	r.startMultisiteSyncChecker(objectStore, namespacedName)
	r.startMultisiteSyncChecker(objectStore, namespacedName)

	storeContext := r.storeContexts[storeContextKeyName(objectStore)]
	require.NotNil(t, storeContext)
	assert.True(t, storeContext.started)
	require.Eventually(t, func() bool { return probes.Load() >= 1 }, 5*time.Second, 10*time.Millisecond)
	assert.Never(t, func() bool { return probes.Load() > 1 }, 200*time.Millisecond, 20*time.Millisecond)

	internalCtx := storeContext.internalCtx
	require.NoError(t, internalCtx.Err())

	r.cancelMultisiteSyncChecker(objectStore)
	assert.Empty(t, r.storeContexts)
	assert.Error(t, internalCtx.Err())

	r.cancelMultisiteSyncChecker(objectStore)
}
