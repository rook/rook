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
	"testing"

	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestBuildPurgeNodeJob(t *testing.T) {
	cfg := purgeNodeJobConfig{
		namespace:   "rook-ceph",
		osdID:       5,
		nodeName:    "worker-0",
		symlinkPath: "/mnt/local-storage/localblock/sdb",
		encrypted:   false,
		dmName:      "",
		rookImage:   "rook/ceph:test",
	}
	job := buildPurgeNodeJob(cfg)

	assert.Equal(t, "rook-ceph-osd-purge-5", job.Name)
	assert.Equal(t, "rook-ceph", job.Namespace)
	assert.Equal(t, "5", job.Labels[oposd.OsdIdLabelKey])

	spec := job.Spec.Template.Spec
	assert.Equal(t, "worker-0", spec.NodeSelector[k8sutil.LabelHostname()])
	assert.Equal(t, v1.RestartPolicyOnFailure, spec.RestartPolicy)
	assert.Equal(t, k8sutil.DefaultServiceAccount, spec.ServiceAccountName)
	assert.True(t, spec.HostIPC)

	require.Len(t, spec.Containers, 1)
	container := spec.Containers[0]
	assert.Equal(t, "rook/ceph:test", container.Image)
	require.Len(t, container.Command, 3)
	assert.Equal(t, "/bin/bash", container.Command[0])
	assert.Equal(t, "-c", container.Command[1])

	require.NotNil(t, container.SecurityContext)
	require.NotNil(t, container.SecurityContext.Privileged)
	assert.True(t, *container.SecurityContext.Privileged)
	require.NotNil(t, container.SecurityContext.RunAsUser)
	assert.Equal(t, int64(0), *container.SecurityContext.RunAsUser)

	envMap := map[string]string{}
	for _, e := range container.Env {
		envMap[e.Name] = e.Value
	}
	assert.Equal(t, "1", envMap["DM_DISABLE_UDEV"])
	assert.Equal(t, "false", envMap["ENCRYPTED"])
	assert.Equal(t, "", envMap["DM_NAME"])
	assert.Equal(t, "/mnt/local-storage/localblock/sdb", envMap["SYMLINK_PATH"])

	mounts := map[string]string{}
	for _, m := range container.VolumeMounts {
		mounts[m.Name] = m.MountPath
	}
	assert.Equal(t, "/dev", mounts["devices"])
	assert.Equal(t, "/run/udev", mounts["run-udev"])
	assert.Equal(t, "/mnt/local-storage", mounts["local-storage"])

	vols := map[string]string{}
	for _, vol := range spec.Volumes {
		if vol.HostPath != nil {
			vols[vol.Name] = vol.HostPath.Path
		}
	}
	assert.Equal(t, "/dev", vols["devices"])
	assert.Equal(t, "/run/udev", vols["run-udev"])
	assert.Equal(t, "/mnt/local-storage", vols["local-storage"])

	require.NotNil(t, job.Spec.BackoffLimit)
	assert.Equal(t, int32(3), *job.Spec.BackoffLimit)
	require.NotNil(t, job.Spec.ActiveDeadlineSeconds)
	assert.Equal(t, int64(600), *job.Spec.ActiveDeadlineSeconds)
}

func TestBuildPurgeNodeJobEncrypted(t *testing.T) {
	cfg := purgeNodeJobConfig{
		namespace:   "rook-ceph",
		osdID:       3,
		nodeName:    "worker-1",
		symlinkPath: "/mnt/local-storage/localblock/sdc",
		encrypted:   true,
		dmName:      "my-pvc-block-dmcrypt",
		rookImage:   "rook/ceph:test",
	}
	job := buildPurgeNodeJob(cfg)

	container := job.Spec.Template.Spec.Containers[0]
	envMap := map[string]string{}
	for _, e := range container.Env {
		envMap[e.Name] = e.Value
	}
	assert.Equal(t, "true", envMap["ENCRYPTED"])
	assert.Equal(t, "my-pvc-block-dmcrypt", envMap["DM_NAME"])
}

func TestCreateAndRunPurgeNodeJob(t *testing.T) {
	const ns = "rook-ceph"

	newCfg := func(node string) purgeNodeJobConfig {
		return purgeNodeJobConfig{
			namespace: ns,
			osdID:     5,
			nodeName:  node,
			rookImage: "rook/ceph:test",
		}
	}

	t.Run("creates the job pinned to the node", func(t *testing.T) {
		clientset := fake.NewClientset()
		err := createAndRunPurgeNodeJob(t.Context(), clientset, newCfg("worker-0"))
		require.NoError(t, err)

		job, err := clientset.BatchV1().Jobs(ns).Get(t.Context(), purgeNodeJobName(5), metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, "worker-0", job.Spec.Template.Spec.NodeSelector[k8sutil.LabelHostname()])
	})

	t.Run("idempotent: re-running does not error", func(t *testing.T) {
		clientset := fake.NewClientset()
		require.NoError(t, createAndRunPurgeNodeJob(t.Context(), clientset, newCfg("worker-0")))
		assert.NoError(t, createAndRunPurgeNodeJob(t.Context(), clientset, newCfg("worker-0")))
	})

	t.Run("replaces an existing active job pinned to a different node", func(t *testing.T) {
		existing := &batch.Job{
			ObjectMeta: metav1.ObjectMeta{Name: purgeNodeJobName(5), Namespace: ns},
			Spec: batch.JobSpec{
				Template: v1.PodTemplateSpec{
					Spec: v1.PodSpec{
						NodeSelector: map[string]string{k8sutil.LabelHostname(): "old-node"},
					},
				},
			},
			Status: batch.JobStatus{Active: 1},
		}
		clientset := fake.NewClientset(existing)
		err := createAndRunPurgeNodeJob(t.Context(), clientset, newCfg("new-node"))
		require.NoError(t, err)

		job, err := clientset.BatchV1().Jobs(ns).Get(t.Context(), purgeNodeJobName(5), metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, "new-node", job.Spec.Template.Spec.NodeSelector[k8sutil.LabelHostname()])
	})
}

func TestGetPurgeNodeJobStatus(t *testing.T) {
	const (
		ns    = "rook-ceph"
		osdID = 4
	)
	jobName := purgeNodeJobName(osdID)

	t.Run("not found", func(t *testing.T) {
		status, err := getPurgeNodeJobStatus(t.Context(), fake.NewClientset(), ns, osdID)
		assert.NoError(t, err)
		assert.Equal(t, purgeJobNotFound, status)
	})

	t.Run("running", func(t *testing.T) {
		job := &batch.Job{
			ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: ns},
			Status:     batch.JobStatus{Active: 1},
		}
		status, err := getPurgeNodeJobStatus(t.Context(), fake.NewClientset(job), ns, osdID)
		assert.NoError(t, err)
		assert.Equal(t, purgeJobRunning, status)
	})

	t.Run("succeeded", func(t *testing.T) {
		job := &batch.Job{
			ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: ns},
			Status:     batch.JobStatus{Succeeded: 1},
		}
		status, err := getPurgeNodeJobStatus(t.Context(), fake.NewClientset(job), ns, osdID)
		assert.NoError(t, err)
		assert.Equal(t, purgeJobSucceeded, status)
	})

	t.Run("failed", func(t *testing.T) {
		job := &batch.Job{
			ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: ns},
			Status: batch.JobStatus{
				Conditions: []batch.JobCondition{
					{Type: batch.JobFailed, Status: v1.ConditionTrue},
				},
			},
		}
		status, err := getPurgeNodeJobStatus(t.Context(), fake.NewClientset(job), ns, osdID)
		assert.NoError(t, err)
		assert.Equal(t, purgeJobFailed, status)
	})
}

func TestWaitForPurgeNodeJob(t *testing.T) {
	const (
		ns    = "rook-ceph"
		osdID = 6
	)
	jobName := purgeNodeJobName(osdID)

	t.Run("returns nil and deletes the job on success", func(t *testing.T) {
		job := &batch.Job{
			ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: ns},
			Status:     batch.JobStatus{Succeeded: 1},
		}
		clientset := fake.NewClientset(job)

		err := waitForPurgeNodeJob(t.Context(), clientset, ns, osdID)
		assert.NoError(t, err)

		_, err = clientset.BatchV1().Jobs(ns).Get(t.Context(), jobName, metav1.GetOptions{})
		assert.True(t, kerrors.IsNotFound(err), "job should be deleted after success")
	})

	t.Run("returns error on job failure", func(t *testing.T) {
		job := &batch.Job{
			ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: ns},
			Status: batch.JobStatus{
				Conditions: []batch.JobCondition{
					{Type: batch.JobFailed, Status: v1.ConditionTrue},
				},
			},
		}
		err := waitForPurgeNodeJob(t.Context(), fake.NewClientset(job), ns, osdID)
		assert.Error(t, err)
	})
}

func TestDeletePurgeNodeJob(t *testing.T) {
	const ns = "rook-ceph"
	osdID := 9
	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{Name: purgeNodeJobName(osdID), Namespace: ns},
	}
	clientset := fake.NewClientset(job)

	err := deletePurgeNodeJob(t.Context(), clientset, ns, osdID)
	assert.NoError(t, err)

	// idempotent: deleting a non-existent job is a no-op
	err = deletePurgeNodeJob(t.Context(), clientset, ns, osdID)
	assert.NoError(t, err)
}
