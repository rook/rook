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
	"strings"
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

// osdMetadataJSON builds an `osd metadata` response from id -> "hostname|devices" pairs.
func osdMetadataJSON(osds map[int]string) string {
	entries := ""
	for id, hostAndDevices := range osds {
		host, devices, _ := strings.Cut(hostAndDevices, "|")
		if entries != "" {
			entries += ","
		}
		entries += fmt.Sprintf(`{"id":%d,"hostname":%q,"devices":%q}`, id, host, devices)
	}
	return fmt.Sprintf(`[%s]`, entries)
}

func newReplaceClusterWithTree(clientset *fake.Clientset, osds map[int]string) *Cluster {
	// Default metadata: every OSD alone on its own device, so only the tree drives the outcome.
	metadata := map[int]string{}
	for id := range osds {
		metadata[id] = fmt.Sprintf("node-1|disk%d", id)
	}
	return newReplaceClusterWithTreeAndMetadata(clientset, osds, metadata)
}

func newReplaceClusterWithTreeAndMetadata(clientset *fake.Clientset, osds, metadata map[int]string) *Cluster {
	c := newTestReplaceCluster(clientset)
	c.context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "osd" && args[1] == "tree" {
				return osdTreeJSON(osds), nil
			}
			if len(args) >= 2 && args[0] == "osd" && args[1] == "metadata" {
				return osdMetadataJSON(metadata), nil
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

	t.Run("valid request gets fenced and marked in progress", func(t *testing.T) {
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-5"}, nil)
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{5: "up"})

		require.NoError(t, c.validateAndStartOSDReplacement())
		got := getDep(c, 5)
		assert.Equal(t, "true", got.Labels[cephv1.SkipReconcileLabelKey])
		assert.Equal(t, "true", got.Annotations[cephv1.ReplaceInProgressOSDAnnotationKey])
	})

	t.Run("id mismatch is rejected and not fenced", func(t *testing.T) {
		// annotation says osd 6 but the deployment is osd 5
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-6"}, nil)
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{5: "up"})

		require.NoError(t, c.validateAndStartOSDReplacement())
		assert.NotContains(t, getDep(c, 5).Labels, cephv1.SkipReconcileLabelKey)
		assert.NotContains(t, getDep(c, 5).Annotations, cephv1.ReplaceInProgressOSDAnnotationKey)
	})

	t.Run("PVC-backed OSD is rejected and not fenced", func(t *testing.T) {
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-5"},
			map[string]string{OSDOverPVCLabelKey: "set1-0"})
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{5: "up"})

		require.NoError(t, c.validateAndStartOSDReplacement())
		assert.NotContains(t, getDep(c, 5).Labels, cephv1.SkipReconcileLabelKey)
		assert.NotContains(t, getDep(c, 5).Annotations, cephv1.ReplaceInProgressOSDAnnotationKey)
	})

	t.Run("already-destroyed OSD is accepted and fenced", func(t *testing.T) {
		// A rare state (manual destroy, or fence cleared mid-flow): accept it so the goroutine can
		// resume idempotently from its destroyed phase rather than rejecting a partway teardown.
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-5"}, nil)
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{5: "destroyed"})

		require.NoError(t, c.validateAndStartOSDReplacement())
		got := getDep(c, 5)
		assert.Equal(t, "true", got.Labels[cephv1.SkipReconcileLabelKey])
		assert.Equal(t, "true", got.Annotations[cephv1.ReplaceInProgressOSDAnnotationKey])
	})

	t.Run("nonexistent OSD is rejected and not fenced", func(t *testing.T) {
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-5"}, nil)
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{7: "up"})

		require.NoError(t, c.validateAndStartOSDReplacement())
		assert.NotContains(t, getDep(c, 5).Labels, cephv1.SkipReconcileLabelKey)
		assert.NotContains(t, getDep(c, 5).Annotations, cephv1.ReplaceInProgressOSDAnnotationKey)
	})

	t.Run("OSD sharing a device with a sibling is rejected and not fenced", func(t *testing.T) {
		// osdsPerDevice > 1: osd 5 and osd 6 both sit on vdb, so replacement cannot pair the destroyed
		// slots one-to-one with blank devices.
		dep := osdDeployment(5, map[string]string{cephv1.ReplaceOSDAnnotationKey: "yes-really-replace-osd-5"}, nil)
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTreeAndMetadata(clientset,
			map[int]string{5: "up", 6: "up"},
			map[int]string{5: "node-1|vdb", 6: "node-1|vdb"})

		require.NoError(t, c.validateAndStartOSDReplacement())
		assert.NotContains(t, getDep(c, 5).Labels, cephv1.SkipReconcileLabelKey)
		assert.NotContains(t, getDep(c, 5).Annotations, cephv1.ReplaceInProgressOSDAnnotationKey)
	})

	t.Run("deployment without the annotation is ignored", func(t *testing.T) {
		dep := osdDeployment(5, nil, nil)
		clientset := fake.NewClientset(dep)
		c := newReplaceClusterWithTree(clientset, map[int]string{5: "up"})

		require.NoError(t, c.validateAndStartOSDReplacement())
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
		require.NoError(t, c.validateAndStartOSDReplacement())
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

		require.NoError(t, c.validateAndStartOSDReplacement())
		assert.Equal(t, "true", getDep(c, 5).Annotations[cephv1.ReplaceInProgressOSDAnnotationKey])
	})
}

func TestValidateSingleOSDPerDevice(t *testing.T) {
	md := func(id int, host, devices string) cephclient.OSDMetadata {
		return cephclient.OSDMetadata{Id: id, HostName: host, Devices: devices}
	}

	tests := []struct {
		name     string
		metadata []cephclient.OSDMetadata
		rejected bool
	}{
		{
			name:     "one OSD per device",
			metadata: []cephclient.OSDMetadata{md(5, "node-1", "vdb"), md(6, "node-1", "vdc")},
		},
		{
			// osdsPerDevice > 1: both OSDs are backed by the same disk.
			name:     "sibling on the same device",
			metadata: []cephclient.OSDMetadata{md(5, "node-1", "vdb"), md(6, "node-1", "vdb")},
			rejected: true,
		},
		{
			// The same kernel name on two hosts is two different disks.
			name:     "same device name on another host",
			metadata: []cephclient.OSDMetadata{md(5, "node-1", "vdb"), md(6, "node-2", "vdb")},
		},
		{
			// Siblings sharing a DB device also report their own distinct data disk, so this supported
			// layout must not be mistaken for osdsPerDevice > 1.
			name:     "shared metadata device",
			metadata: []cephclient.OSDMetadata{md(5, "node-1", "nvme0n1,vdb"), md(6, "node-1", "nvme0n1,vdc")},
		},
		{
			name:     "shared metadata device and a sibling on the data disk",
			metadata: []cephclient.OSDMetadata{md(5, "node-1", "nvme0n1,vdb"), md(6, "node-1", "nvme0n1,vdb")},
			rejected: true,
		},
		{
			// Missing or unresolved metadata must not block the request.
			name:     "target has no metadata",
			metadata: []cephclient.OSDMetadata{md(6, "node-1", "vdb")},
		},
		{
			name:     "target reports no devices",
			metadata: []cephclient.OSDMetadata{md(5, "node-1", ""), md(6, "node-1", "")},
		},
		{
			name:     "target reports no hostname",
			metadata: []cephclient.OSDMetadata{md(5, "", "vdb"), md(6, "", "vdb")},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSingleOSDPerDevice(5, test.metadata)
			if test.rejected {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
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
