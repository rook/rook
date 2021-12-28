/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package crash

import (
	"fmt"
	"path"

	"k8s.io/api/batch/v1beta1"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/ceph/controller"

	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	crashCollectorKeyringUsername = "client.crash"
	crashCollectorKeyName         = "rook-ceph-crash-collector-keyring"
	// pruneSchedule is scheduled to run every day at midnight.
	pruneSchedule = "0 0 * * *"
)

// createOrUpdateCephCrash is a wrapper around controllerutil.CreateOrUpdate
func (r *ReconcileNode) createOrUpdateCephCrash(node corev1.Node, tolerations []corev1.Toleration, cephCluster cephv1.CephCluster, cephVersion *cephver.CephVersion) (controllerutil.OperationResult, error) {
	// Create or Update the deployment default/foo
	nodeHostnameLabel, ok := node.ObjectMeta.Labels[corev1.LabelHostname]
	if !ok {
		return controllerutil.OperationResultNone, errors.Errorf("label key %q does not exist on node %q", corev1.LabelHostname, node.GetName())
	}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sutil.TruncateNodeName(fmt.Sprintf("%s-%%s", AppName), nodeHostnameLabel),
			Namespace: cephCluster.GetNamespace(),
		},
	}
	err := controllerutil.SetControllerReference(&cephCluster, deploy, r.scheme)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Errorf("failed to set owner reference of crashcollector deployment %q", deploy.Name)
	}

	volumes := controller.DaemonVolumesBase(config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath), "")
	volumes = append(volumes, keyring.Volume().CrashCollector())

	mutateFunc := func() error {

		// labels for the pod, the deployment, and the deploymentSelector
		deploymentLabels := map[string]string{
			corev1.LabelHostname: nodeHostnameLabel,
			k8sutil.AppAttr:      AppName,
			NodeNameLabel:        node.GetName(),
		}
		deploymentLabels[config.CrashType] = "crash"
		deploymentLabels[controller.DaemonIDLabel] = "crash"
		deploymentLabels[k8sutil.ClusterAttr] = cephCluster.GetNamespace()

		selectorLabels := map[string]string{
			corev1.LabelHostname: nodeHostnameLabel,
			k8sutil.AppAttr:      AppName,
			NodeNameLabel:        node.GetName(),
		}

		nodeSelector := map[string]string{corev1.LabelHostname: nodeHostnameLabel}

		// Deployment selector is immutable so we set this value only if
		// a new object is going to be created
		if deploy.ObjectMeta.CreationTimestamp.IsZero() {
			deploy.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			}
		}

		deploy.ObjectMeta.Labels = deploymentLabels
		cephv1.GetCrashCollectorLabels(cephCluster.Spec.Labels).ApplyToObjectMeta(&deploy.ObjectMeta)
		k8sutil.AddRookVersionLabelToDeployment(deploy)
		if cephVersion != nil {
			controller.AddCephVersionLabelToDeployment(*cephVersion, deploy)
		}
		deploy.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: deploymentLabels,
			},
			Spec: corev1.PodSpec{
				NodeSelector: nodeSelector,
				InitContainers: []corev1.Container{
					getCrashDirInitContainer(cephCluster),
					getCrashChownInitContainer(cephCluster),
				},
				Containers: []corev1.Container{
					getCrashDaemonContainer(cephCluster, *cephVersion),
				},
				Tolerations:       tolerations,
				RestartPolicy:     corev1.RestartPolicyAlways,
				HostNetwork:       cephCluster.Spec.Network.IsHost(),
				Volumes:           volumes,
				PriorityClassName: cephv1.GetCrashCollectorPriorityClassName(cephCluster.Spec.PriorityClassNames),
			},
		}

		return nil
	}

	return controllerutil.CreateOrUpdate(r.opManagerContext, r.client, deploy, mutateFunc)
}

// createOrUpdateCephCron is a wrapper around controllerutil.CreateOrUpdate
func (r *ReconcileNode) createOrUpdateCephCron(cephCluster cephv1.CephCluster, cephVersion *cephver.CephVersion, useCronJobV1 bool) (controllerutil.OperationResult, error) {
	objectMeta := metav1.ObjectMeta{
		Name:      prunerName,
		Namespace: cephCluster.GetNamespace(),
	}
	// Adding volumes to pods containing data needed to connect to the ceph cluster.
	volumes := controller.DaemonVolumesBase(config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath), "")
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
				getCrashPruneContainer(cephCluster, *cephVersion),
			},
			RestartPolicy: corev1.RestartPolicyNever,
			HostNetwork:   cephCluster.Spec.Network.IsHost(),
			Volumes:       volumes,
		},
	}

	// After 100 failures, the cron job will no longer run.
	// To avoid this, the cronjob is configured to only count the failures
	// that occurred in the last hour.
	deadline := int64(60)

	// minimum k8s version required for v1 cronJob is 'v1.21.0'. Apply v1 if k8s version is at least 'v1.21.0', else apply v1beta1 cronJob.
	if useCronJobV1 {
		// delete v1beta1 cronJob if it already exists
		err := r.client.Delete(r.opManagerContext, &v1beta1.CronJob{ObjectMeta: objectMeta})
		if err != nil && !apierrors.IsNotFound(err) {
			return controllerutil.OperationResultNone, errors.Wrapf(err, "failed to delete CronJob v1Beta1 %q", prunerName)
		}

		cronJob := &v1.CronJob{ObjectMeta: objectMeta}
		err = controllerutil.SetControllerReference(&cephCluster, cronJob, r.scheme)
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
	cronJob := &v1beta1.CronJob{ObjectMeta: objectMeta}
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

func getCrashDirInitContainer(cephCluster cephv1.CephCluster) corev1.Container {
	dataPathMap := config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath)
	crashPostedDir := path.Join(dataPathMap.ContainerCrashDir(), "posted")

	container := corev1.Container{
		Name: "make-container-crash-dir",
		Command: []string{
			"mkdir",
			"-p",
		},
		Args: []string{
			crashPostedDir,
		},
		Image:           cephCluster.Spec.CephVersion.Image,
		SecurityContext: controller.PodSecurityContext(),
		Resources:       cephv1.GetCrashCollectorResources(cephCluster.Spec.Resources),
		VolumeMounts:    controller.DaemonVolumeMounts(dataPathMap, ""),
	}
	return container
}

func getCrashChownInitContainer(cephCluster cephv1.CephCluster) corev1.Container {
	dataPathMap := config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath)

	return controller.ChownCephDataDirsInitContainer(
		*dataPathMap,
		cephCluster.Spec.CephVersion.Image,
		controller.DaemonVolumeMounts(dataPathMap, ""),
		cephv1.GetCrashCollectorResources(cephCluster.Spec.Resources),
		controller.PodSecurityContext(),
	)
}

func getCrashDaemonContainer(cephCluster cephv1.CephCluster, cephVersion cephver.CephVersion) corev1.Container {
	cephImage := cephCluster.Spec.CephVersion.Image
	dataPathMap := config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath)
	crashEnvVar := generateCrashEnvVar()
	envVars := append(controller.DaemonEnvVars(cephImage), crashEnvVar)
	volumeMounts := controller.DaemonVolumeMounts(dataPathMap, "")
	volumeMounts = append(volumeMounts, keyring.VolumeMount().CrashCollector())

	container := corev1.Container{
		Name: "ceph-crash",
		Command: []string{
			"ceph-crash",
		},
		Image:           cephImage,
		Env:             envVars,
		VolumeMounts:    volumeMounts,
		Resources:       cephv1.GetCrashCollectorResources(cephCluster.Spec.Resources),
		SecurityContext: controller.PodSecurityContext(),
	}

	return container
}

func getCrashPruneContainer(cephCluster cephv1.CephCluster, cephVersion cephver.CephVersion) corev1.Container {
	cephImage := cephCluster.Spec.CephVersion.Image
	envVars := append(controller.DaemonEnvVars(cephImage), generateCrashEnvVar())
	dataPathMap := config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath)
	volumeMounts := controller.DaemonVolumeMounts(dataPathMap, "")
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
		Image:           cephImage,
		Env:             envVars,
		VolumeMounts:    volumeMounts,
		Resources:       cephv1.GetCrashCollectorResources(cephCluster.Spec.Resources),
		SecurityContext: controller.PodSecurityContext(),
	}

	return container
}

func generateCrashEnvVar() corev1.EnvVar {
	val := fmt.Sprintf("-m $(ROOK_CEPH_MON_HOST) -k %s", keyring.VolumeMount().CrashCollectorKeyringFilePath())
	env := corev1.EnvVar{Name: "CEPH_ARGS", Value: val}

	return env
}
