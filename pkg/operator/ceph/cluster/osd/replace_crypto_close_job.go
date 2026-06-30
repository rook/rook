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
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	cryptCloseJobAppName = "rook-ceph-osd-crypt-close"
	// cryptCloseJobNameFmt embeds the OSD id so each replacement gets its own replaceable Job.
	cryptCloseJobNameFmt    = "rook-ceph-osd-crypt-close-%d"
	cryptCloseContainerName = "crypt-close"
)

var (
	cryptCloseJobBackoffLimit int32 = 3
	// cryptCloseJobActiveDeadlineSeconds bounds the Job lifetime (including time spent Pending) so it
	// surfaces as Failed rather than running forever.
	cryptCloseJobActiveDeadlineSeconds int64 = 600
)

func cryptCloseJobName(osdID int) string {
	return fmt.Sprintf(cryptCloseJobNameFmt, osdID)
}

// cryptCloseJobStatus is the status of a per-OSD crypto-close Job, polled by the health-monitor
// goroutine across ticks.
type cryptCloseJobStatus string

const (
	cryptCloseJobNotFound  cryptCloseJobStatus = "NotFound"
	cryptCloseJobRunning   cryptCloseJobStatus = "Running"
	cryptCloseJobSucceeded cryptCloseJobStatus = "Succeeded"
	cryptCloseJobFailed    cryptCloseJobStatus = "Failed"
)

// startCryptCloseJob creates (or replaces) the privileged Job, pinned to the OSD's node, that closes
// the OSD's host dm-crypt mappings. The operator pod cannot run host device-mapper operations itself.
func (c *Cluster) startCryptCloseJob(osdID int, nodeName string) error {
	job := c.makeCryptCloseJob(osdID, nodeName)

	// deleteIfFound=true: replace any existing Job (even an active one) so a re-pinned Job always wins.
	if err := k8sutil.RunReplaceableJob(c.clusterInfo.Context, c.context.Clientset, job, true); err != nil {
		return errors.Wrapf(err, "failed to run crypto-close job for osd.%d on node %q", osdID, nodeName)
	}
	return nil
}

func (c *Cluster) cryptCloseJobStatusForOSD(osdID int) (cryptCloseJobStatus, error) {
	job, err := c.context.Clientset.BatchV1().Jobs(c.clusterInfo.Namespace).Get(c.clusterInfo.Context, cryptCloseJobName(osdID), metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return cryptCloseJobNotFound, nil
		}
		return "", errors.Wrapf(err, "failed to get crypto-close job for osd.%d", osdID)
	}

	if job.Status.Succeeded > 0 {
		return cryptCloseJobSucceeded, nil
	}
	for _, condition := range job.Status.Conditions {
		if condition.Type == batch.JobFailed && condition.Status == v1.ConditionTrue {
			return cryptCloseJobFailed, nil
		}
	}
	return cryptCloseJobRunning, nil
}

// deleteCryptCloseJob removes the per-OSD crypto-close Job. It is a no-op if the Job does not exist.
func (c *Cluster) deleteCryptCloseJob(osdID int) error {
	return k8sutil.DeleteBatchJob(c.clusterInfo.Context, c.context.Clientset, c.clusterInfo.Namespace, cryptCloseJobName(osdID), false)
}

func (c *Cluster) makeCryptCloseJob(osdID int, nodeName string) *batch.Job {
	podSpec := c.cryptCloseJobPodTemplateSpec(osdID)
	podSpec.Spec.NodeSelector = map[string]string{k8sutil.LabelHostname(): nodeName}

	labels := controller.AppLabels(cryptCloseJobAppName, c.clusterInfo.Namespace)
	labels[OsdIdLabelKey] = strconv.Itoa(osdID)

	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cryptCloseJobName(osdID),
			Namespace: c.clusterInfo.Namespace,
			Labels:    labels,
		},
		Spec: batch.JobSpec{
			// Bound retries and lifetime so a close that cannot succeed surfaces as Failed for the
			// health-monitor goroutine instead of running forever.
			BackoffLimit:          &cryptCloseJobBackoffLimit,
			ActiveDeadlineSeconds: &cryptCloseJobActiveDeadlineSeconds,
			Template:              podSpec,
		},
	}

	k8sutil.AddRookVersionLabelToJob(job)
	if c.clusterInfo.OwnerInfo != nil {
		// Best-effort owner reference so the Job is garbage-collected with the CephCluster; failure
		// is non-fatal because the Job is also explicitly deleted by deleteCryptCloseJob.
		if err := c.clusterInfo.OwnerInfo.SetControllerReference(job); err != nil {
			logger.Warningf("failed to set owner reference on crypto-close job for osd.%d. %v", osdID, err)
		}
	}

	return job
}

func (c *Cluster) cryptCloseJobContainer(osdID int) v1.Container {
	envVars := []v1.EnvVar{
		{Name: "ROOK_LOG_LEVEL", Value: "DEBUG"},
		// device-mapper operations from cryptsetup hang on udev sync in a container; disable it as
		// the cleanup job does.
		{Name: "DM_DISABLE_UDEV", Value: "1"},
	}
	if controller.LoopDevicesAllowed() {
		envVars = append(envVars, v1.EnvVar{Name: "CEPH_VOLUME_ALLOW_LOOP_DEVICES", Value: "true"})
	}

	return v1.Container{
		Name: cryptCloseContainerName,
		// The rook image carries the rook binary as well as ceph-volume and cryptsetup (it is built on
		// the ceph base image), so it can both resolve the dm-crypt mappings and close them.
		Image: c.rookVersion,
		// Run as UID 0: device-mapper and cryptsetup require root, matching the cleanup job.
		SecurityContext: controller.PrivilegedContext(true),
		VolumeMounts: []v1.VolumeMount{
			{Name: "devices", MountPath: "/dev"},
			{Name: "run-udev", MountPath: "/run/udev"},
		},
		Env:  envVars,
		Args: []string{"ceph", "osd", "close-encrypted-devices", "--osd-id", strconv.Itoa(osdID)},
	}
}

func (c *Cluster) cryptCloseJobPodTemplateSpec(osdID int) v1.PodTemplateSpec {
	volumes := []v1.Volume{
		{Name: "devices", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/dev"}}},
		{Name: "run-udev", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/run/udev"}}},
	}

	podSpec := v1.PodSpec{
		Containers:         []v1.Container{c.cryptCloseJobContainer(osdID)},
		Volumes:            volumes,
		RestartPolicy:      v1.RestartPolicyOnFailure,
		PriorityClassName:  cephv1.GetOSDPriorityClassName(c.spec.PriorityClassNames),
		ServiceAccountName: k8sutil.DefaultServiceAccount,
		HostNetwork:        controller.EnforceHostNetwork(),
		SecurityContext:    &v1.PodSecurityContext{},
		// cryptsetup synchronizes with udev on the host through a semaphore; share the host IPC
		// namespace so luksClose can reach it, matching the OSD prepare and key-rotation jobs.
		HostIPC: true,
	}

	// Apply the OSD placement so the pod tolerates the same node taints the OSD daemon and prepare
	// jobs do; without this it could stay Pending forever on a tainted storage node. The hostname
	// NodeSelector (set by makeCryptCloseJob) still pins it to the exact OSD node, and tolerations are
	// additive, so this only widens what the pod tolerates, never where it lands.
	c.applyAllPlacementIfNeeded(&podSpec)
	c.spec.Placement[cephv1.KeyOSD].ApplyToPodSpec(&podSpec)

	return v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name: cryptCloseJobAppName,
		},
		Spec: podSpec,
	}
}
