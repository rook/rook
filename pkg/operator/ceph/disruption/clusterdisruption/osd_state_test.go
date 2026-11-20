/*
Copyright 2019 The Rook Authors. All rights reserved.

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
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var testZones = []string{"zone-1", "zone-2", "zone-3"}

func TestNextDrainState(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	earlier := now.Add(-10 * time.Minute)
	planned := drainState{domain: "zone-1", noout: true, lastDisruption: earlier, nooutSetAt: earlier}
	zone1NodeDrain := observations{allDomains: testZones, nodeDrainDomains: []string{"zone-1"}, osdDownDomains: []string{"zone-1"}, downOSDs: []int{1}}
	zone1NodeDrainClean := zone1NodeDrain
	zone1NodeDrainClean.pgClean = true

	testcases := []struct {
		name            string
		prev            drainState
		obs             observations
		expectedState   drainState
		expectedExclude []int
	}{
		{
			name:          "steady state: OSDs up and PGs clean ends the drain",
			prev:          planned,
			obs:           observations{pgClean: true, allDomains: testZones},
			expectedState: drainState{},
		},
		{
			name:          "recovery: OSDs are back but PGs are not clean keeps the drain",
			prev:          planned,
			obs:           observations{allDomains: testZones},
			expectedState: planned,
		},
		{
			name:          "recovery: PGs not clean without a previous drain does not start a drain",
			prev:          drainState{},
			obs:           observations{allDomains: testZones},
			expectedState: drainState{},
		},
		{
			name:          "active disruption: node drain starts a planned drain with noout",
			prev:          drainState{},
			obs:           zone1NodeDrain,
			expectedState: drainState{domain: "zone-1", noout: true, lastDisruption: now, nooutSetAt: now},
		},
		{
			name:          "active disruption: the noout timeout is not restarted for an ongoing drain",
			prev:          planned,
			obs:           zone1NodeDrain,
			expectedState: drainState{domain: "zone-1", noout: true, lastDisruption: now, nooutSetAt: earlier},
		},
		{
			name:          "active disruption: osd failure starts an unplanned drain without noout",
			prev:          drainState{},
			obs:           observations{allDomains: testZones, osdDownDomains: []string{"zone-2"}, downOSDs: []int{2}},
			expectedState: drainState{domain: "zone-2", lastDisruption: now},
		},
		{
			name:          "active disruption: only one failure domain drains when OSDs are down in all of them",
			prev:          drainState{},
			obs:           observations{allDomains: testZones, osdDownDomains: testZones, downOSDs: []int{1, 2, 3}},
			expectedState: drainState{domain: "zone-1", lastDisruption: now},
		},
		{
			name:            "drive failure: OSDs down with clean PGs ends the drain and excludes the down OSDs",
			prev:            planned,
			obs:             observations{pgClean: true, allDomains: testZones, osdDownDomains: []string{"zone-1"}, downOSDs: []int{1}},
			expectedState:   drainState{},
			expectedExclude: []int{1},
		},
		{
			name:          "node drain with clean PGs: wait for the PG status to settle",
			prev:          drainState{},
			obs:           zone1NodeDrainClean,
			expectedState: drainState{lastDisruption: now},
		},
		{
			name:          "node drain with clean PGs: still waiting within the settle time",
			prev:          drainState{lastDisruption: now.Add(-cephStatusSettleTime + time.Second)},
			obs:           zone1NodeDrainClean,
			expectedState: drainState{lastDisruption: now.Add(-cephStatusSettleTime + time.Second)},
		},
		{
			name:            "node drain with clean PGs: exclude the down OSDs after the settle time",
			prev:            drainState{lastDisruption: now.Add(-cephStatusSettleTime)},
			obs:             zone1NodeDrainClean,
			expectedState:   drainState{lastDisruption: now.Add(-cephStatusSettleTime)},
			expectedExclude: []int{1},
		},
		{
			name:          "node drain with clean PGs: an active drain is kept",
			prev:          planned,
			obs:           zone1NodeDrainClean,
			expectedState: planned,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			decision := nextDrainState(tc.prev, tc.obs, now)
			assert.Equal(t, tc.expectedState, decision.state)
			assert.Equal(t, tc.expectedExclude, decision.excludeOSDs)
			assert.NotEmpty(t, decision.reason)
		})
	}
}

// A node that stays cordoned with down OSDs while the PGs are clean must not make the pdbs flap
func TestNextDrainStateIsStableForLongNodeDrain(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	obs := observations{pgClean: true, allDomains: testZones, nodeDrainDomains: []string{"zone-1"}, osdDownDomains: []string{"zone-1"}, downOSDs: []int{1}}
	state := drainState{}
	for i := 0; i < 20; i++ {
		elapsed := time.Duration(i) * 30 * time.Second
		decision := nextDrainState(state, obs, now.Add(elapsed))
		state = decision.state
		assert.Empty(t, state.domain, "no failure domain should be draining after %s", elapsed)
		if elapsed < cephStatusSettleTime {
			assert.Empty(t, decision.excludeOSDs, "down OSDs should not be excluded after %s", elapsed)
		} else {
			assert.Equal(t, []int{1}, decision.excludeOSDs, "down OSDs should be excluded after %s", elapsed)
		}
	}
}

func TestDrainStateConfigMapRoundTrip(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cm := &corev1.ConfigMap{Data: map[string]string{
		// timestamp left behind by a failure domain that is not draining
		"zone-9" + nooutLastSetAtKeySuffix: now.Format(time.RFC3339),
		"unrelated":                        "keep",
	}}
	state := drainState{domain: "zone-1", noout: true, lastDisruption: now, nooutSetAt: now.Add(-time.Minute)}
	state.save(cm)
	assert.Equal(t, map[string]string{
		drainingFailureDomainKey:           "zone-1",
		setNoOut:                           "true",
		drainingFailureDomainDurationKey:   "2026-01-01T12:00:00Z",
		"zone-1" + nooutLastSetAtKeySuffix: "2026-01-01T11:59:00Z",
		"unrelated":                        "keep",
	}, cm.Data)
	assert.Equal(t, state, loadDrainState(namespace, cm.Data))

	drainState{}.save(cm)
	assert.Equal(t, map[string]string{drainingFailureDomainKey: "", setNoOut: "", "unrelated": "keep"}, cm.Data)
	assert.Equal(t, drainState{}, loadDrainState(namespace, cm.Data))

	// invalid timestamps are ignored
	cm.Data[drainingFailureDomainDurationKey] = "not-a-time"
	assert.Equal(t, drainState{}, loadDrainState(namespace, cm.Data))
}

func TestApplyOSDPDBs(t *testing.T) {
	ownedPDB := func(name string) *policyv1.PodDisruptionBudget {
		pdb := &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}, Spec: blockingPDBSpec("host", name)}
		require.NoError(t, k8sutil.NewOwnerInfo(cephCluster, scheme.Scheme).SetControllerReference(pdb))
		return pdb
	}
	pdbNames := func(r *ReconcileClusterDisruption) []string {
		pdbs := &policyv1.PodDisruptionBudgetList{}
		require.NoError(t, r.client.List(context.TODO(), pdbs))
		names := []string{}
		for _, pdb := range pdbs.Items {
			names = append(names, pdb.Name)
		}
		return names
	}
	// blocking pdb of a failure domain type that is no longer used
	stalePDB := ownedPDB("rook-ceph-osd-host-node-a")
	// pdb owned by the cluster that is not an OSD pdb
	otherOwnedPDB := ownedPDB("rook-ceph-mds-myfs")
	// pdb with the OSD prefix that was not created by this controller
	userPDB := &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph-osd-custom", Namespace: namespace}, Spec: blockingPDBSpec("host", "custom")}

	// default pdb without an owner reference, for example recreated by hand or restored from a backup
	unownedDefaultPDB := &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: osdPDBAppName, Namespace: namespace}, Spec: defaultPDBSpec(nil)}

	r := getFakeReconciler(t, cephCluster, stalePDB, otherOwnedPDB, userPDB, unownedDefaultPDB)
	r.context = &controllerconfig.Context{}

	// active drain of zone-1: blocking pdbs for the other zones, no default pdb, stale pdb removed
	drain := drainDecision{state: drainState{domain: "zone-1"}}
	require.NoError(t, r.applyOSDPDBs(namespace, cephCluster, desiredOSDPDBs(drain, "zone", testZones), osdPDBNames("zone", testZones)))
	assert.ElementsMatch(t, []string{"rook-ceph-osd-zone-zone-2", "rook-ceph-osd-zone-zone-3", "rook-ceph-mds-myfs", "rook-ceph-osd-custom"}, pdbNames(r))

	// a blocking pdb of a current failure domain must be deleted even if it lost its owner reference
	blockingPDB := &policyv1.PodDisruptionBudget{}
	require.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Name: "rook-ceph-osd-zone-zone-2", Namespace: namespace}, blockingPDB))
	blockingPDB.OwnerReferences = nil
	require.NoError(t, r.client.Update(context.TODO(), blockingPDB))

	// no drain: default pdb excluding the down OSDs, blocking pdbs removed
	noDrain := drainDecision{excludeOSDs: []int{3, 7}}
	require.NoError(t, r.applyOSDPDBs(namespace, cephCluster, desiredOSDPDBs(noDrain, "zone", testZones), osdPDBNames("zone", testZones)))
	assert.ElementsMatch(t, []string{osdPDBAppName, "rook-ceph-mds-myfs", "rook-ceph-osd-custom"}, pdbNames(r))
	defaultPDB := &policyv1.PodDisruptionBudget{}
	require.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Name: osdPDBAppName, Namespace: namespace}, defaultPDB))
	assert.Equal(t, defaultPDBSpec([]int{3, 7}), defaultPDB.Spec)
	assert.True(t, pdbExcludesOSDs(defaultPDB))

	// the exclusion is removed again when the OSDs are back
	require.NoError(t, r.applyOSDPDBs(namespace, cephCluster, desiredOSDPDBs(drainDecision{}, "zone", testZones), osdPDBNames("zone", testZones)))
	require.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Name: osdPDBAppName, Namespace: namespace}, defaultPDB))
	assert.Equal(t, defaultPDBSpec(nil), defaultPDB.Spec)
	assert.False(t, pdbExcludesOSDs(defaultPDB))
}

// The drain decision and its timestamps must be saved even if noout could not be set in Ceph
func TestReconcilePDBForOSDNooutSetFailure(t *testing.T) {
	configMap := fakePDBConfigMap("")
	r := getFakeReconciler(t, cephCluster, configMap)
	r.maintenanceTimeout = DefaultMaintenanceTimeout
	clusterInfo := getFakeClusterInfo()
	clusterInfo.Context = context.TODO()
	request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}

	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if args[0] == "status" {
			return unHealthyCephStatus, nil
		}
		if args[0] == "osd" && args[1] == "dump" {
			return `{"OSDs": [{"OSD": 3, "Up": 3, "In": 3}]}`, nil
		}
		return "", errors.Errorf("mock failure of ceph command '%v'", args)
	}
	r.context = &controllerconfig.Context{ClusterdContext: &clusterd.Context{Executor: executor}}

	_, err := r.reconcilePDBsForOSDs(clusterInfo, request, configMap, "zone", testZones, []string{"zone-1"}, []string{"zone-1"}, []int{1}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "noout")

	saved := &corev1.ConfigMap{}
	require.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Name: pdbStateMapName, Namespace: namespace}, saved))
	assert.Equal(t, "zone-1", saved.Data[drainingFailureDomainKey])
	assert.Equal(t, "true", saved.Data[setNoOut])
	assert.NotEmpty(t, saved.Data[drainingFailureDomainDurationKey])
	assert.NotEmpty(t, saved.Data["zone-1"+nooutLastSetAtKeySuffix])

	// the blocking pdbs are in place despite the noout failure
	pdbs := &policyv1.PodDisruptionBudgetList{}
	require.NoError(t, r.client.List(context.TODO(), pdbs))
	assert.Len(t, pdbs.Items, 2)
}
