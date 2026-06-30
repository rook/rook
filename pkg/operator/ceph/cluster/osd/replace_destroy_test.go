/*
Copyright 2025 The Rook Authors. All rights reserved.

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
	"strings"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// replaceTestState is the durable Ceph state a fake cluster reports, plus a record of the mutating
// ceph commands the teardown issues. It lets a test assert both the action taken and that the
// teardown reads only durable state.
type replaceTestState struct {
	// tree maps osd id -> status ("up", "destroyed", ...) as reported by `osd tree`.
	tree map[int]string
	// inByID maps osd id -> in status (1 = in, 0 = out) as reported by `osd dump`.
	inByID map[int]int
	// safeToDestroy is the set of ids `osd safe-to-destroy` reports as safe.
	safeToDestroy map[int]bool

	cephCmds []string // recorded "osd out", "osd destroy 5", etc.
}

func osdDumpJSON(inByID map[int]int) string {
	osds := ""
	for id, in := range inByID {
		if osds != "" {
			osds += ","
		}
		osds += fmt.Sprintf(`{"osd":%d,"up":0,"in":%d}`, id, in)
	}
	return fmt.Sprintf(`{"osds":[%s]}`, osds)
}

// newReplaceHealthMonitor wires an OSDHealthMonitor to a mock executor backed by st.
func newReplaceHealthMonitor(t *testing.T, clientset *fake.Clientset, st *replaceTestState) *OSDHealthMonitor {
	t.Helper()
	clusterInfo := cephclient.AdminTestClusterInfo("rook-ceph")
	clusterInfo.Context = context.TODO()
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "osd" && args[1] == "tree" {
				return osdTreeJSON(st.tree), nil
			}
			if len(args) >= 2 && args[0] == "osd" && args[1] == "dump" {
				return osdDumpJSON(st.inByID), nil
			}
			if len(args) >= 3 && args[0] == "osd" && args[1] == "safe-to-destroy" {
				id, _ := strconv.Atoi(args[2])
				if st.safeToDestroy[id] {
					return fmt.Sprintf(`{"safe_to_destroy":[%d]}`, id), nil
				}
				return `{"safe_to_destroy":[]}`, nil
			}
			// record mutating commands: osd out/in/down/destroy. Only the meaningful leading args
			// are recorded; the ceph command appends global flags (--cluster, --conf, ...).
			if len(args) >= 2 && args[0] == "osd" {
				switch args[1] {
				case "out", "in", "down":
					st.cephCmds = append(st.cephCmds, strings.Join(args[:3], " "))
				case "destroy":
					st.cephCmds = append(st.cephCmds, strings.Join(args[:4], " "))
				}
			}
			return "", nil
		},
	}
	ctx := &clusterd.Context{Executor: executor, Clientset: clientset}
	return NewOSDHealthMonitor(ctx, clusterInfo, false, cephv1.CephClusterHealthCheckSpec{}, cephv1.ClusterSpec{}, "rook/ceph:test")
}

func getReplaceDep(t *testing.T, m *OSDHealthMonitor, osdID int) *appsv1.Deployment {
	t.Helper()
	d, err := m.context.Clientset.AppsV1().Deployments("rook-ceph").Get(context.TODO(), fmt.Sprintf(osdAppNameFmt, osdID), metav1.GetOptions{})
	require.NoError(t, err)
	return d
}

// replaceMarkedDep builds an OSD deployment that carries the replace annotation and the
// do-not-reconcile label, the normal starting state for the goroutine. It includes the minimal "osd"
// container so the up-front encryption derivation can read OSD info, mirroring a real deployment.
func replaceMarkedDep(osdID int) *appsv1.Deployment {
	return withOSDContainer(osdDeployment(osdID,
		map[string]string{cephv1.ReplaceOSDAnnotationKey: fmt.Sprintf(cephv1.ReplaceOSDAnnotationValueFmt, osdID)},
		map[string]string{cephv1.SkipReconcileLabelKey: "true"}), "node-1", false)
}

// withOSDContainer adds the minimal "osd" container env that getOSDInfo reads, so encryption and
// node-name detection work in unit tests. node is the node the OSD runs on; encrypted controls the
// dmcrypt block path.
func withOSDContainer(d *appsv1.Deployment, node string, isEncrypted bool) *appsv1.Deployment {
	blockPath := "/dev/vg/osd-block-foo"
	if isEncrypted {
		blockPath = "/dev/mapper/foo-block-dmcrypt"
		if d.Labels == nil {
			d.Labels = map[string]string{}
		}
		d.Labels[encrypted] = "true"
	}
	d.Spec.Template.Spec.Containers = []corev1.Container{{
		Name: "osd",
		Env: []corev1.EnvVar{
			{Name: "ROOK_NODE_NAME", Value: node},
			{Name: "ROOK_OSD_UUID", Value: "00000000-0000-0000-0000-000000000000"},
			{Name: "ROOK_BLOCK_PATH", Value: blockPath},
			{Name: "ROOK_CV_MODE", Value: "lvm"},
		},
	}}
	return d
}

// advanceFromState reads the durable Ceph state from the fake and runs processOSDReplacementDestroy
// on the given deployment, mirroring one tick of processOSDsDestroyForReplacement for one OSD.
func advanceFromState(t *testing.T, m *OSDHealthMonitor, st *replaceTestState, d *appsv1.Deployment, osdID int) error {
	t.Helper()
	return m.processOSDReplacementDestroy(d, osdID, treeFromState(t, m, st), dumpFromState(t, m, st))
}

func TestProcessOSDReplacementDestroy(t *testing.T) {
	osdID := 5
	t.Run("not selected without the do-not-reconcile label", func(t *testing.T) {
		dep := osdDeployment(osdID, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-5"}, nil)
		st := &replaceTestState{tree: map[int]string{osdID: "up"}, inByID: map[int]int{osdID: 1}}
		m := newReplaceHealthMonitor(t, fake.NewClientset(dep), st)
		// A deployment without the do-not-reconcile label is not selected by the goroutine at all.
		replacing, err := m.processOSDsDestroyForReplacement()
		require.NoError(t, err)
		assert.NotContains(t, replacing, osdID)
		assert.Empty(t, st.cephCmds)
	})

	t.Run("in -> osd out, no fall-through to destroy", func(t *testing.T) {
		dep := replaceMarkedDep(osdID)
		st := &replaceTestState{tree: map[int]string{osdID: "up"}, inByID: map[int]int{osdID: 1}, safeToDestroy: map[int]bool{osdID: true}}
		m := newReplaceHealthMonitor(t, fake.NewClientset(dep), st)
		require.NoError(t, advanceFromState(t, m, st, dep, osdID))
		assert.Contains(t, st.cephCmds, fmt.Sprintf("osd out %d", osdID))
		// Even though the fake reports safe-to-destroy, a just-out OSD must NOT be destroyed in the
		// same tick: the out step returns and waits for the real drain.
		assert.NotContains(t, st.cephCmds, fmt.Sprintf("osd destroy osd.%d --yes-i-really-mean-it", osdID))
	})

	t.Run("draining when out but not safe", func(t *testing.T) {
		dep := replaceMarkedDep(osdID)
		st := &replaceTestState{tree: map[int]string{osdID: "up"}, inByID: map[int]int{osdID: 0}, safeToDestroy: map[int]bool{}}
		m := newReplaceHealthMonitor(t, fake.NewClientset(dep), st)
		require.NoError(t, advanceFromState(t, m, st, dep, osdID))
		// not safe -> no destroy commands
		assert.NotContains(t, st.cephCmds, fmt.Sprintf("osd destroy osd.%d --yes-i-really-mean-it", osdID))
	})

	t.Run("out and safe destroys this tick, annotates next tick", func(t *testing.T) {
		// out + safe-to-destroy with the pod already gone: the drain gate and destroy steps collapse
		// into this tick, which ends with osd destroy and returns. The ready-for-swap annotation is a
		// separate transition driven by the destroyed slot on the next tick. Use an OSD container so
		// the (non-encrypted) destroy can complete.
		dep := withOSDContainer(replaceMarkedDep(osdID), "node-1", false)
		zero := int32(0)
		dep.Spec.Replicas = &zero // already scaled down, no pod present
		st := &replaceTestState{tree: map[int]string{osdID: "up"}, inByID: map[int]int{osdID: 0}, safeToDestroy: map[int]bool{osdID: true}}
		m := newReplaceHealthMonitor(t, fake.NewClientset(dep), st)

		// Destroy tick: slot destroyed, not yet annotated.
		require.NoError(t, advanceFromState(t, m, st, getReplaceDep(t, m, osdID), osdID))
		assert.Contains(t, st.cephCmds, fmt.Sprintf("osd destroy osd.%d --yes-i-really-mean-it", osdID))
		assert.NotContains(t, getReplaceDep(t, m, osdID).Annotations, cephv1.ReadyForSwapOSDAnnotationKey,
			"annotate is a separate transition once the slot reads destroyed")

		// Next tick: the slot now reads destroyed in the tree -> annotate ready-for-swap.
		st.tree[osdID] = "destroyed"
		require.NoError(t, advanceFromState(t, m, st, getReplaceDep(t, m, osdID), osdID))
		assert.Contains(t, getReplaceDep(t, m, osdID).Annotations, cephv1.ReadyForSwapOSDAnnotationKey)
	})

	t.Run("destroyed -> annotate ready for swap", func(t *testing.T) {
		dep := replaceMarkedDep(osdID)
		st := &replaceTestState{tree: map[int]string{osdID: "destroyed"}, inByID: map[int]int{osdID: 0}}
		m := newReplaceHealthMonitor(t, fake.NewClientset(dep), st)
		require.NoError(t, advanceFromState(t, m, st, dep, osdID))
		got := getReplaceDep(t, m, osdID)
		assert.Contains(t, got.Annotations, cephv1.ReadyForSwapOSDAnnotationKey)
		// destroyed slot: no destructive ceph commands re-issued.
		assert.NotContains(t, st.cephCmds, fmt.Sprintf("osd destroy osd.%d --yes-i-really-mean-it", osdID))
	})

	t.Run("done when ready for swap present", func(t *testing.T) {
		dep := replaceMarkedDep(osdID)
		dep.Annotations[cephv1.ReadyForSwapOSDAnnotationKey] = "true"
		st := &replaceTestState{tree: map[int]string{osdID: "destroyed"}, inByID: map[int]int{osdID: 0}}
		m := newReplaceHealthMonitor(t, fake.NewClientset(dep), st)
		require.NoError(t, advanceFromState(t, m, st, dep, osdID))
		assert.Empty(t, st.cephCmds)
	})

	t.Run("cancellation before destroy -> osd in + clear do-not-reconcile label", func(t *testing.T) {
		// do-not-reconcile label set + NO replace annotation, slot not destroyed = cancellation
		dep := osdDeployment(osdID, nil, map[string]string{cephv1.SkipReconcileLabelKey: "true"})
		st := &replaceTestState{tree: map[int]string{osdID: "up"}, inByID: map[int]int{osdID: 0}}
		m := newReplaceHealthMonitor(t, fake.NewClientset(dep), st)
		require.NoError(t, advanceFromState(t, m, st, dep, osdID))
		assert.Contains(t, st.cephCmds, fmt.Sprintf("osd in %d", osdID))
		got := getReplaceDep(t, m, osdID)
		assert.NotContains(t, got.Labels, cephv1.SkipReconcileLabelKey)
	})

	t.Run("cancellation deletes in-flight crypto-close job", func(t *testing.T) {
		// Encrypted OSD cancelled after its crypto-close Job was created: do-not-reconcile label set +
		// NO replace annotation, slot not destroyed = cancellation. The lingering Job must be deleted
		// before the fence label is cleared so its `cryptsetup close` cannot race the daemon scaling
		// back up.
		dep := withOSDContainer(osdDeployment(osdID, nil, map[string]string{cephv1.SkipReconcileLabelKey: "true"}), "node-1", true)
		st := &replaceTestState{tree: map[int]string{osdID: "up"}, inByID: map[int]int{osdID: 0}}
		m := newReplaceHealthMonitor(t, fake.NewClientset(dep), st)
		// a crypto-close Job exists from a prior tick before the cancellation.
		require.NoError(t, m.cluster.startCryptCloseJob(osdID, "node-1"))
		_, err := m.context.Clientset.BatchV1().Jobs("rook-ceph").Get(context.TODO(), cryptCloseJobName(osdID), metav1.GetOptions{})
		require.NoError(t, err, "crypto-close job should exist before cancellation")

		require.NoError(t, advanceFromState(t, m, st, dep, osdID))

		// OSD marked back in and fence label cleared.
		assert.Contains(t, st.cephCmds, fmt.Sprintf("osd in %d", osdID))
		got := getReplaceDep(t, m, osdID)
		assert.NotContains(t, got.Labels, cephv1.SkipReconcileLabelKey)
		// the in-flight crypto-close Job is gone.
		_, err = m.context.Clientset.BatchV1().Jobs("rook-ceph").Get(context.TODO(), cryptCloseJobName(osdID), metav1.GetOptions{})
		assert.True(t, kerrors.IsNotFound(err), "crypto-close job must be deleted on cancellation")
	})

	t.Run("cancellation after destroy is not honored", func(t *testing.T) {
		// do-not-reconcile label set + NO replace annotation, but slot already destroyed: must NOT mark in.
		dep := withOSDContainer(osdDeployment(osdID, nil, map[string]string{cephv1.SkipReconcileLabelKey: "true"}), "node-1", false)
		st := &replaceTestState{tree: map[int]string{osdID: "destroyed"}, inByID: map[int]int{osdID: 0}}
		m := newReplaceHealthMonitor(t, fake.NewClientset(dep), st)
		require.NoError(t, advanceFromState(t, m, st, dep, osdID))
		assert.NotContains(t, st.cephCmds, fmt.Sprintf("osd in %d", osdID))
		got := getReplaceDep(t, m, osdID)
		assert.Contains(t, got.Annotations, cephv1.ReadyForSwapOSDAnnotationKey)
	})
}

func treeFromState(t *testing.T, m *OSDHealthMonitor, st *replaceTestState) *cephclient.OsdTree {
	t.Helper()
	tree, err := cephclient.HostTree(m.context, m.clusterInfo)
	require.NoError(t, err)
	return &tree
}

func dumpFromState(t *testing.T, m *OSDHealthMonitor, st *replaceTestState) *cephclient.OSDDump {
	t.Helper()
	dump, err := cephclient.GetOSDDump(m.context, m.clusterInfo)
	require.NoError(t, err)
	return dump
}

// TestReplaceScaleDown covers the scale-to-0 step. Pod-gone is checked separately by the caller (see
// TestReplaceWaitsForPodToTerminate), not by this helper.
func TestReplaceScaleDown(t *testing.T) {
	osdID := 5
	t.Run("scales to zero, idempotent when already zero", func(t *testing.T) {
		dep := replaceMarkedDep(osdID)
		st := &replaceTestState{tree: map[int]string{osdID: "up"}, inByID: map[int]int{osdID: 0}}
		m := newReplaceHealthMonitor(t, fake.NewClientset(dep), st)

		// not yet scaled down -> scales to 0
		d := getReplaceDep(t, m, osdID)
		require.NoError(t, m.scaleDownOSDDeployment(d, osdID))
		assert.Equal(t, int32(0), *getReplaceDep(t, m, osdID).Spec.Replicas)

		// already at 0 -> no-op, no error
		require.NoError(t, m.scaleDownOSDDeployment(getReplaceDep(t, m, osdID), osdID))
		assert.Equal(t, int32(0), *getReplaceDep(t, m, osdID).Spec.Replicas)
	})
}

// TestReplaceWaitsForPodToTerminate verifies the downscaled-but-pod-not-gone wait: the deployment is
// scaled to 0 but the daemon pod is still Running (terminating), so the flow must NOT destroy. A
// terminating pod still holds the data/DB LVs open, so destroying here would be premature.
func TestReplaceWaitsForPodToTerminate(t *testing.T) {
	osdID := 5
	dep := withOSDContainer(replaceMarkedDep(osdID), "node-1", false)
	zero := int32(0)
	dep.Spec.Replicas = &zero
	// a still-Running (terminating) pod for this OSD blocks pod-gone.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-ceph-osd-5-abc",
			Namespace: "rook-ceph",
			Labels:    map[string]string{OsdIdLabelKey: strconv.Itoa(osdID)},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	st := &replaceTestState{tree: map[int]string{osdID: "up"}, inByID: map[int]int{osdID: 0}, safeToDestroy: map[int]bool{osdID: true}}
	m := newReplaceHealthMonitor(t, fake.NewClientset(dep, pod), st)

	require.NoError(t, advanceFromState(t, m, st, getReplaceDep(t, m, osdID), osdID))
	// pod still Running -> not pod-gone -> no destroy this tick.
	assert.NotContains(t, st.cephCmds, fmt.Sprintf("osd destroy osd.%d --yes-i-really-mean-it", osdID))
	assert.NotContains(t, getReplaceDep(t, m, osdID).Annotations, cephv1.ReadyForSwapOSDAnnotationKey)

	// Remove the pod (daemon terminated) and re-run: now pod-gone -> destroy proceeds.
	require.NoError(t, m.context.Clientset.CoreV1().Pods("rook-ceph").Delete(context.TODO(), pod.Name, metav1.DeleteOptions{}))
	require.NoError(t, advanceFromState(t, m, st, getReplaceDep(t, m, osdID), osdID))
	assert.Contains(t, st.cephCmds, fmt.Sprintf("osd destroy osd.%d --yes-i-really-mean-it", osdID))
}

// TestReplaceDestroySteps covers the destroy steps for a non-encrypted OSD reached from out +
// safe-to-destroy with the pod already gone: osd down -> osd destroy (no crypto job). The annotate is
// a separate transition once the slot reads destroyed on the next tick.
func TestReplaceDestroySteps(t *testing.T) {
	osdID := 5
	dep := withOSDContainer(replaceMarkedDep(osdID), "node-1", false)
	zero := int32(0)
	dep.Spec.Replicas = &zero // already scaled down, no pod present
	st := &replaceTestState{tree: map[int]string{osdID: "up"}, inByID: map[int]int{osdID: 0}, safeToDestroy: map[int]bool{osdID: true}}
	m := newReplaceHealthMonitor(t, fake.NewClientset(dep), st)

	require.NoError(t, advanceFromState(t, m, st, getReplaceDep(t, m, osdID), osdID))
	assert.Contains(t, st.cephCmds, fmt.Sprintf("osd down %d", osdID))
	assert.Contains(t, st.cephCmds, fmt.Sprintf("osd destroy osd.%d --yes-i-really-mean-it", osdID))
	assert.NotContains(t, getReplaceDep(t, m, osdID).Annotations, cephv1.ReadyForSwapOSDAnnotationKey,
		"annotate is a separate transition once the slot reads destroyed")

	// Next tick: the slot reads destroyed -> annotate ready-for-swap.
	st.tree[osdID] = "destroyed"
	require.NoError(t, advanceFromState(t, m, st, getReplaceDep(t, m, osdID), osdID))
	assert.Contains(t, getReplaceDep(t, m, osdID).Annotations, cephv1.ReadyForSwapOSDAnnotationKey)
}

// TestReplaceDestroyStepsEncrypted covers the encrypted path: the destroy steps must spawn the
// crypto-close Job and wait for it before running osd down/destroy.
func TestReplaceDestroyStepsEncrypted(t *testing.T) {
	osdID := 5
	dep := withOSDContainer(replaceMarkedDep(osdID), "node-1", true)
	zero := int32(0)
	dep.Spec.Replicas = &zero
	st := &replaceTestState{tree: map[int]string{osdID: "up"}, inByID: map[int]int{osdID: 0}, safeToDestroy: map[int]bool{osdID: true}}
	m := newReplaceHealthMonitor(t, fake.NewClientset(dep), st)

	// First tick: no crypto job yet -> it is created, destroy steps NOT run.
	require.NoError(t, advanceFromState(t, m, st, getReplaceDep(t, m, osdID), osdID))
	assert.NotContains(t, st.cephCmds, fmt.Sprintf("osd destroy osd.%d --yes-i-really-mean-it", osdID))
	assert.NotContains(t, getReplaceDep(t, m, osdID).Annotations, cephv1.ReadyForSwapOSDAnnotationKey, "crypto job not yet done, so not destroyed this tick")
	job, err := m.context.Clientset.BatchV1().Jobs("rook-ceph").Get(context.TODO(), cryptCloseJobName(osdID), metav1.GetOptions{})
	require.NoError(t, err, "crypto-close job should have been created")

	// Mark the job succeeded and re-run: destroy steps proceed and the job is deleted BEFORE destroy.
	job.Status.Succeeded = 1
	_, err = m.context.Clientset.BatchV1().Jobs("rook-ceph").UpdateStatus(context.TODO(), job, metav1.UpdateOptions{})
	require.NoError(t, err)

	require.NoError(t, advanceFromState(t, m, st, getReplaceDep(t, m, osdID), osdID))
	assert.Contains(t, st.cephCmds, fmt.Sprintf("osd down %d", osdID))
	assert.Contains(t, st.cephCmds, fmt.Sprintf("osd destroy osd.%d --yes-i-really-mean-it", osdID))
	assert.NotContains(t, getReplaceDep(t, m, osdID).Annotations, cephv1.ReadyForSwapOSDAnnotationKey,
		"annotate is a separate transition once the slot reads destroyed")
	_, err = m.context.Clientset.BatchV1().Jobs("rook-ceph").Get(context.TODO(), cryptCloseJobName(osdID), metav1.GetOptions{})
	assert.True(t, kerrors.IsNotFound(err), "crypto-close job is deleted on the destroy tick, before/with osd destroy")

	// Next tick: the slot reads destroyed -> pure annotate ready-for-swap; the Job is already gone, so
	// this tick must not touch it.
	st.tree[osdID] = "destroyed"
	require.NoError(t, advanceFromState(t, m, st, getReplaceDep(t, m, osdID), osdID))
	assert.Contains(t, getReplaceDep(t, m, osdID).Annotations, cephv1.ReadyForSwapOSDAnnotationKey, "crypto job succeeded and slot destroyed")
	_, err = m.context.Clientset.BatchV1().Jobs("rook-ceph").Get(context.TODO(), cryptCloseJobName(osdID), metav1.GetOptions{})
	assert.True(t, kerrors.IsNotFound(err), "crypto-close job remains deleted; the destroyed tick does not touch it")
}

// TestReplaceExcludedFromNormalHealth verifies a replacement-marked OSD is reported as under
// replacement so the caller can exclude it from normal health processing.
func TestReplaceExcludedFromNormalHealth(t *testing.T) {
	osdID := 5
	dep := replaceMarkedDep(osdID)
	st := &replaceTestState{tree: map[int]string{osdID: "up"}, inByID: map[int]int{osdID: 1}}
	m := newReplaceHealthMonitor(t, fake.NewClientset(dep), st)

	replacing, err := m.processOSDsDestroyForReplacement()
	require.NoError(t, err)
	assert.Contains(t, replacing, osdID)
}

// TestReplaceExclusionSetCompleteOnFetchFailure verifies the exclusion set is fully populated even
// when the Ceph fetch fails: the set is built over all replacement-marked deployments before any
// Ceph query, so a tree/dump failure that aborts the per-OSD actions still returns every marked OSD.
// Without this, a down+out+safe replacement not yet reached this tick could have its marker deleted
// by removeOSDDeploymentIfSafeToDestroy.
func TestReplaceExclusionSetCompleteOnFetchFailure(t *testing.T) {
	dep5 := replaceMarkedDep(5)
	dep7 := replaceMarkedDep(7)
	clusterInfo := cephclient.AdminTestClusterInfo("rook-ceph")
	clusterInfo.Context = context.TODO()
	// Executor fails the osd tree fetch, so processOSDsDestroyForReplacement aborts the per-OSD actions.
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "osd" && args[1] == "tree" {
				return "", fmt.Errorf("simulated osd tree failure")
			}
			return "", nil
		},
	}
	ctx := &clusterd.Context{Executor: executor, Clientset: fake.NewClientset(dep5, dep7)}
	m := NewOSDHealthMonitor(ctx, clusterInfo, false, cephv1.CephClusterHealthCheckSpec{}, cephv1.ClusterSpec{}, "rook/ceph:test")

	replacing, err := m.processOSDsDestroyForReplacement()
	require.NoError(t, err)
	// Both marked OSDs must be in the exclusion set despite the fetch failure aborting the actions.
	assert.Contains(t, replacing, 5)
	assert.Contains(t, replacing, 7)
}

// TestReplaceMarkerSurvivesNormalHealthPath is the highest-value exclusion test: it drives the full
// checkOSDHealth -> checkOSDDump path against a replacement-marked, down+out, safe-to-destroy OSD with
// removeOSDsIfOUTAndSafeToRemove enabled. Without the exclusion, removeOSDDeploymentIfSafeToDestroy
// would delete the marker deployment (the OSD is down, out, safe-to-destroy, and the fake
// deployment's zero creation timestamp is well past graceTime). The marker MUST survive because the
// replacement processing excludes it from the normal path.
func TestReplaceMarkerSurvivesNormalHealthPath(t *testing.T) {
	osdID := 5
	dep := replaceMarkedDep(osdID)
	// down + out so the normal path would consider it for removal; safe-to-destroy so the grace gate
	// is the only remaining guard (which the zero creation timestamp clears).
	st := &replaceTestState{tree: map[int]string{osdID: "up"}, inByID: map[int]int{osdID: 0}, safeToDestroy: map[int]bool{osdID: true}}
	m := newReplaceHealthMonitor(t, fake.NewClientset(dep), st)
	// enable the dangerous normal-path action.
	m.removeOSDsIfOUTAndSafeToRemove = true

	// Sanity: the marker exists before the tick.
	_ = getReplaceDep(t, m, osdID)

	m.checkOSDHealth()

	// The marker deployment must still exist — it was excluded from removeOSDDeploymentIfSafeToDestroy.
	_, err := m.context.Clientset.AppsV1().Deployments("rook-ceph").Get(context.TODO(), fmt.Sprintf(osdAppNameFmt, osdID), metav1.GetOptions{})
	assert.NoError(t, err, "replacement marker must survive the normal health path")
}

// TestReplaceIdempotentRestartResume verifies the memory-less teardown resumes from durable
// state: a SECOND, freshly-constructed monitor (simulating an operator restart that loses all
// in-memory state) re-reads the shared durable state (clientset + Ceph tree/dump) and takes the
// correct next action — only ever annotating once and issuing no further destroy commands. Using a
// distinct monitor instance is what would catch a regression that smuggled progress into a monitor
// field.
func TestReplaceIdempotentRestartResume(t *testing.T) {
	osdID := 5
	dep := replaceMarkedDep(osdID)
	st := &replaceTestState{tree: map[int]string{osdID: "destroyed"}, inByID: map[int]int{osdID: 0}}
	clientset := fake.NewClientset(dep)

	// First tick on the first monitor: slot destroyed, no ready-for-swap -> annotate.
	m1 := newReplaceHealthMonitor(t, clientset, st)
	_, err := m1.processOSDsDestroyForReplacement()
	require.NoError(t, err)
	assert.Contains(t, getReplaceDep(t, m1, osdID).Annotations, cephv1.ReadyForSwapOSDAnnotationKey)

	// Second tick on a brand-new monitor over the SAME clientset and durable Ceph state, modeling an
	// operator restart: it must read Done from durable state and issue no further ceph commands.
	before := len(st.cephCmds)
	m2 := newReplaceHealthMonitor(t, clientset, st)
	_, err = m2.processOSDsDestroyForReplacement()
	require.NoError(t, err)
	assert.Equal(t, before, len(st.cephCmds), "an already-ready-for-swap replacement issues no ceph commands")
}
