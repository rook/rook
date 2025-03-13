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

package nodedaemon

import (
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *ReconcileNode) reconcileCrashPruner(namespace string, cephCluster cephv1.CephCluster, tolerations []corev1.Toleration) error {
	if cephCluster.Spec.CrashCollector.Disable {
		logger.Debugf("crash collector is disabled in namespace %q so skipping crash retention reconcile", namespace)
		return nil
	}

	objectMeta := metav1.ObjectMeta{
		Name:      prunerName,
		Namespace: namespace,
	}

	if cephCluster.Spec.CrashCollector.DaysToRetain == 0 {
		logger.Debug("deleting cronjob if it exists...")

		cronJob := &v1.CronJob{ObjectMeta: objectMeta}

		err := r.client.Delete(r.opManagerContext, cronJob)
		if err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debug("cronJob resource not found. Ignoring since object must be deleted.")
			} else {
				return err
			}
		} else {
			logger.Debug("successfully deleted crash pruner cronjob.")
		}
	} else {
		logger.Debugf("daysToRetain set to: %d", cephCluster.Spec.CrashCollector.DaysToRetain)
		op, err := r.createOrUpdateCephCron(cephCluster, tolerations)
		if err != nil {
			return errors.Wrapf(err, "node reconcile failed on op %q", op)
		}
		logger.Debugf("cronjob successfully reconciled. operation: %q", op)
	}
	return nil
}

func (r *ReconcileNode) createOrUpdateCephCron(cephCluster cephv1.CephCluster, tolerations []corev1.Toleration) (controllerutil.OperationResult, error) {
	objectMeta := metav1.ObjectMeta{
		Name:      prunerName,
		Namespace: cephCluster.GetNamespace(),
	}
	// Adding volumes to pods containing data needed to connect to the ceph cluster.
	volumes := controller.DaemonVolumesBase(config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath), "", cephCluster.Spec.DataDirHostPath)
	volumes = append(volumes, keyring.Volume().CrashCollector())

	// labels for the pod, the deployment, and the deploymentSelector
	cronJobLabels := map[string]string{
		k8sutil.AppAttr: prunerName,
	}
	cronJobLabels[k8sutil.ClusterAttr] = cephCluster.GetNamespace()
	podTemplateSpec := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: cronJobLabels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				getCrashPruneContainer(cephCluster),
			},
			RestartPolicy:      corev1.RestartPolicyNever,
			HostNetwork:        cephCluster.Spec.Network.IsHost(),
			Volumes:            volumes,
			SecurityContext:    &corev1.PodSecurityContext{},
			ServiceAccountName: k8sutil.DefaultServiceAccount,
			Tolerations:        tolerations,
		},
	}

	// After 100 failures, the cron job will no longer run.
	// To avoid this, the cronjob is configured to only count the failures
	// that occurred in the last hour.
	deadline := int64(60)
	cronJob := &v1.CronJob{ObjectMeta: objectMeta}
	err := controllerutil.SetControllerReference(&cephCluster, cronJob, r.scheme)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Errorf("failed to set owner reference of deployment %q", cronJob.Name)
	}
	mutateFunc := func() error {
		cronJob.ObjectMeta.Labels = cronJobLabels
		cronJob.Spec.JobTemplate.Spec.Template = podTemplateSpec
		cronJob.Spec.Schedule = pruneSchedule
		cronJob.Spec.StartingDeadlineSeconds = &deadline

		return nil
	}

	return controllerutil.CreateOrUpdate(r.opManagerContext, r.client, cronJob, mutateFunc)
}

func getCrashPruneContainer(cephCluster cephv1.CephCluster) corev1.Container {
	envVars := append(controller.DaemonEnvVars(&cephCluster.Spec), generateCrashEnvVar())
	dataPathMap := config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath)
	volumeMounts := controller.DaemonVolumeMounts(dataPathMap, "", cephCluster.Spec.DataDirHostPath)
	volumeMounts = append(volumeMounts, keyring.VolumeMount().CrashCollector())

	container := corev1.Container{
		Name: "ceph-crash-pruner",
		Command: []string{
			"ceph",
			"-n",
			crashClient,
			"crash",
			"prune",
		},
		Args: []string{
			fmt.Sprintf("%d", cephCluster.Spec.CrashCollector.DaysToRetain),
		},
		Image:           cephCluster.Spec.CephVersion.Image,
		ImagePullPolicy: controller.GetContainerImagePullPolicy(cephCluster.Spec.CephVersion.ImagePullPolicy),
		Env:             envVars,
		VolumeMounts:    volumeMounts,
		Resources:       cephv1.GetCrashCollectorResources(cephCluster.Spec.Resources),
		SecurityContext: controller.PodSecurityContext(),
	}

	return container
}
