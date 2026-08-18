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

package osd

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func machineTree(t *testing.T, jsonStr string) *cephclient.OsdTree {
	t.Helper()
	var tree cephclient.OsdTree
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &tree))
	return &tree
}

func machineDump(t *testing.T, jsonStr string) *cephclient.OSDDump {
	t.Helper()
	var dump cephclient.OSDDump
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &dump))
	return &dump
}

func machineDeployment(osdID int, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        deploymentName(osdID),
			Namespace:   "rook-ceph",
			Annotations: map[string]string{},
			// Both labels matter: the app label is what getOSDDeployments' selector lists by —
			// without it the sweep never sees the deployment — and the id label is what GetOSDID
			// reads.
			Labels: map[string]string{k8sutil.AppAttr: AppName, OsdIdLabelKey: strconv.Itoa(osdID)},
		},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas},
	}
}

func TestDeriveDestroyState(t *testing.T) {
	tree := machineTree(t, `{"nodes":[{"id":7,"type":"osd","status":"destroyed"},{"id":8,"type":"osd","status":"up"}]}`)
	dump := machineDump(t, `{"osds":[{"osd":7,"up":1,"in":1},{"osd":8,"up":1,"in":0}]}`)

	t.Run("up in tree, out in dump, not destroyed, not downscaled", func(t *testing.T) {
		st := deriveDestroyState("rook-ceph", machineDeployment(8, 1), 8, tree, dump)
		assert.False(t, st.destroyed)
		assert.False(t, st.downscaled)
		assert.False(t, st.in) // osd.8 is out in the dump
	})
	t.Run("destroyed slot", func(t *testing.T) {
		st := deriveDestroyState("rook-ceph", machineDeployment(7, 1), 7, tree, dump)
		assert.True(t, st.destroyed)
		assert.True(t, st.in)
	})
	t.Run("downscaled deployment", func(t *testing.T) {
		st := deriveDestroyState("rook-ceph", machineDeployment(8, 0), 8, tree, dump)
		assert.True(t, st.downscaled)
	})
	t.Run("absent from dump is treated as in", func(t *testing.T) {
		st := deriveDestroyState("rook-ceph", machineDeployment(9, 1), 9, tree, dump)
		assert.True(t, st.in)
		assert.False(t, st.destroyed)
	})
}

// fakeDestroyFlow scripts every flow decision and records the calls the spine makes, in order.
type fakeDestroyFlow struct {
	calls []string

	disengageReq bool
	termReached  bool
	drainActed   bool
	stopOK       bool
	termOK       bool
}

func (f *fakeDestroyFlow) name() string                             { return "fake" }
func (f *fakeDestroyFlow) ownsDeployment(_ *appsv1.Deployment) bool { return true }
func (f *fakeDestroyFlow) isComplete(_ *appsv1.Deployment) bool     { return false }
func (f *fakeDestroyFlow) skipSweep() bool                          { return false }
func (f *fakeDestroyFlow) disengageRequested(_ *appsv1.Deployment, _ destroyState) bool {
	f.calls = append(f.calls, "disengageRequested")
	return f.disengageReq
}

func (f *fakeDestroyFlow) disengage(_ *appsv1.Deployment, _ int) error {
	f.calls = append(f.calls, "disengage")
	return nil
}

func (f *fakeDestroyFlow) terminalReached(_ destroyState) bool {
	f.calls = append(f.calls, "terminalReached")
	return f.termReached
}

func (f *fakeDestroyFlow) finalize(_ *appsv1.Deployment, _ int) error {
	f.calls = append(f.calls, "finalize")
	return nil
}

func (f *fakeDestroyFlow) startDrain(_ *appsv1.Deployment, _ int, _ destroyState) (bool, error) {
	f.calls = append(f.calls, "startDrain")
	return f.drainActed, nil
}

func (f *fakeDestroyFlow) stopGate(_ int) (bool, error) {
	f.calls = append(f.calls, "stopGate")
	return f.stopOK, nil
}

func (f *fakeDestroyFlow) terminalGate(_ int) (bool, error) {
	f.calls = append(f.calls, "terminalGate")
	return f.termOK, nil
}

func (f *fakeDestroyFlow) terminal(_ *appsv1.Deployment, _ int) error {
	f.calls = append(f.calls, "terminal")
	return nil
}

// newMachineTestMonitor builds a real monitor over a fake clientset and an executor that fails the
// test on ANY ceph command: the spine itself must never talk to Ceph — only flows and the shared
// helpers it is given do.
func newMachineTestMonitor(t *testing.T, objects ...runtime.Object) *OSDHealthMonitor {
	t.Helper()
	clusterInfo := cephclient.AdminTestClusterInfo("rook-ceph")
	clusterInfo.Context = context.TODO()
	// Stub BOTH executor hooks: cephclient commands route through WithOutput or WithTimeout
	// depending on the command builder.
	clusterdCtx := &clusterd.Context{
		Clientset: fake.NewClientset(objects...),
		Executor: &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				t.Errorf("unexpected ceph command from machine spine: %s %s", command, strings.Join(args, " "))
				return "", errors.New("unexpected ceph command")
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				t.Errorf("unexpected ceph command from machine spine: %s %s", command, strings.Join(args, " "))
				return "", errors.New("unexpected ceph command")
			},
		},
	}
	return NewOSDHealthMonitor(clusterdCtx, clusterInfo, false, cephv1.CephClusterHealthCheckSpec{}, cephv1.ClusterSpec{}, "rook/rook:test")
}

func TestAdvanceDestroySpine(t *testing.T) {
	tree := machineTree(t, `{"nodes":[{"id":7,"type":"osd","status":"up"}]}`)
	dump := machineDump(t, `{"osds":[{"osd":7,"up":1,"in":1}]}`)
	outDump := machineDump(t, `{"osds":[{"osd":7,"up":1,"in":0}]}`)
	destroyedTree := machineTree(t, `{"nodes":[{"id":7,"type":"osd","status":"destroyed"}]}`)

	t.Run("disengage wins over everything", func(t *testing.T) {
		m := newMachineTestMonitor(t)
		f := &fakeDestroyFlow{disengageReq: true, termReached: true}
		require.NoError(t, m.advanceDestroy(f, machineDeployment(7, 1), 7, tree, dump))
		assert.Equal(t, []string{"disengageRequested", "disengage"}, f.calls)
	})
	t.Run("terminal reached short-circuits to finalize", func(t *testing.T) {
		m := newMachineTestMonitor(t)
		f := &fakeDestroyFlow{termReached: true}
		require.NoError(t, m.advanceDestroy(f, machineDeployment(7, 1), 7, destroyedTree, dump))
		assert.Equal(t, []string{"disengageRequested", "terminalReached", "finalize"}, f.calls)
	})
	t.Run("drain action ends the tick without touching the stop gate", func(t *testing.T) {
		m := newMachineTestMonitor(t)
		f := &fakeDestroyFlow{drainActed: true}
		require.NoError(t, m.advanceDestroy(f, machineDeployment(7, 1), 7, tree, dump))
		assert.Equal(t, []string{"disengageRequested", "terminalReached", "startDrain"}, f.calls)
	})
	t.Run("stop gate refusal leaves the deployment up", func(t *testing.T) {
		m := newMachineTestMonitor(t)
		f := &fakeDestroyFlow{stopOK: false}
		d := machineDeployment(7, 1)
		require.NoError(t, m.advanceDestroy(f, d, 7, tree, outDump))
		assert.Equal(t, []string{"disengageRequested", "terminalReached", "startDrain", "stopGate"}, f.calls)
	})
	t.Run("stop gate pass scales the deployment down", func(t *testing.T) {
		d := machineDeployment(7, 1)
		m := newMachineTestMonitor(t, d)
		f := &fakeDestroyFlow{stopOK: true}
		require.NoError(t, m.advanceDestroy(f, d, 7, tree, outDump))
		assert.Equal(t, []string{"disengageRequested", "terminalReached", "startDrain", "stopGate"}, f.calls)
		updated, err := m.context.Clientset.AppsV1().Deployments("rook-ceph").Get(context.TODO(), d.Name, metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, int32(0), *updated.Spec.Replicas)
	})
	t.Run("downscaled with pod gone runs terminal gate then terminal", func(t *testing.T) {
		d := machineDeployment(7, 0)
		d.Labels[encrypted] = "true"
		succeededJob := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: cryptCloseJobName(7), Namespace: "rook-ceph"},
			Status:     batchv1.JobStatus{Succeeded: 1},
		}
		m := newMachineTestMonitor(t, d, succeededJob)
		f := &fakeDestroyFlow{termOK: true}
		require.NoError(t, m.advanceDestroy(f, d, 7, tree, outDump))
		assert.Equal(t, []string{"disengageRequested", "terminalReached", "terminalGate", "terminal"}, f.calls)
	})
	t.Run("downscaled with terminal gate refusal retries next tick", func(t *testing.T) {
		d := machineDeployment(7, 0)
		d.Labels[encrypted] = "true"
		succeededJob := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: cryptCloseJobName(7), Namespace: "rook-ceph"},
			Status:     batchv1.JobStatus{Succeeded: 1},
		}
		m := newMachineTestMonitor(t, d, succeededJob)
		f := &fakeDestroyFlow{termOK: false}
		require.NoError(t, m.advanceDestroy(f, d, 7, tree, outDump))
		assert.Equal(t, []string{"disengageRequested", "terminalReached", "terminalGate"}, f.calls)
	})
}

// newSweepTestMonitor is newMachineTestMonitor without the t.Errorf guard: the sweep legitimately
// issues `osd tree`/`osd dump`, and these tests exercise the fetch-FAILURE path, so every ceph
// command fails with a plain error instead of failing the test.
func newSweepTestMonitor(objects ...runtime.Object) *OSDHealthMonitor {
	clusterInfo := cephclient.AdminTestClusterInfo("rook-ceph")
	clusterInfo.Context = context.TODO()
	clusterdCtx := &clusterd.Context{
		Clientset: fake.NewClientset(objects...),
		Executor: &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				return "", errors.Errorf("induced ceph failure: %s", command)
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				return "", errors.Errorf("induced ceph failure: %s", command)
			},
		},
	}
	return NewOSDHealthMonitor(clusterdCtx, clusterInfo, false, cephv1.CephClusterHealthCheckSpec{}, cephv1.ClusterSpec{}, "rook/rook:test")
}

func TestDestroySweepOwnedSetOnFetchFailure(t *testing.T) {
	// The owned set must be complete even when the tree/dump fetch fails: it is built from
	// deployment markers BEFORE any Ceph query, so the normal health path never acts on an
	// owned OSD just because Ceph was briefly unreachable.
	d := machineDeployment(7, 1)
	m := newSweepTestMonitor(d)
	f := &fakeDestroyFlow{}
	owned, err := m.destroySweep(f)
	require.NoError(t, err)
	assert.Equal(t, map[int]struct{}{7: {}}, owned)
	// advance was never reached: no flow decision calls beyond collection were made
	assert.Empty(t, f.calls)
}

func TestDestroySweepSkip(t *testing.T) {
	m := newSweepTestMonitor(machineDeployment(7, 1))
	lists := 0
	m.context.Clientset.(*fake.Clientset).PrependReactor("list", "deployments",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			lists++
			return false, nil, nil
		})
	f := &skippingFlow{&fakeDestroyFlow{}}
	owned, err := m.destroySweep(f)
	require.NoError(t, err)
	assert.Empty(t, owned)
	assert.Zero(t, lists, "skipSweep must short-circuit before the deployment LIST")
}

type skippingFlow struct{ *fakeDestroyFlow }

func (s *skippingFlow) skipSweep() bool { return true }
