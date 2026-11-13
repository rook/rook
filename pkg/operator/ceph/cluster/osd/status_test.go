/*
Copyright 2018 The Rook Authors. All rights reserved.

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
	"path/filepath"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
)

func TestOrchestrationStatus(t *testing.T) {
	ctx := t.Context()
	clientset := fake.NewClientset()
	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   "ns",
		CephVersion: cephver.Squid,
	}
	context := &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}
	spec := cephv1.ClusterSpec{}
	c := New(context, clusterInfo, spec, "myversion")
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	kv := k8sutil.NewConfigMapKVStore(c.clusterInfo.Namespace, clientset, ownerInfo)
	nodeName := "mynode"
	cmName := fmt.Sprintf(orchestrationStatusMapName, nodeName)

	// status map should not exist yet
	_, err := c.context.Clientset.CoreV1().ConfigMaps(c.clusterInfo.Namespace).Get(ctx, cmName, metav1.GetOptions{})
	assert.True(t, errors.IsNotFound(err))

	// update the status map with some status
	status := OrchestrationStatus{Status: OrchestrationStatusOrchestrating, Message: "doing work"}
	UpdateNodeOrPVCStatus(ctx, kv, nodeName, status)

	// retrieve the status and verify it
	statusMap, err := c.context.Clientset.CoreV1().ConfigMaps(c.clusterInfo.Namespace).Get(ctx, cmName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, statusMap)
	retrievedStatus := parseOrchestrationStatus(statusMap.Data)
	assert.NotNil(t, retrievedStatus)
	assert.Equal(t, status, *retrievedStatus)
}

func TestCreateOSDsForStatusMapValidatesRecord(t *testing.T) {
	const (
		namespace       = "ns"
		nodeName        = "node1"
		pvcName         = "pvc1"
		fsid            = "c5b8e5d0-4a7e-4a8b-9f0e-6b1d2c3e4f50"
		dataDirHostPath = "/var/lib/rook"
		validUUID       = "6e7c4d3f-3494-4dc1-90dc-58527fdf05d7"
	)

	oldConditionFunc := updateConditionFunc
	t.Cleanup(func() { updateConditionFunc = oldConditionFunc })
	updateConditionFunc = func(context.Context, *clusterd.Context, types.NamespacedName, int64, cephv1.ConditionType, corev1.ConditionStatus, cephv1.ConditionReason, string) {
	}

	newCluster := func(t *testing.T) *Cluster {
		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:   nodeName,
			Labels: map[string]string{corev1.LabelHostname: nodeName},
		}}
		clusterInfo := &cephclient.ClusterInfo{Namespace: namespace, FSID: fsid, CephVersion: cephver.Squid, Context: t.Context()}
		clusterInfo.SetName("mycluster")
		clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
		ctx := &clusterd.Context{Clientset: fake.NewClientset(node)}
		c := New(ctx, clusterInfo, cephv1.ClusterSpec{DataDirHostPath: dataDirHostPath}, "rook/rook:master")
		c.ValidStorage.Nodes = []cephv1.Node{{Name: nodeName}}
		c.deviceSets = []deviceSet{{
			Name:       pvcName,
			PVCSources: map[string]corev1.PersistentVolumeClaimVolumeSource{bluestorePVCData: {ClaimName: pvcName}},
			Portable:   true,
		}}
		return c
	}

	// write the status into the status ConfigMap of the node or PVC, then process it the way the
	// operator does when a prepare job reports completion
	processStatus := func(t *testing.T, c *Cluster, nodeOrPVC string, status OrchestrationStatus) (*createConfig, *provisionErrors) {
		cmName := UpdateNodeOrPVCStatus(t.Context(), c.kv, nodeOrPVC, status)
		cm, err := c.context.Clientset.CoreV1().ConfigMaps(namespace).Get(t.Context(), cmName, metav1.GetOptions{})
		require.NoError(t, err)
		createConfig := c.newCreateConfig(c.newProvisionConfig(), sets.New(cmName), newExistenceListWithCapacity(0))
		errs := newProvisionErrors()
		c.createOSDsForStatusMap(cm, createConfig, errs)
		return createConfig, errs
	}

	// the OSD deployments that exist, each mapped to the host path of its activate volume, or to ""
	// when it has none
	activatePathsByDeployment := func(t *testing.T, c *Cluster) map[string]string {
		deps, err := c.context.Clientset.AppsV1().Deployments(namespace).List(t.Context(), metav1.ListOptions{})
		require.NoError(t, err)
		paths := map[string]string{}
		for _, d := range deps.Items {
			paths[d.Name] = ""
			for _, v := range d.Spec.Template.Spec.Volumes {
				if v.Name == activateOSDVolumeName && v.HostPath != nil {
					paths[d.Name] = v.HostPath.Path
				}
			}
		}
		return paths
	}
	validActivatePath := filepath.Join(dataDirHostPath, namespace, fsid+"_"+validUUID)

	for _, cvMode := range []string{"lvm", "raw"} {
		t.Run("node: valid "+cvMode+" record creates the deployment", func(t *testing.T) {
			c := newCluster(t)
			createConfig, errs := processStatus(t, c, nodeName, OrchestrationStatus{
				Status: OrchestrationStatusCompleted,
				OSDs:   []OSDInfo{{ID: 8, UUID: validUUID, BlockPath: "/dev/sdc", CVMode: cvMode}},
			})
			assert.Zero(t, errs.len())
			assert.True(t, createConfig.doneCreating())
			assert.Equal(t, map[string]string{deploymentName(8): validActivatePath}, activatePathsByDeployment(t, c))
		})

		t.Run("pvc: valid "+cvMode+" record creates the deployment", func(t *testing.T) {
			c := newCluster(t)
			createConfig, errs := processStatus(t, c, pvcName, OrchestrationStatus{
				Status:       OrchestrationStatusCompleted,
				PvcBackedOSD: true,
				OSDs:         []OSDInfo{{ID: 8, UUID: validUUID, BlockPath: "/mnt/" + pvcName, CVMode: cvMode}},
			})
			assert.Zero(t, errs.len())
			assert.True(t, createConfig.doneCreating())
			assert.Equal(t, map[string]string{deploymentName(8): ""}, activatePathsByDeployment(t, c))
		})
	}

	invalidRecords := []struct{ name, uuid, cvMode, wantErr string }{
		{"uuid with a path traversal", "x/../../../../../etc", "raw", "is not a UUID in canonical form"},
		{"uuid with cap string syntax", `x",profile=osd,profile="y`, "raw", "is not a UUID in canonical form"},
		{"ceph-volume mode with shell syntax", "0b6b0a4e-6d0b-4d55-8f8e-0f5e1c9b2a17", "raw; true", `ceph-volume mode "raw; true" is not "lvm" or "raw"`},
	}
	for _, tc := range invalidRecords {
		t.Run("node: "+tc.name+" is skipped and other OSDs are created", func(t *testing.T) {
			c := newCluster(t)
			createConfig, errs := processStatus(t, c, nodeName, OrchestrationStatus{
				Status: OrchestrationStatusCompleted,
				OSDs: []OSDInfo{
					{ID: 7, UUID: tc.uuid, BlockPath: "/dev/sdb", CVMode: tc.cvMode, Encrypted: true},
					{ID: 8, UUID: validUUID, BlockPath: "/dev/sdc", CVMode: "lvm"},
				},
			})
			assert.Equal(t, 1, errs.len())
			assert.Contains(t, errs.asMessages(), "failed to create OSD 7")
			assert.Contains(t, errs.asMessages(), tc.wantErr)
			assert.True(t, createConfig.doneCreating())
			assert.Equal(t, map[string]string{deploymentName(8): validActivatePath}, activatePathsByDeployment(t, c))
		})

		t.Run("pvc: "+tc.name+" is skipped", func(t *testing.T) {
			c := newCluster(t)
			createConfig, errs := processStatus(t, c, pvcName, OrchestrationStatus{
				Status:       OrchestrationStatusCompleted,
				PvcBackedOSD: true,
				OSDs:         []OSDInfo{{ID: 7, UUID: tc.uuid, BlockPath: "/mnt/" + pvcName, CVMode: tc.cvMode}},
			})
			assert.Equal(t, 1, errs.len())
			assert.Contains(t, errs.asMessages(), "failed to create OSD 7")
			assert.Contains(t, errs.asMessages(), tc.wantErr)
			assert.True(t, createConfig.doneCreating())
			assert.Empty(t, activatePathsByDeployment(t, c))
		})
	}
}

func mockNodeOrchestrationCompletion(c *Cluster, nodeName string, statusMapWatcher *watch.RaceFreeFakeWatcher) {
	ctx := context.TODO()
	// if no valid osd node, don't need to check its status, return immediately
	if len(c.spec.Storage.Nodes) == 0 {
		return
	}
	for {
		// wait for the node's orchestration status to change to "starting"
		cmName := statusConfigMapName(nodeName)
		cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.clusterInfo.Namespace).Get(ctx, cmName, metav1.GetOptions{})
		if err == nil {
			status := parseOrchestrationStatus(cm.Data)
			if status != nil && status.Status == OrchestrationStatusStarting {
				// the node has started orchestration, simulate its completion now by performing 2 tasks:
				// 1) update the config map manually (which doesn't trigger a watch event, see https://github.com/kubernetes/kubernetes/issues/54075#issuecomment-337298950)
				status = &OrchestrationStatus{
					OSDs: []OSDInfo{
						{
							ID:        1,
							UUID:      "00000000-0000-0000-0000-000000000001",
							Cluster:   "rook",
							CVMode:    "raw",
							BlockPath: "/dev/some/path",
						},
					},
					Status: OrchestrationStatusCompleted,
				}
				UpdateNodeOrPVCStatus(ctx, c.kv, nodeName, *status)

				// 2) call modify on the fake watcher so a watch event will get triggered
				s, _ := json.Marshal(status)
				cm.Data[orchestrationStatusKey] = string(s)
				statusMapWatcher.Modify(cm)
				break
			} else {
				logger.Debugf("waiting for node %s orchestration to start. status: %+v", nodeName, *status)
			}
		} else {
			logger.Warningf("failed to get node %s orchestration status, will try again: %+v", nodeName, err)
		}
		<-time.After(50 * time.Millisecond)
	}
}

func waitForOrchestrationCompletion(c *Cluster, nodeName string, startCompleted *bool) {
	ctx := context.TODO()
	for {
		if *startCompleted {
			break
		}
		cmName := statusConfigMapName(nodeName)
		cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.clusterInfo.Namespace).Get(ctx, cmName, metav1.GetOptions{})
		if err == nil {
			status := parseOrchestrationStatus(cm.Data)
			if status != nil {
				logger.Debugf("start has not completed, status is %+v", status)
			}
		}
		<-time.After(50 * time.Millisecond)
	}
}

func TestPrepareStatusRetry(t *testing.T) {
	for _, pvcBacked := range []bool{false, true} {
		for _, succeeds := range []bool{false, true} {
			t.Run(fmt.Sprintf("pvc=%t/succeeds=%t", pvcBacked, succeeds), func(t *testing.T) {
				ctx := context.Background()
				clientset := fake.NewClientset()
				clusterInfo := &cephclient.ClusterInfo{
					Context: ctx, Namespace: "ns", CephVersion: cephver.Squid,
					OwnerInfo: cephclient.NewMinimumOwnerInfoWithOwnerRef(),
				}
				c := New(&clusterd.Context{Clientset: clientset}, clusterInfo, cephv1.ClusterSpec{}, "rook/rook:master")
				name := "retry-device"
				cmName := statusConfigMapName(name)
				config := c.newCreateConfig(c.newProvisionConfig(), sets.New(cmName), newExistenceListWithCapacity(1))
				errs := newProvisionErrors()
				job, err := clientset.BatchV1().Jobs("ns").Create(ctx, &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{Name: provisionJobName(name), Namespace: "ns"},
					Status:     batchv1.JobStatus{Active: 1},
				}, metav1.CreateOptions{})
				require.NoError(t, err)

				creates := 0
				oldNode, oldPVC := createDaemonOnNodeFunc, createDaemonOnPVCFunc
				t.Cleanup(func() { createDaemonOnNodeFunc, createDaemonOnPVCFunc = oldNode, oldPVC })
				create := func(_ *Cluster, osd *OSDInfo, nodeOrPVC string, _ *provisionConfig) error {
					creates++
					assert.Equal(t, 12, osd.ID)
					assert.Equal(t, name, nodeOrPVC)
					return nil
				}
				createDaemonOnNodeFunc, createDaemonOnPVCFunc = create, create

				process := func(status OrchestrationStatus) {
					UpdateNodeOrPVCStatus(ctx, c.kv, name, status)
					cm, err := clientset.CoreV1().ConfigMaps("ns").Get(ctx, cmName, metav1.GetOptions{})
					require.NoError(t, err)
					c.createOSDsForStatusMap(cm, config, errs)
				}
				process(OrchestrationStatus{Status: OrchestrationStatusFailed, PvcBackedOSD: pvcBacked, Message: "attempt interrupted"})
				assert.False(t, config.doneCreating(), "an attempt failure must not finish provisioning")
				assert.Zero(t, errs.len())
				_, err = clientset.CoreV1().ConfigMaps("ns").Get(ctx, cmName, metav1.GetOptions{})
				require.NoError(t, err, "retain the status while the Job can retry")

				// Backoff between attempts can have no active pod without being terminal.
				job.Status.Active = 0
				job.Status.Failed = 1
				job, err = clientset.BatchV1().Jobs("ns").UpdateStatus(ctx, job, metav1.UpdateOptions{})
				require.NoError(t, err)
				require.NoError(t, c.recheckPendingOSDStatuses(config, errs))
				assert.False(t, config.doneCreating())

				if succeeds {
					// A successful retry must be accepted even without an intermediate status event.
					completed := OrchestrationStatus{Status: OrchestrationStatusCompleted, PvcBackedOSD: pvcBacked, OSDs: []OSDInfo{{ID: 12}}}
					process(completed)
					assert.Equal(t, 1, creates)
					assert.Zero(t, errs.len())
					process(completed)
					assert.Equal(t, 1, creates, "duplicate success must not create OSDs again")
				} else {
					// Only the Job changes when retries are exhausted; no ConfigMap event arrives.
					job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}
					_, err = clientset.BatchV1().Jobs("ns").UpdateStatus(ctx, job, metav1.UpdateOptions{})
					require.NoError(t, err)
					require.NoError(t, c.recheckPendingOSDStatuses(config, errs))
					assert.Equal(t, 1, errs.len())
					assert.Zero(t, creates)
				}
				assert.True(t, config.doneCreating())
				_, err = clientset.CoreV1().ConfigMaps("ns").Get(ctx, cmName, metav1.GetOptions{})
				assert.True(t, errors.IsNotFound(err), "processed terminal results should be cleaned up")
			})
		}
	}
}
