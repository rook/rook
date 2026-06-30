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
	"strconv"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestMakeCryptCloseJob(t *testing.T) {
	c := newTestReplaceCluster(fake.NewClientset())
	job := c.makeCryptCloseJob(7, "node-a")

	assert.Equal(t, "rook-ceph-osd-crypt-close-7", job.Name)
	assert.Equal(t, "rook-ceph", job.Namespace)
	assert.Equal(t, "7", job.Labels[OsdIdLabelKey])

	spec := job.Spec.Template.Spec
	// pinned to the OSD's node
	assert.Equal(t, "node-a", spec.NodeSelector[k8sutil.LabelHostname()])
	assert.Equal(t, v1.RestartPolicyOnFailure, spec.RestartPolicy)
	assert.Equal(t, k8sutil.DefaultServiceAccount, spec.ServiceAccountName)

	require.Len(t, spec.Containers, 1)
	container := spec.Containers[0]
	// runs the rook subcommand that resolves + closes the dm-crypt mappings by id
	assert.Equal(t, []string{"ceph", "osd", "close-encrypted-devices", "--osd-id", "7"}, container.Args)
	// uses the rook image (carries rook + ceph-volume + cryptsetup)
	assert.Equal(t, "rook/ceph:test", container.Image)
	// privileged root for device-mapper ops
	require.NotNil(t, container.SecurityContext)
	require.NotNil(t, container.SecurityContext.Privileged)
	assert.True(t, *container.SecurityContext.Privileged)

	// DM_DISABLE_UDEV must be set so cryptsetup doesn't hang on udev sync in a container
	var dmDisableUdev string
	for _, e := range container.Env {
		if e.Name == "DM_DISABLE_UDEV" {
			dmDisableUdev = e.Value
		}
	}
	assert.Equal(t, "1", dmDisableUdev)

	// /dev and /run/udev host mounts present
	mounts := map[string]string{}
	for _, m := range container.VolumeMounts {
		mounts[m.Name] = m.MountPath
	}
	assert.Equal(t, "/dev", mounts["devices"])
	assert.Equal(t, "/run/udev", mounts["run-udev"])

	vols := map[string]string{}
	for _, vol := range spec.Volumes {
		if vol.HostPath != nil {
			vols[vol.Name] = vol.HostPath.Path
		}
	}
	assert.Equal(t, "/dev", vols["devices"])
	assert.Equal(t, "/run/udev", vols["run-udev"])

	// cryptsetup needs the host IPC namespace for its udev semaphore
	assert.True(t, spec.HostIPC)
	// pod security context present for parity with the prepare/cleanup jobs
	require.NotNil(t, spec.SecurityContext)

	// fail-fast bounds so a stuck close surfaces as Failed instead of running forever
	require.NotNil(t, job.Spec.BackoffLimit)
	assert.Equal(t, int32(3), *job.Spec.BackoffLimit)
	require.NotNil(t, job.Spec.ActiveDeadlineSeconds)
	assert.Equal(t, int64(600), *job.Spec.ActiveDeadlineSeconds)

	// owner reference set so the Job is garbage-collected with the CephCluster
	assert.NotEmpty(t, job.OwnerReferences)
}

func TestMakeCryptCloseJobAppliesOSDPlacementTolerations(t *testing.T) {
	// An OSD node may be tainted; the close Job must tolerate it the same way the OSD daemon does, or
	// it would stay Pending forever.
	taintToleration := v1.Toleration{Key: "storage-node", Operator: v1.TolerationOpExists, Effect: v1.TaintEffectNoSchedule}
	spec := cephv1.ClusterSpec{
		Placement: cephv1.PlacementSpec{
			cephv1.KeyOSD: cephv1.Placement{Tolerations: []v1.Toleration{taintToleration}},
		},
	}
	c := newTestReplaceClusterWithSpec(fake.NewClientset(), spec)
	job := c.makeCryptCloseJob(2, "node-a")

	spc := job.Spec.Template.Spec
	// NodeSelector still pins to the exact node
	assert.Equal(t, "node-a", spc.NodeSelector[k8sutil.LabelHostname()])
	// and the OSD toleration is applied
	assert.Contains(t, spc.Tolerations, taintToleration)
}

func TestStartCryptCloseJob(t *testing.T) {
	t.Run("creates the job pinned to the node", func(t *testing.T) {
		clientset := fake.NewClientset()
		c := newTestReplaceCluster(clientset)

		err := c.startCryptCloseJob(3, "node-b")
		require.NoError(t, err)

		job, err := clientset.BatchV1().Jobs("rook-ceph").Get(context.TODO(), "rook-ceph-osd-crypt-close-3", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, "node-b", job.Spec.Template.Spec.NodeSelector[k8sutil.LabelHostname()])

		// idempotent: re-running replaces the job without error
		err = c.startCryptCloseJob(3, "node-b")
		assert.NoError(t, err)
	})

	t.Run("replaces an already-active job re-pinned to a different node", func(t *testing.T) {
		// A stale, still-running Job pinned to the wrong node must be replaced (deleteIfFound=true),
		// not left in place. This is the active-Job branch of RunReplaceableJob.
		existing := &batch.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph-osd-crypt-close-3", Namespace: "rook-ceph"},
			Spec: batch.JobSpec{
				Template: v1.PodTemplateSpec{
					Spec: v1.PodSpec{NodeSelector: map[string]string{k8sutil.LabelHostname(): "old-node"}},
				},
			},
			Status: batch.JobStatus{Active: 1},
		}
		clientset := fake.NewClientset(existing)
		c := newTestReplaceCluster(clientset)

		err := c.startCryptCloseJob(3, "new-node")
		require.NoError(t, err)

		job, err := clientset.BatchV1().Jobs("rook-ceph").Get(context.TODO(), "rook-ceph-osd-crypt-close-3", metav1.GetOptions{})
		require.NoError(t, err)
		// the replacement Job is pinned to the new node, proving the active job was replaced
		assert.Equal(t, "new-node", job.Spec.Template.Spec.NodeSelector[k8sutil.LabelHostname()])
	})
}

func TestCryptCloseJobStatusForOSD(t *testing.T) {
	jobName := "rook-ceph-osd-crypt-close-4"

	t.Run("not found", func(t *testing.T) {
		c := newTestReplaceCluster(fake.NewClientset())
		status, err := c.cryptCloseJobStatusForOSD(4)
		assert.NoError(t, err)
		assert.Equal(t, cryptCloseJobNotFound, status)
	})

	t.Run("running", func(t *testing.T) {
		job := &batch.Job{
			ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: "rook-ceph"},
			Status:     batch.JobStatus{Active: 1},
		}
		c := newTestReplaceCluster(fake.NewClientset(job))
		status, err := c.cryptCloseJobStatusForOSD(4)
		assert.NoError(t, err)
		assert.Equal(t, cryptCloseJobRunning, status)
	})

	t.Run("succeeded", func(t *testing.T) {
		job := &batch.Job{
			ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: "rook-ceph"},
			Status:     batch.JobStatus{Succeeded: 1},
		}
		c := newTestReplaceCluster(fake.NewClientset(job))
		status, err := c.cryptCloseJobStatusForOSD(4)
		assert.NoError(t, err)
		assert.Equal(t, cryptCloseJobSucceeded, status)
	})

	t.Run("failed", func(t *testing.T) {
		job := &batch.Job{
			ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: "rook-ceph"},
			Status: batch.JobStatus{
				Conditions: []batch.JobCondition{
					{Type: batch.JobFailed, Status: v1.ConditionTrue},
				},
			},
		}
		c := newTestReplaceCluster(fake.NewClientset(job))
		status, err := c.cryptCloseJobStatusForOSD(4)
		assert.NoError(t, err)
		assert.Equal(t, cryptCloseJobFailed, status)
	})
}

func TestDeleteCryptCloseJob(t *testing.T) {
	osdID := 9
	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph-osd-crypt-close-" + strconv.Itoa(osdID), Namespace: "rook-ceph"},
	}
	clientset := fake.NewClientset(job)
	c := newTestReplaceCluster(clientset)

	err := c.deleteCryptCloseJob(osdID)
	assert.NoError(t, err)

	// idempotent: deleting a non-existent job is a no-op
	err = c.deleteCryptCloseJob(osdID)
	assert.NoError(t, err)
}
