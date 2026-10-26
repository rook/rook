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
	"time"

	"github.com/pkg/errors"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	purgeNodeJobAppName = "rook-ceph-osd-purge"
	purgeNodeJobNameFmt = "rook-ceph-osd-purge-%d"
)

var (
	purgeNodeJobBackoffLimit          int32         = 3
	purgeNodeJobActiveDeadlineSeconds int64         = 600
	purgeNodeJobWaitTimeout                         = 10 * time.Minute
)

type purgeNodeJobConfig struct {
	namespace   string
	osdID       int
	nodeName    string // k8s node name from PV nodeAffinity, not the CRUSH host name
	symlinkPath string
	encrypted   bool
	dmName      string // "<pvcName>-block-dmcrypt"; empty when not encrypted
	rookImage   string
}

func purgeNodeJobName(osdID int) string {
	return fmt.Sprintf(purgeNodeJobNameFmt, osdID)
}

// purgeNodeJobStatus is the observed status of a per-OSD purge Job.
type purgeNodeJobStatus string

const (
	purgeJobNotFound  purgeNodeJobStatus = "NotFound"
	purgeJobRunning   purgeNodeJobStatus = "Running"
	purgeJobSucceeded purgeNodeJobStatus = "Succeeded"
	purgeJobFailed    purgeNodeJobStatus = "Failed"
)

// buildPurgeNodeJob returns the *batch.Job for the given config. It does not submit it.
func buildPurgeNodeJob(cfg purgeNodeJobConfig) *batch.Job {
	privileged := true
	uid := int64(0)
	secCtx := &v1.SecurityContext{
		Privileged: &privileged,
		RunAsUser:  &uid,
		RunAsGroup: &uid,
	}

	encryptedVal := "false"
	if cfg.encrypted {
		encryptedVal = "true"
	}

	envVars := []v1.EnvVar{
		// device-mapper operations from cryptsetup hang on udev sync in a container; disable it.
		{Name: "DM_DISABLE_UDEV", Value: "1"},
		{Name: "ENCRYPTED", Value: encryptedVal},
		{Name: "DM_NAME", Value: cfg.dmName},
		{Name: "SYMLINK_PATH", Value: cfg.symlinkPath},
	}

	script := `set -e

if [ "$ENCRYPTED" = "true" ] && [ -n "$DM_NAME" ]; then
    if cryptsetup status "$DM_NAME" > /dev/null 2>&1; then
        cryptsetup close "$DM_NAME"
    fi
fi

if [ -L "$SYMLINK_PATH" ]; then
    REAL_DEVICE=$(readlink -f "$SYMLINK_PATH")
    wipefs -a "$REAL_DEVICE"
    rm -f "$SYMLINK_PATH"
elif [ -n "$SYMLINK_PATH" ] && [ -b "$SYMLINK_PATH" ]; then
    wipefs -a "$SYMLINK_PATH"
fi`

	container := v1.Container{
		Name:            "purge",
		Image:           cfg.rookImage,
		Command:         []string{"/bin/sh", "-c", script},
		SecurityContext: secCtx,
		Env:             envVars,
		VolumeMounts: []v1.VolumeMount{
			{Name: "devices", MountPath: "/dev"},
			{Name: "run-udev", MountPath: "/run/udev"},
			{Name: "local-storage", MountPath: "/mnt/local-storage"},
		},
	}

	hostPathType := v1.HostPathDirectory
	volumes := []v1.Volume{
		{
			Name: "devices",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: "/dev", Type: &hostPathType},
			},
		},
		{
			Name: "run-udev",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: "/run/udev", Type: &hostPathType},
			},
		},
		{
			Name: "local-storage",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: "/mnt/local-storage", Type: &hostPathType},
			},
		},
	}

	labels := map[string]string{
		k8sutil.AppAttr:     purgeNodeJobAppName,
		k8sutil.ClusterAttr: cfg.namespace,
		oposd.OsdIdLabelKey: strconv.Itoa(cfg.osdID),
	}

	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name: purgeNodeJobAppName,
		},
		Spec: v1.PodSpec{
			Containers:         []v1.Container{container},
			Volumes:            volumes,
			RestartPolicy:      v1.RestartPolicyOnFailure,
			ServiceAccountName: k8sutil.DefaultServiceAccount,
			// cryptsetup synchronizes with udev on the host through a semaphore; share the host IPC
			// namespace so cryptsetup close can reach it.
			HostIPC: true,
			NodeSelector: map[string]string{
				k8sutil.LabelHostname(): cfg.nodeName,
			},
		},
	}

	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      purgeNodeJobName(cfg.osdID),
			Namespace: cfg.namespace,
			Labels:    labels,
		},
		Spec: batch.JobSpec{
			BackoffLimit:          &purgeNodeJobBackoffLimit,
			ActiveDeadlineSeconds: &purgeNodeJobActiveDeadlineSeconds,
			Template:              podSpec,
		},
	}
	k8sutil.AddRookVersionLabelToJob(job)
	return job
}

// createAndRunPurgeNodeJob submits the node-pinned purge Job, replacing any existing one.
func createAndRunPurgeNodeJob(ctx context.Context, clientset kubernetes.Interface, cfg purgeNodeJobConfig) error {
	job := buildPurgeNodeJob(cfg)
	if err := k8sutil.RunReplaceableJob(ctx, clientset, job, true); err != nil {
		return errors.Wrapf(err, "failed to create purge node job for osd.%d on node %q", cfg.osdID, cfg.nodeName)
	}
	logger.Infof("created purge node job %q for osd.%d on node %q", job.Name, cfg.osdID, cfg.nodeName)
	return nil
}

// getPurgeNodeJobStatus returns the current observed status of the purge Job for the given OSD.
func getPurgeNodeJobStatus(ctx context.Context, clientset kubernetes.Interface, namespace string, osdID int) (purgeNodeJobStatus, error) {
	job, err := clientset.BatchV1().Jobs(namespace).Get(ctx, purgeNodeJobName(osdID), metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return purgeJobNotFound, nil
		}
		return "", errors.Wrapf(err, "failed to get purge node job for osd.%d", osdID)
	}

	if job.Status.Succeeded > 0 {
		return purgeJobSucceeded, nil
	}
	for _, condition := range job.Status.Conditions {
		if condition.Type == batch.JobFailed && condition.Status == v1.ConditionTrue {
			return purgeJobFailed, nil
		}
	}
	return purgeJobRunning, nil
}

// waitForPurgeNodeJob polls until the purge Job succeeds or fails, timing out after 10 minutes.
func waitForPurgeNodeJob(ctx context.Context, clientset kubernetes.Interface, namespace string, osdID int) error {
	deadline := time.Now().Add(purgeNodeJobWaitTimeout)
	for time.Now().Before(deadline) {
		status, err := getPurgeNodeJobStatus(ctx, clientset, namespace, osdID)
		if err != nil {
			return errors.Wrapf(err, "error polling purge node job for osd.%d", osdID)
		}
		switch status {
		case purgeJobSucceeded:
			logger.Infof("purge node job for osd.%d completed successfully", osdID)
			_ = deletePurgeNodeJob(ctx, clientset, namespace, osdID)
			return nil
		case purgeJobFailed:
			return errors.Errorf("purge node job for osd.%d failed; inspect job %q for details", osdID, purgeNodeJobName(osdID))
		case purgeJobNotFound:
			logger.Warningf("purge node job for osd.%d not yet visible, may still be starting", osdID)
		default:
			logger.Infof("purge node job for osd.%d is running, waiting...", osdID)
		}
		time.Sleep(10 * time.Second)
	}
	return errors.Errorf("timed out waiting for purge node job for osd.%d", osdID)
}

// deletePurgeNodeJob removes the purge Job for the given OSD. No-op if the Job does not exist.
func deletePurgeNodeJob(ctx context.Context, clientset kubernetes.Interface, namespace string, osdID int) error {
	return k8sutil.DeleteBatchJob(ctx, clientset, namespace, purgeNodeJobName(osdID), false)
}
