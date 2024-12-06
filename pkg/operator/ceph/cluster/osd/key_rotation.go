/*
Copyright 2023 The Rook Authors. All rights reserved.

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
	"path/filepath"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	kms "github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrl "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	keyRotationCronJobAppName    = "rook-ceph-osd-key-rotation"
	keyRotationCronJobAppNameFmt = "rook-ceph-osd-key-rotation-%d"
)

// keyRotationCronJobName returns the name of the key rotation cron job for the given OSD ID.
func keyRotationCronJobName(osdID int) string {
	return fmt.Sprintf(keyRotationCronJobAppNameFmt, osdID)
}

// applyKeyRotationPlacement applies the placement settings for the key rotation job
// so that it is scheduled on the same node as the OSD for which the key rotation is scheduled.
func applyKeyRotationPlacement(spec *v1.PodSpec, labels map[string]string) {
	spec.TopologySpreadConstraints = nil
	if spec.Affinity == nil {
		spec.Affinity = &v1.Affinity{}
	} else {
		spec.Affinity.PodAntiAffinity = nil
	}
	spec.Affinity.PodAffinity = &v1.PodAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
			{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
				TopologyKey: v1.LabelHostname,
			},
		},
	}
}

// getKeyRotationContainer returns the container spec for the key rotation job.
func (c *Cluster) getKeyRotationContainer(osdProps osdProperties, volumeMounts []v1.VolumeMount, devices []string) (v1.Container, error) {
	envVars := c.getConfigEnvVars(osdProps, k8sutil.DataDir, true)

	// enable debug logging
	envVars = append(envVars, setDebugLogLevelEnvVar(true))
	envVars = append(envVars, v1.EnvVar{Name: "ROOK_CEPH_VERSION", Value: c.clusterInfo.CephVersion.CephVersionFormatted()})
	envVars = append(envVars, kms.ConfigToEnvVar(c.spec)...)

	// run privileged always since we always mount /dev
	privileged := true
	runAsUser := int64(0)
	runAsNonRoot := false
	readOnlyRootFilesystem := false

	args := []string{"key-management", "rotate-key", osdProps.pvc.ClaimName}
	args = append(args, devices...)

	osdProvisionContainer := v1.Container{
		Args:            args,
		Name:            keyRotationCronJobAppName,
		Image:           c.rookVersion,
		ImagePullPolicy: controller.GetContainerImagePullPolicy(c.spec.CephVersion.ImagePullPolicy),
		VolumeMounts:    volumeMounts,
		Env:             envVars,
		EnvFrom:         getEnvFromSources(),
		SecurityContext: &v1.SecurityContext{
			Privileged:             &privileged,
			RunAsUser:              &runAsUser,
			RunAsNonRoot:           &runAsNonRoot,
			ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
			Capabilities: &v1.Capabilities{
				Add: []v1.Capability{},
				Drop: []v1.Capability{
					"NET_RAW",
				},
			},
		},
		Resources: osdProps.resources,
	}

	return osdProvisionContainer, nil
}

// getKeyRotationPodTemplateSpec returns the pod template spec for the key rotation job.
func (c *Cluster) getKeyRotationPodTemplateSpec(osdProps osdProperties, osd OSDInfo, restart v1.RestartPolicy) (*v1.PodTemplateSpec, error) {
	// create a volume on /dev so the pod can access devices on the host
	devVolume := v1.Volume{Name: "devices", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/dev"}}}
	udevVolume := v1.Volume{Name: "udev", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/run/udev"}}}
	hostPathType := v1.HostPathDirectory
	hostPath := filepath.Join(c.spec.DataDirHostPath, c.clusterInfo.Namespace, osdProps.pvc.ClaimName, fmt.Sprintf("ceph-%d", osd.ID))
	hostPathVolume := v1.Volume{
		Name: "bridge",
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{
				Path: hostPath,
				Type: &hostPathType,
			},
		},
	}
	devicesBasePath := "/var/lib/ceph/osd/"
	volumes := []v1.Volume{
		udevVolume,
		devVolume,
		hostPathVolume,
	}
	volumeMounts := []v1.VolumeMount{
		{Name: "devices", MountPath: "/dev"},
		{Name: "udev", MountPath: "/run/udev"},
		{Name: "bridge", MountPath: devicesBasePath},
	}

	devices := []string{encryptionBlockDestinationCopy(devicesBasePath, bluestoreBlockName)}
	if osdProps.metadataPVC.ClaimName != "" {
		devices = append(devices, encryptionBlockDestinationCopy(devicesBasePath, bluestoreMetadataName))
	}
	if osdProps.walPVC.ClaimName != "" {
		devices = append(devices, encryptionBlockDestinationCopy(devicesBasePath, bluestoreWalName))
	}

	if c.spec.Security.KeyManagementService.IsVaultKMS() {
		volumeTLS, volumeMountTLS := kms.VaultVolumeAndMount(c.spec.Security.KeyManagementService.ConnectionDetails, "")
		volumes = append(volumes, volumeTLS)
		volumeMounts = append(volumeMounts, volumeMountTLS)
	}

	keyRotationContainer, err := c.getKeyRotationContainer(osdProps, volumeMounts, devices)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate key rotation container")
	}

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name: keyRotationCronJobAppName,
			Labels: map[string]string{
				k8sutil.AppAttr:     keyRotationCronJobAppName,
				k8sutil.ClusterAttr: c.clusterInfo.Namespace,
			},
			Annotations: map[string]string{},
		},
		Spec: v1.PodSpec{
			ServiceAccountName: serviceAccountName,
			Containers: []v1.Container{
				keyRotationContainer,
			},
			RestartPolicy:     restart,
			Volumes:           volumes,
			HostNetwork:       c.spec.Network.IsHost(),
			PriorityClassName: cephv1.GetOSDPriorityClassName(c.spec.PriorityClassNames),
			SchedulerName:     osdProps.schedulerName,
			SecurityContext:   &v1.PodSecurityContext{},
		},
	}
	if c.spec.Network.IsHost() {
		podTemplateSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if c.spec.Network.IsMultus() {
		if err := k8sutil.ApplyMultus(c.clusterInfo.Namespace, &c.spec.Network, &podTemplateSpec.ObjectMeta); err != nil {
			return nil, err
		}
	}

	cephv1.GetKeyRotationAnnotations(c.spec.Annotations).ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)
	cephv1.GetKeyRotationLabels(c.spec.Labels).ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)

	c.applyAllPlacementIfNeeded(&podTemplateSpec.Spec)
	// apply storageClassDeviceSets.Placement
	osdProps.placement.ApplyToPodSpec(&podTemplateSpec.Spec)
	applyKeyRotationPlacement(&podTemplateSpec.Spec, c.getOSDLabels(osd, osdProps.crushHostname, osdProps.portable))

	// cryptsetup synchronizes with udev on host through semaphore
	podTemplateSpec.Spec.HostIPC = true

	k8sutil.RemoveDuplicateEnvVars(&podTemplateSpec.Spec)
	return &podTemplateSpec, nil
}

// makeKeyRotationCronJob creates a key rotation cron job for the given OSD.
func (c *Cluster) makeKeyRotationCronJob(pvcName string, osd OSDInfo, osdProps osdProperties) (*batch.CronJob, error) {
	podSpec, err := c.getKeyRotationPodTemplateSpec(osdProps, osd, v1.RestartPolicyOnFailure)
	if err != nil {
		return nil, err
	}
	c.applyResourcesToAllContainers(&podSpec.Spec, cephv1.GetOSDResources(c.spec.Resources, osd.DeviceClass))
	schedule := c.spec.Security.KeyRotation.Schedule
	if schedule == "" {
		// default to rotate keyrings weekly (default is in code since default in crds causes issues)
		schedule = "@weekly"
	}
	cronJob := &batch.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        keyRotationCronJobName(osd.ID),
			Namespace:   c.clusterInfo.Namespace,
			Labels:      podSpec.Labels,
			Annotations: podSpec.Annotations,
		},
		Spec: batch.CronJobSpec{
			ConcurrencyPolicy: batch.ForbidConcurrent,
			Schedule:          schedule,
			JobTemplate: batch.JobTemplateSpec{
				Spec: batch.JobSpec{
					Template: *podSpec,
				},
			},
		},
	}

	return cronJob, nil
}

// reconcileKeyRotationCronJob reconciles the key rotation cron jobs for the OSDs.
func (c *Cluster) reconcileKeyRotationCronJob() error {
	if !c.spec.Security.KeyRotation.Enabled {
		listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, keyRotationCronJobAppName)}
		err := c.context.Clientset.BatchV1().
			CronJobs(c.clusterInfo.Namespace).
			DeleteCollection(c.clusterInfo.Context,
				metav1.DeleteOptions{},
				listOpts)
		if err != nil {
			if client.IgnoreNotFound(err) != nil {
				return errors.Wrap(err, "failed to delete key rotation cron jobs")
			}
			return nil
		}
		logger.Debugf("successfully deleted key rotation cron jobs")

		return nil
	}

	// Get the list of OSDs backed by pvc.
	osdListOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s,%s", k8sutil.AppAttr, AppName, OSDOverPVCLabelKey)}
	deployments, err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).List(c.clusterInfo.Context, osdListOpts)
	if err != nil {
		return errors.Wrap(err, "failed to query existing OSD deployments")
	}

	logger.Debugf("found %d osd deployments", len(deployments.Items))
	for i := range deployments.Items {
		osdDep := deployments.Items[i]
		osd, err := c.getOSDInfo(&osdDep)
		if err != nil {
			return errors.Wrapf(err, "failed to get osd info for osd %q", osdDep.Name)
		}
		pvcName := osdDep.Labels[OSDOverPVCLabelKey]
		if pvcName == "" {
			return errors.Errorf("pvc name label %q for osd %q is empty",
				OSDOverPVCLabelKey, osdDep.Name)
		}
		osdProps, err := c.getOSDPropsForPVC(pvcName)
		if err != nil {
			return errors.Wrapf(err, "failed to generate config for osd %q", osdDep.Name)
		}
		if !osdProps.encrypted {
			continue
		}

		logger.Infof("starting OSD key rotation cron job for osd %q", osd.ID)
		cj, err := c.makeKeyRotationCronJob(pvcName, osd, osdProps)
		if err != nil {
			return errors.Wrap(err, "failed to make key rotation cron job")
		}

		err = ctrl.SetOwnerReference(&osdDep, cj, c.context.Client.Scheme())
		if err != nil {
			return errors.Wrapf(err, "failed to set controllerReference on cron job %q", cj.Name)
		}

		_, err = k8sutil.CreateOrUpdateCronJob(c.clusterInfo.Context, c.context.Clientset, cj)
		if err != nil {
			return errors.Wrapf(err, "failed to create or update key rotation cron job %q", cj.Name)
		}
		logger.Infof("started OSD key rotation cron job %q", cj.Name)
	}
	logger.Infof("successfully started OSD key rotation cron jobs")

	return nil
}
