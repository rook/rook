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
	"fmt"
	"strconv"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newTestReplaceCluster(clientset *fake.Clientset) *Cluster {
	return newTestReplaceClusterWithSpec(clientset, cephv1.ClusterSpec{})
}

func newTestReplaceClusterWithSpec(clientset *fake.Clientset, spec cephv1.ClusterSpec) *Cluster {
	clusterInfo := &cephclient.ClusterInfo{
		Namespace: "rook-ceph",
		Context:   context.TODO(),
		OwnerInfo: cephclient.NewMinimumOwnerInfoWithOwnerRef(),
	}
	clusterInfo.SetName("test")
	c := &clusterd.Context{Clientset: clientset}
	return &Cluster{
		context:     c,
		clusterInfo: clusterInfo,
		rookVersion: "rook/ceph:test",
		spec:        spec,
	}
}

// osdTreeJSON builds an `osd tree` response containing one osd-type node per given id->status pair.
func osdTreeJSON(osds map[int]string) string {
	nodes := ""
	for id, status := range osds {
		if nodes != "" {
			nodes += ","
		}
		nodes += fmt.Sprintf(`{"id":%d,"name":"osd.%d","type":"osd","type_id":0,"exists":1,"status":%q}`, id, id, status)
	}
	return fmt.Sprintf(`{"nodes":[%s],"stray":[]}`, nodes)
}

func newReplaceClusterWithTree(clientset *fake.Clientset, osds map[int]string) *Cluster {
	c := newTestReplaceCluster(clientset)
	c.context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "osd" && args[1] == "tree" {
				return osdTreeJSON(osds), nil
			}
			return "", nil
		},
	}
	return c
}

func osdDeployment(osdID int, annotations, labels map[string]string) *appsv1.Deployment {
	if labels == nil {
		labels = map[string]string{}
	}
	labels[k8sutil.AppAttr] = AppName
	labels[OsdIdLabelKey] = strconv.Itoa(osdID)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf(osdAppNameFmt, osdID),
			Namespace:   "rook-ceph",
			Labels:      labels,
			Annotations: annotations,
		},
	}
}

func TestProcessOSDReplacements(t *testing.T) {
	getDep := func(c *Cluster, osdID int) *appsv1.Deployment {
		d, err := c.context.Clientset.AppsV1().Deployments("rook-ceph").Get(context.TODO(), fmt.Sprintf(osdAppNameFmt, osdID), metav1.GetOptions{})
		require.NoError(t, err)
		return d
	}

	t.Run("valid request gets fenced and marked in progress", func(t *testing.T) {
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-5"}, nil)
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{5: "up"})

		require.NoError(t, c.processOSDReplacements())
		got := getDep(c, 5)
		assert.Equal(t, "true", got.Labels[cephv1.SkipReconcileLabelKey])
		assert.Equal(t, "true", got.Annotations[cephv1.ReplaceInProgressOSDAnnotationKey])
	})

	t.Run("id mismatch is rejected and not fenced", func(t *testing.T) {
		// annotation says osd 6 but the deployment is osd 5
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-6"}, nil)
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{5: "up"})

		require.NoError(t, c.processOSDReplacements())
		assert.NotContains(t, getDep(c, 5).Labels, cephv1.SkipReconcileLabelKey)
		assert.NotContains(t, getDep(c, 5).Annotations, cephv1.ReplaceInProgressOSDAnnotationKey)
	})

	t.Run("PVC-backed OSD is rejected and not fenced", func(t *testing.T) {
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-5"},
			map[string]string{OSDOverPVCLabelKey: "set1-0"})
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{5: "up"})

		require.NoError(t, c.processOSDReplacements())
		assert.NotContains(t, getDep(c, 5).Labels, cephv1.SkipReconcileLabelKey)
		assert.NotContains(t, getDep(c, 5).Annotations, cephv1.ReplaceInProgressOSDAnnotationKey)
	})

	t.Run("already-destroyed OSD is accepted and fenced", func(t *testing.T) {
		// A rare state (manual destroy, or fence cleared mid-flow): accept it so the goroutine can
		// resume idempotently from its destroyed phase rather than rejecting a partway teardown.
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-5"}, nil)
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{5: "destroyed"})

		require.NoError(t, c.processOSDReplacements())
		got := getDep(c, 5)
		assert.Equal(t, "true", got.Labels[cephv1.SkipReconcileLabelKey])
		assert.Equal(t, "true", got.Annotations[cephv1.ReplaceInProgressOSDAnnotationKey])
	})

	t.Run("nonexistent OSD is rejected and not fenced", func(t *testing.T) {
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-5"}, nil)
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{7: "up"})

		require.NoError(t, c.processOSDReplacements())
		assert.NotContains(t, getDep(c, 5).Labels, cephv1.SkipReconcileLabelKey)
		assert.NotContains(t, getDep(c, 5).Annotations, cephv1.ReplaceInProgressOSDAnnotationKey)
	})

	t.Run("deployment without the annotation is ignored", func(t *testing.T) {
		dep := osdDeployment(5, nil, nil)
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{5: "up"})

		require.NoError(t, c.processOSDReplacements())
		assert.NotContains(t, getDep(c, 5).Labels, cephv1.SkipReconcileLabelKey)
	})

	t.Run("already-marked deployment is left untouched", func(t *testing.T) {
		dep := osdDeployment(5, map[string]string{
			cephv1.ReplaceOSDAnnotationKey:           "yes-really-replace-osd-5",
			cephv1.ReplaceInProgressOSDAnnotationKey: "true",
		}, map[string]string{cephv1.SkipReconcileLabelKey: "true"})
		clientset := fake.NewClientset(dep)
		c := newTestReplaceCluster(clientset)
		// no executor: an OSD the goroutine already owns must not trigger an osd tree lookup
		require.NoError(t, c.processOSDReplacements())
		assert.Equal(t, "true", getDep(c, 5).Labels[cephv1.SkipReconcileLabelKey])
	})

	t.Run("already-fenced deployment is still validated", func(t *testing.T) {
		// An OSD an admin (or the maintenance plugin) already fenced carries the same label without having
		// been validated. The label must not be mistaken for a completed validation, or the goroutine would
		// take over an unvalidated request.
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-5"},
			map[string]string{cephv1.SkipReconcileLabelKey: "true"})
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{5: "up"})

		require.NoError(t, c.processOSDReplacements())
		assert.Equal(t, "true", getDep(c, 5).Annotations[cephv1.ReplaceInProgressOSDAnnotationKey])
	})
}

// TestCleanupAbortedReplacement covers the cleanup of a replacement that can never complete: the OSD is
// gone from the osdmap, so no destroyed slot is left to provision the swapped-in disk into. Driven through
// processOSDReplacements to cover the wiring, since such a deployment is otherwise skipped there.
func TestCleanupAbortedReplacement(t *testing.T) {
	osdID := 5
	waitingForSwapDep := func() *appsv1.Deployment {
		d := osdDeployment(osdID, map[string]string{
			cephv1.ReplaceOSDAnnotationKey:           fmt.Sprintf(cephv1.ReplaceOSDAnnotationValueFmt, osdID),
			cephv1.ReplaceInProgressOSDAnnotationKey: "true",
			cephv1.ReadyForSwapOSDAnnotationKey:      "true",
		}, map[string]string{cephv1.SkipReconcileLabelKey: "true"})
		zero := int32(0)
		d.Spec.Replicas = &zero
		return d
	}

	// The osd tree must never be fetched for an already-marked deployment, so anything but `osd dump` fails.
	newCluster := func(clientset *fake.Clientset, inByID map[int]int) *Cluster {
		c := newTestReplaceCluster(clientset)
		c.context.Executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if len(args) >= 2 && args[0] == "osd" && args[1] == "dump" {
					return osdDumpJSON(inByID), nil
				}
				return "", fmt.Errorf("unexpected ceph command %v", args)
			},
		}
		return c
	}

	// Only a NotFound counts as deleted; any other error fails the test rather than reading as a
	// successful cleanup.
	depExists := func(t *testing.T, c *Cluster) bool {
		t.Helper()
		_, err := c.context.Clientset.AppsV1().Deployments("rook-ceph").Get(context.TODO(), fmt.Sprintf(osdAppNameFmt, osdID), metav1.GetOptions{})
		if kerrors.IsNotFound(err) {
			return false
		}
		require.NoError(t, err)
		return true
	}

	t.Run("osd gone from the osdmap deletes the leftover deployment", func(t *testing.T) {
		c := newCluster(fake.NewClientset(waitingForSwapDep()), map[int]int{7: 1})
		require.NoError(t, c.processOSDReplacements())
		assert.False(t, depExists(t, c))
	})

	t.Run("osd still in the osdmap is left alone", func(t *testing.T) {
		// The destroyed slot is still waiting for its disk; deleting it here would drop the marker and
		// the user's ready-for-swap signal mid-replacement.
		c := newCluster(fake.NewClientset(waitingForSwapDep()), map[int]int{osdID: 0})
		require.NoError(t, c.processOSDReplacements())
		assert.True(t, depExists(t, c))
	})

	t.Run("empty osd dump is not treated as a missing osd", func(t *testing.T) {
		c := newCluster(fake.NewClientset(waitingForSwapDep()), map[int]int{})
		require.NoError(t, c.processOSDReplacements())
		assert.True(t, depExists(t, c))
	})

	t.Run("osd dump failure leaves the deployment alone", func(t *testing.T) {
		// Without the dump the OSD's existence cannot be established, so a replacement that may still
		// be in progress must keep its deployment rather than risk losing the marker.
		c := newTestReplaceCluster(fake.NewClientset(waitingForSwapDep()))
		c.context.Executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				return "", fmt.Errorf("mon is unreachable")
			},
		}
		require.NoError(t, c.processOSDReplacements())
		assert.True(t, depExists(t, c))
	})
}

func TestReplacementReadyToRecreate(t *testing.T) {
	withReplicas := func(d *appsv1.Deployment, n int32) *appsv1.Deployment {
		d.Spec.Replicas = &n
		return d
	}

	t.Run("ready-for-swap annotation present and scaled to zero", func(t *testing.T) {
		dep := withReplicas(osdDeployment(5, map[string]string{cephv1.ReadyForSwapOSDAnnotationKey: ""}, nil), 0)
		c := newTestReplaceCluster(fake.NewClientset(dep))
		ready, err := c.replacementReadyForSwap(5)
		require.NoError(t, err)
		assert.True(t, ready)
	})

	t.Run("annotation absent", func(t *testing.T) {
		dep := withReplicas(osdDeployment(5, nil, nil), 0)
		c := newTestReplaceCluster(fake.NewClientset(dep))
		ready, err := c.replacementReadyForSwap(5)
		require.NoError(t, err)
		assert.False(t, ready)
	})

	t.Run("deployment missing is not an error", func(t *testing.T) {
		c := newTestReplaceCluster(fake.NewClientset())
		ready, err := c.replacementReadyForSwap(5)
		require.NoError(t, err)
		assert.False(t, ready)
	})
}
