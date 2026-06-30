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
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
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

func TestValidateAndStartOSDReplacement(t *testing.T) {
	getDep := func(c *Cluster, osdID int) *appsv1.Deployment {
		d, err := c.context.Clientset.AppsV1().Deployments("rook-ceph").Get(context.TODO(), fmt.Sprintf(osdAppNameFmt, osdID), metav1.GetOptions{})
		require.NoError(t, err)
		return d
	}

	t.Run("valid request gets fenced", func(t *testing.T) {
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-5"}, nil)
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{5: "up"})

		require.NoError(t, c.validateAndStartOSDReplacement())
		assert.Equal(t, "true", getDep(c, 5).Labels[cephv1.SkipReconcileLabelKey])
	})

	t.Run("id mismatch is rejected and not fenced", func(t *testing.T) {
		// annotation says osd 6 but the deployment is osd 5
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-6"}, nil)
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{5: "up"})

		require.NoError(t, c.validateAndStartOSDReplacement())
		assert.NotEqual(t, "true", getDep(c, 5).Labels[cephv1.SkipReconcileLabelKey])
	})

	t.Run("PVC-backed OSD is rejected and not fenced", func(t *testing.T) {
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-5"},
			map[string]string{OSDOverPVCLabelKey: "set1-0"})
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{5: "up"})

		require.NoError(t, c.validateAndStartOSDReplacement())
		assert.NotEqual(t, "true", getDep(c, 5).Labels[cephv1.SkipReconcileLabelKey])
	})

	t.Run("already-destroyed OSD is accepted and fenced", func(t *testing.T) {
		// A rare state (manual destroy, or fence cleared mid-flow): accept it so the goroutine can
		// resume idempotently from its destroyed phase rather than rejecting a partway teardown.
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-5"}, nil)
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{5: "destroyed"})

		require.NoError(t, c.validateAndStartOSDReplacement())
		assert.Equal(t, "true", getDep(c, 5).Labels[cephv1.SkipReconcileLabelKey])
	})

	t.Run("nonexistent OSD is rejected and not fenced", func(t *testing.T) {
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-5"}, nil)
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{7: "up"})

		require.NoError(t, c.validateAndStartOSDReplacement())
		assert.NotEqual(t, "true", getDep(c, 5).Labels[cephv1.SkipReconcileLabelKey])
	})

	t.Run("deployment without the annotation is ignored", func(t *testing.T) {
		dep := osdDeployment(5, nil, nil)
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{5: "up"})

		require.NoError(t, c.validateAndStartOSDReplacement())
		assert.NotContains(t, getDep(c, 5).Labels, cephv1.SkipReconcileLabelKey)
	})

	t.Run("already-fenced deployment is left untouched", func(t *testing.T) {
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-5"},
			map[string]string{cephv1.SkipReconcileLabelKey: "true"})
		clientset := fake.NewClientset(dep)
		c := newTestReplaceCluster(clientset)
		// no executor: an already-fenced OSD must not trigger an osd tree lookup
		require.NoError(t, c.validateAndStartOSDReplacement())
		assert.Equal(t, "true", getDep(c, 5).Labels[cephv1.SkipReconcileLabelKey])
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
		ready, err := c.replacementReadyToRecreate(5)
		require.NoError(t, err)
		assert.True(t, ready)
	})

	t.Run("annotation present but still scaled up is not ready", func(t *testing.T) {
		// Defense-in-depth: a marker awaiting recreate is always scaled to zero, so a still-running
		// OSD must never be selected for delete-then-recreate.
		dep := withReplicas(osdDeployment(5, map[string]string{cephv1.ReadyForSwapOSDAnnotationKey: ""}, nil), 1)
		c := newTestReplaceCluster(fake.NewClientset(dep))
		ready, err := c.replacementReadyToRecreate(5)
		require.NoError(t, err)
		assert.False(t, ready)
	})

	t.Run("annotation present but nil replicas (defaults to 1) is not ready", func(t *testing.T) {
		dep := osdDeployment(5, map[string]string{cephv1.ReadyForSwapOSDAnnotationKey: ""}, nil)
		c := newTestReplaceCluster(fake.NewClientset(dep))
		ready, err := c.replacementReadyToRecreate(5)
		require.NoError(t, err)
		assert.False(t, ready)
	})

	t.Run("annotation absent", func(t *testing.T) {
		dep := withReplicas(osdDeployment(5, nil, nil), 0)
		c := newTestReplaceCluster(fake.NewClientset(dep))
		ready, err := c.replacementReadyToRecreate(5)
		require.NoError(t, err)
		assert.False(t, ready)
	})

	t.Run("deployment missing is not an error", func(t *testing.T) {
		c := newTestReplaceCluster(fake.NewClientset())
		ready, err := c.replacementReadyToRecreate(5)
		require.NoError(t, err)
		assert.False(t, ready)
	})
}
