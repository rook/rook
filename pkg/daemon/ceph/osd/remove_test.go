/*
Copyright 2022 The Rook Authors. All rights reserved.

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
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	testexec "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func TestGatherCleanupMetadata(t *testing.T) {
	ns := "testns"
	clusterInfo := client.AdminTestClusterInfo(ns)

	t.Run("gather metadata for data, metadata, and wal pvcs", func(t *testing.T) {
		clientset := testexec.New(t, 0)
		context := &clusterd.Context{Clientset: clientset}

		// Create PVs and PVCs with device set labels
		deviceSet := "mydata"
		setIndex := "0"
		labels := map[string]string{
			oposd.CephDeviceSetLabelKey: deviceSet,
			oposd.CephSetIndexLabelKey:  setIndex,
		}

		// Create PVs
		pv1 := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "pv-data"},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					Local: &corev1.LocalVolumeSource{Path: "/mnt/local-storage/localblock/vdb"},
				},
				NodeAffinity: &corev1.VolumeNodeAffinity{
					Required: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      corev1.LabelHostname,
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"node1"},
									},
								},
							},
						},
					},
				},
			},
		}
		pv2 := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "pv-wal"},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					Local: &corev1.LocalVolumeSource{Path: "/mnt/local-storage/localblock/vdc"},
				},
				NodeAffinity: &corev1.VolumeNodeAffinity{
					Required: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      corev1.LabelHostname,
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"node1"},
									},
								},
							},
						},
					},
				},
			},
		}
		_, _ = clientset.CoreV1().PersistentVolumes().Create(clusterInfo.Context, pv1, metav1.CreateOptions{})
		_, _ = clientset.CoreV1().PersistentVolumes().Create(clusterInfo.Context, pv2, metav1.CreateOptions{})

		// Create PVCs with matching labels
		pvc1 := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "data-pvc", Namespace: ns, Labels: labels},
			Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "pv-data"},
		}
		pvc2 := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "wal-pvc", Namespace: ns, Labels: labels},
			Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "pv-wal"},
		}
		_, _ = clientset.CoreV1().PersistentVolumeClaims(ns).Create(clusterInfo.Context, pvc1, metav1.CreateOptions{})
		_, _ = clientset.CoreV1().PersistentVolumeClaims(ns).Create(clusterInfo.Context, pvc2, metav1.CreateOptions{})

		// Gather cleanup metadata
		devices := gatherCleanupMetadata(context, clusterInfo, 5, "data-pvc", false)
		require.Equal(t, 2, len(devices))

		// Verify device information
		assert.Equal(t, "data-pvc", devices[0].pvcName)
		assert.Equal(t, "pv-data", devices[0].pvName)
		assert.Equal(t, "node1", devices[0].nodeName)
		assert.Equal(t, "/mnt/local-storage/localblock/vdb", devices[0].devicePath)
		assert.False(t, devices[0].encrypted)

		assert.Equal(t, "wal-pvc", devices[1].pvcName)
		assert.Equal(t, "pv-wal", devices[1].pvName)
		assert.Equal(t, "node1", devices[1].nodeName)
	})

	t.Run("gather metadata with encrypted osd", func(t *testing.T) {
		clientset := testexec.New(t, 0)
		context := &clusterd.Context{Clientset: clientset}

		// Create PV and PVC
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "pv-data"},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					Local: &corev1.LocalVolumeSource{Path: "/mnt/local-storage/localblock/vdb"},
				},
				NodeAffinity: &corev1.VolumeNodeAffinity{
					Required: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      corev1.LabelHostname,
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"node1"},
									},
								},
							},
						},
					},
				},
			},
		}
		_, _ = clientset.CoreV1().PersistentVolumes().Create(clusterInfo.Context, pv, metav1.CreateOptions{})

		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "encrypted-pvc",
				Namespace: ns,
				Labels: map[string]string{
					oposd.CephDeviceSetLabelKey: "mydata",
					oposd.CephSetIndexLabelKey:  "0",
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{VolumeName: "pv-data"},
		}
		_, _ = clientset.CoreV1().PersistentVolumeClaims(ns).Create(clusterInfo.Context, pvc, metav1.CreateOptions{})

		// Gather cleanup metadata with encrypted=true
		devices := gatherCleanupMetadata(context, clusterInfo, 5, "encrypted-pvc", true)
		require.Equal(t, 1, len(devices))

		// Verify encryption info
		assert.True(t, devices[0].encrypted)
		assert.NotEmpty(t, devices[0].dmName)
	})

	t.Run("pvc not found returns empty list", func(t *testing.T) {
		clientset := testexec.New(t, 0)
		context := &clusterd.Context{Clientset: clientset}

		// Try to gather metadata for nonexistent PVC
		devices := gatherCleanupMetadata(context, clusterInfo, 5, "nonexistent-pvc", false)
		require.Equal(t, 0, len(devices))
	})

	t.Run("error when pv cannot be fetched", func(t *testing.T) {
		clientset := testexec.New(t, 0)
		clientset.PrependReactor("get", "persistentvolumes", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, nil, errors.New("failed to get PV")
		})
		context := &clusterd.Context{Clientset: clientset}

		// Create PVC with device set labels
		labels := map[string]string{
			oposd.CephDeviceSetLabelKey: "mydata",
			oposd.CephSetIndexLabelKey:  "0",
		}
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "data-pvc", Namespace: ns, Labels: labels},
			Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "pv-data"},
		}
		_, _ = clientset.CoreV1().PersistentVolumeClaims(ns).Create(clusterInfo.Context, pvc, metav1.CreateOptions{})

		// Should handle error when PV cannot be fetched
		gatherCleanupMetadata(context, clusterInfo, 5, "data-pvc", false)
	})
}

func TestFenceDeployment(t *testing.T) {
	ns := "testns"
	clusterInfo := client.AdminTestClusterInfo(ns)

	t.Run("successfully fences deployment", func(t *testing.T) {
		clientset := testexec.New(t, 0)
		context := &clusterd.Context{Clientset: clientset}

		// Create deployment
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph-osd-5", Namespace: ns, Labels: map[string]string{}},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(int32(1))},
		}
		_, _ = clientset.AppsV1().Deployments(ns).Create(clusterInfo.Context, deployment, metav1.CreateOptions{})

		// Fence it
		fenceDeployment(context, clusterInfo, ns, "rook-ceph-osd-5", 5)

		// Verify it's fenced
		updated, err := clientset.AppsV1().Deployments(ns).Get(clusterInfo.Context, "rook-ceph-osd-5", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, "true", updated.Labels[cephv1.SkipReconcileLabelKey])
		assert.Equal(t, int32(0), *updated.Spec.Replicas)
	})

	t.Run("logs error if deployment not found", func(t *testing.T) {
		clientset := testexec.New(t, 0)
		context := &clusterd.Context{Clientset: clientset}

		// Try to fence nonexistent deployment (should log error, not panic)
		fenceDeployment(context, clusterInfo, ns, "nonexistent-deployment", 5)
		// If we get here, it logged the error successfully
	})
}

func TestRemovePVCs(t *testing.T) {
	ns := "testns"
	clusterInfo := client.AdminTestClusterInfo(ns)

	t.Run("delete pvcs from devices list", func(t *testing.T) {
		clientset := testexec.New(t, 0)
		context := &clusterd.Context{Clientset: clientset}

		// Create 3 PVCs
		pvc1 := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "data-pvc", Namespace: ns},
		}
		pvc2 := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "wal-pvc", Namespace: ns},
		}
		pvc3 := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "db-pvc", Namespace: ns},
		}
		_, _ = clientset.CoreV1().PersistentVolumeClaims(ns).Create(clusterInfo.Context, pvc1, metav1.CreateOptions{})
		_, _ = clientset.CoreV1().PersistentVolumeClaims(ns).Create(clusterInfo.Context, pvc2, metav1.CreateOptions{})
		_, _ = clientset.CoreV1().PersistentVolumeClaims(ns).Create(clusterInfo.Context, pvc3, metav1.CreateOptions{})

		// Create devices list
		devices := []pvcDeviceCleanup{
			{pvcName: "data-pvc", pvName: "pv-data"},
			{pvcName: "wal-pvc", pvName: "pv-wal"},
			{pvcName: "db-pvc", pvName: "pv-db"},
		}

		// Delete the PVCs
		removePVCs(context, clusterInfo, devices, false)

		// Verify all PVCs are deleted
		pvcs, err := clientset.CoreV1().PersistentVolumeClaims(ns).List(clusterInfo.Context, metav1.ListOptions{})
		assert.NoError(t, err)
		assert.Equal(t, 0, len(pvcs.Items))
	})

	t.Run("detach pvcs when preservePVC is true", func(t *testing.T) {
		clientset := testexec.New(t, 0)
		context := &clusterd.Context{Clientset: clientset}

		// Create PVC with device set label
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "data-pvc",
				Namespace: ns,
				Labels: map[string]string{
					oposd.CephDeviceSetPVCIDLabelKey: "device-id",
				},
			},
		}
		_, err := clientset.CoreV1().PersistentVolumeClaims(ns).Create(clusterInfo.Context, pvc, metav1.CreateOptions{})
		require.NoError(t, err)

		// Detach the PVC
		devices := []pvcDeviceCleanup{
			{pvcName: "data-pvc", pvName: "pv-data"},
		}
		removePVCs(context, clusterInfo, devices, true)

		// Verify PVC still exists but label removed
		retrieved, err := clientset.CoreV1().PersistentVolumeClaims(ns).Get(clusterInfo.Context, "data-pvc", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.NotContains(t, retrieved.Labels, oposd.CephDeviceSetPVCIDLabelKey)
	})

	t.Run("error when delete fails", func(t *testing.T) {
		clientset := testexec.New(t, 0)
		clientset.PrependReactor("delete", "persistentvolumeclaims", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, nil, errors.New("delete failed")
		})
		context := &clusterd.Context{Clientset: clientset}

		// Create a PVC
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "data-pvc", Namespace: ns},
		}
		_, _ = clientset.CoreV1().PersistentVolumeClaims(ns).Create(clusterInfo.Context, pvc, metav1.CreateOptions{})

		// Try to delete
		devices := []pvcDeviceCleanup{
			{pvcName: "data-pvc", pvName: "pv-data"},
		}
		removePVCs(context, clusterInfo, devices, false)
	})
}

func TestArchiveCrash(t *testing.T) {
	newExecutor := func(crashList string, archived *[]string) *exectest.MockExecutor {
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			if args[0] == "crash" && args[1] == "ls" {
				return crashList, nil
			}
			if args[0] == "crash" && args[1] == "archive" {
				*archived = append(*archived, args[2])
				return "", nil
			}
			return "", errors.Errorf("unexpected ceph command %q", args)
		}
		return executor
	}

	t.Run("archives the crash for the removed osd", func(t *testing.T) {
		crashList := `[
			{
				"crash_id": "2020-11-09_13:58:08.230130Z_ca918f58-c078-444d-a91a-bd972c14c155",
				"entity_name": "osd.1"
			},
			{
				"crash_id": "2020-11-09_13:58:08.230130Z_bd972c14-c155-444d-a91a-ca918f58c078",
				"entity_name": "osd.0"
			}
		]`
		var archived []string
		context := &clusterd.Context{Executor: newExecutor(crashList, &archived)}

		archiveCrash(context, client.AdminTestClusterInfo("mycluster"), 0)
		assert.Equal(t, []string{"2020-11-09_13:58:08.230130Z_bd972c14-c155-444d-a91a-ca918f58c078"}, archived)
	})

	t.Run("archives every crash for the removed osd", func(t *testing.T) {
		// ceph crash ls is oldest-first and includes already-archived entries, so an osd
		// with more than one crash must have all of them archived, not just the first match.
		crashList := `[
			{
				"crash_id": "2020-11-09_13:58:08.230130Z_00000000-0000-0000-0000-000000000000",
				"entity_name": "osd.0"
			},
			{
				"crash_id": "2020-11-09_13:58:08.230130Z_ca918f58-c078-444d-a91a-bd972c14c155",
				"entity_name": "osd.1"
			},
			{
				"crash_id": "2020-11-09_13:58:08.230130Z_bd972c14-c155-444d-a91a-ca918f58c078",
				"entity_name": "osd.0"
			}
		]`
		var archived []string
		context := &clusterd.Context{Executor: newExecutor(crashList, &archived)}

		archiveCrash(context, client.AdminTestClusterInfo("mycluster"), 0)
		assert.Equal(t, []string{
			"2020-11-09_13:58:08.230130Z_00000000-0000-0000-0000-000000000000",
			"2020-11-09_13:58:08.230130Z_bd972c14-c155-444d-a91a-ca918f58c078",
		}, archived)
	})

	t.Run("does not archive when there is no crash", func(t *testing.T) {
		var archived []string
		context := &clusterd.Context{Executor: newExecutor("[]", &archived)}

		archiveCrash(context, client.AdminTestClusterInfo("mycluster"), 0)
		assert.Empty(t, archived)
	})

	t.Run("does not archive when no crash matches the removed osd", func(t *testing.T) {
		crashList := `[
			{
				"crash_id": "2020-11-09_13:58:08.230130Z_ca918f58-c078-444d-a91a-bd972c14c155",
				"entity_name": "osd.1"
			}
		]`
		var archived []string
		context := &clusterd.Context{Executor: newExecutor(crashList, &archived)}

		archiveCrash(context, client.AdminTestClusterInfo("mycluster"), 0)
		assert.Empty(t, archived)
	})
}
