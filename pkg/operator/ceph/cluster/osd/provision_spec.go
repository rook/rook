/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	"encoding/json"
	"fmt"
	"path"

	"github.com/libopenstorage/secrets"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	kms "github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Cluster) makeJob(osdProps osdProperties, provisionConfig *provisionConfig) (*batch.Job, error) {
	podSpec, err := c.provisionPodTemplateSpec(osdProps, v1.RestartPolicyOnFailure, provisionConfig)
	if err != nil {
		return nil, err
	}

	if !osdProps.onPVC() {
		podSpec.Spec.NodeSelector = map[string]string{v1.LabelHostname: osdProps.crushHostname}
	} else {
		// This is not needed in raw mode and 14.2.8 brings it
		// but we still want to do this not to lose backward compatibility with lvm based OSDs...
		podSpec.Spec.InitContainers = append(podSpec.Spec.InitContainers, c.getPVCInitContainer(osdProps))
		if osdProps.onPVCWithMetadata() {
			podSpec.Spec.InitContainers = append(podSpec.Spec.InitContainers, c.getPVCMetadataInitContainer("/srv", osdProps))
		}
		if osdProps.onPVCWithWal() {
			podSpec.Spec.InitContainers = append(podSpec.Spec.InitContainers, c.getPVCWalInitContainer("/wal", osdProps))
		}
	}

	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sutil.TruncateNodeName(prepareAppNameFmt, osdProps.crushHostname),
			Namespace: c.clusterInfo.Namespace,
			Labels: map[string]string{
				k8sutil.AppAttr:     prepareAppName,
				k8sutil.ClusterAttr: c.clusterInfo.Namespace,
			},
		},
		Spec: batch.JobSpec{
			Template: *podSpec,
		},
	}

	if osdProps.onPVC() {
		k8sutil.AddLabelToJob(OSDOverPVCLabelKey, osdProps.pvc.ClaimName, job)
		k8sutil.AddLabelToJob(CephDeviceSetLabelKey, osdProps.deviceSetName, job)
		k8sutil.AddLabelToPod(OSDOverPVCLabelKey, osdProps.pvc.ClaimName, &job.Spec.Template)
		k8sutil.AddLabelToPod(CephDeviceSetLabelKey, osdProps.deviceSetName, &job.Spec.Template)
	}

	k8sutil.AddRookVersionLabelToJob(job)
	controller.AddCephVersionLabelToJob(c.clusterInfo.CephVersion, job)
	err = c.clusterInfo.OwnerInfo.SetControllerReference(job)
	if err != nil {
		return nil, err
	}

	// override the resources of all the init containers and main container with the expected osd prepare resources
	c.applyResourcesToAllContainers(&podSpec.Spec, cephv1.GetPrepareOSDResources(c.spec.Resources))
	return job, nil
}

// applyResourcesToAllContainers applies consistent resource requests for all containers and all init containers in the pod
func (c *Cluster) applyResourcesToAllContainers(spec *v1.PodSpec, resources v1.ResourceRequirements) {
	for i := range spec.InitContainers {
		spec.InitContainers[i].Resources = resources
	}
	for i := range spec.Containers {
		spec.Containers[i].Resources = resources
	}
}

func (c *Cluster) provisionPodTemplateSpec(osdProps osdProperties, restart v1.RestartPolicy, provisionConfig *provisionConfig) (*v1.PodTemplateSpec, error) {
	copyBinariesVolume, copyBinariesContainer := c.getCopyBinariesContainer()

	// ceph-volume is currently set up to use /etc/ceph/ceph.conf; this means no user config
	// overrides will apply to ceph-volume, but this is unnecessary anyway
	volumes := append(controller.PodVolumes(provisionConfig.DataPathMap, c.spec.DataDirHostPath, true), copyBinariesVolume)

	// create a volume on /dev so the pod can access devices on the host
	devVolume := v1.Volume{Name: "devices", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/dev"}}}
	volumes = append(volumes, devVolume)
	udevVolume := v1.Volume{Name: "udev", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/run/udev"}}}
	volumes = append(volumes, udevVolume)

	// If not running on PVC we mount the rootfs of the host to validate the presence of the LVM package
	if !osdProps.onPVC() {
		rootFSVolume := v1.Volume{Name: "rootfs", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/"}}}
		volumes = append(volumes, rootFSVolume)
	}

	if osdProps.onPVC() {
		// Create volume config for PVCs
		volumes = append(volumes, getPVCOSDVolumes(&osdProps, c.spec.DataDirHostPath, c.clusterInfo.Namespace, true)...)
		if osdProps.encrypted {
			// If a KMS is configured we populate
			if c.spec.Security.KeyManagementService.IsEnabled() {
				kmsProvider := kms.GetParam(c.spec.Security.KeyManagementService.ConnectionDetails, kms.Provider)
				if kmsProvider == secrets.TypeVault {
					volumeTLS, _ := kms.VaultVolumeAndMount(c.spec.Security.KeyManagementService.ConnectionDetails)
					volumes = append(volumes, volumeTLS)
				}
			}
		}
	}

	if len(volumes) == 0 {
		return nil, errors.New("empty volumes")
	}

	provisionContainer, err := c.provisionOSDContainer(osdProps, copyBinariesContainer.VolumeMounts[0], provisionConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate OSD provisioning container")
	}

	podSpec := v1.PodSpec{
		ServiceAccountName: serviceAccountName,
		InitContainers: []v1.Container{
			*copyBinariesContainer,
		},
		Containers: []v1.Container{
			provisionContainer,
		},
		RestartPolicy:     restart,
		Volumes:           volumes,
		HostNetwork:       c.spec.Network.IsHost(),
		PriorityClassName: cephv1.GetOSDPriorityClassName(c.spec.PriorityClassNames),
		SchedulerName:     osdProps.schedulerName,
	}
	if c.spec.Network.IsHost() {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	if osdProps.onPVC() {
		c.applyAllPlacementIfNeeded(&podSpec)
		// apply storageClassDeviceSets.preparePlacement
		osdProps.getPreparePlacement().ApplyToPodSpec(&podSpec)
	} else {
		c.applyAllPlacementIfNeeded(&podSpec)
		// apply spec.placement.prepareosd
		c.spec.Placement[cephv1.KeyOSDPrepare].ApplyToPodSpec(&podSpec)
	}

	k8sutil.RemoveDuplicateEnvVars(&podSpec)

	podMeta := metav1.ObjectMeta{
		Name: AppName,
		Labels: map[string]string{
			k8sutil.AppAttr:     prepareAppName,
			k8sutil.ClusterAttr: c.clusterInfo.Namespace,
			OSDOverPVCLabelKey:  osdProps.pvc.ClaimName,
		},
		Annotations: map[string]string{},
	}

	cephv1.GetOSDPrepareAnnotations(c.spec.Annotations).ApplyToObjectMeta(&podMeta)
	cephv1.GetOSDPrepareLabels(c.spec.Labels).ApplyToObjectMeta(&podMeta)

	// ceph-volume --dmcrypt uses cryptsetup that synchronizes with udev on
	// host through semaphore
	podSpec.HostIPC = osdProps.storeConfig.EncryptedDevice || osdProps.encrypted

	return &v1.PodTemplateSpec{
		ObjectMeta: podMeta,
		Spec:       podSpec,
	}, nil
}

func (c *Cluster) provisionOSDContainer(osdProps osdProperties, copyBinariesMount v1.VolumeMount, provisionConfig *provisionConfig) (v1.Container, error) {
	envVars := c.getConfigEnvVars(osdProps, k8sutil.DataDir)

	// enable debug logging in the prepare job
	envVars = append(envVars, setDebugLogLevelEnvVar(true))

	// only 1 of device list, device filter, device path filter and use all devices can be specified.  We prioritize in that order.
	if len(osdProps.devices) > 0 {
		configuredDevices := []config.ConfiguredDevice{}
		for _, device := range osdProps.devices {
			id := device.Name
			if device.FullPath != "" {
				id = device.FullPath
			}
			cd := config.ConfiguredDevice{
				ID:          id,
				StoreConfig: config.ToStoreConfig(device.Config),
			}
			configuredDevices = append(configuredDevices, cd)
		}
		marshalledDevices, err := json.Marshal(configuredDevices)
		if err != nil {
			return v1.Container{}, errors.Wrapf(err, "failed to JSON marshal configured devices for node %q", osdProps.crushHostname)
		}
		envVars = append(envVars, dataDevicesEnvVar(string(marshalledDevices)))
	} else if osdProps.selection.DeviceFilter != "" {
		envVars = append(envVars, deviceFilterEnvVar(osdProps.selection.DeviceFilter))
	} else if osdProps.selection.DevicePathFilter != "" {
		envVars = append(envVars, devicePathFilterEnvVar(osdProps.selection.DevicePathFilter))
	} else if osdProps.selection.GetUseAllDevices() {
		envVars = append(envVars, deviceFilterEnvVar("all"))
	}
	envVars = append(envVars, v1.EnvVar{Name: "ROOK_CEPH_VERSION", Value: c.clusterInfo.CephVersion.CephVersionFormatted()})
	envVars = append(envVars, crushDeviceClassEnvVar(osdProps.storeConfig.DeviceClass))
	envVars = append(envVars, crushInitialWeightEnvVar(osdProps.storeConfig.InitialWeight))

	if osdProps.metadataDevice != "" {
		envVars = append(envVars, metadataDeviceEnvVar(osdProps.metadataDevice))
	}

	volumeMounts := append(controller.CephVolumeMounts(provisionConfig.DataPathMap, true), []v1.VolumeMount{
		{Name: "devices", MountPath: "/dev"},
		{Name: "udev", MountPath: "/run/udev"},
		copyBinariesMount,
	}...)

	// If not running on PVC we mount the rootfs of the host to validate the presence of the LVM package
	if !osdProps.onPVC() {
		volumeMounts = append(volumeMounts, v1.VolumeMount{Name: "rootfs", MountPath: "/rootfs", ReadOnly: true})
	}

	// If the OSD runs on PVC
	if osdProps.onPVC() {
		volumeMounts = append(volumeMounts, getPvcOSDBridgeMount(osdProps.pvc.ClaimName))
		// The device list is read by the Rook CLI via environment variables so let's add them
		configuredDevices := []config.ConfiguredDevice{
			{
				ID:          fmt.Sprintf("/mnt/%s", osdProps.pvc.ClaimName),
				StoreConfig: config.NewStoreConfig(),
			},
		}
		if osdProps.onPVCWithMetadata() {
			volumeMounts = append(volumeMounts, getPvcMetadataOSDBridgeMount(osdProps.metadataPVC.ClaimName))
			configuredDevices = append(configuredDevices,
				config.ConfiguredDevice{
					ID:          fmt.Sprintf("/srv/%s", osdProps.metadataPVC.ClaimName),
					StoreConfig: config.NewStoreConfig(),
				})
		}
		if osdProps.onPVCWithWal() {
			volumeMounts = append(volumeMounts, getPvcWalOSDBridgeMount(osdProps.walPVC.ClaimName))
			configuredDevices = append(configuredDevices,
				config.ConfiguredDevice{
					ID:          fmt.Sprintf("/wal/%s", osdProps.walPVC.ClaimName),
					StoreConfig: config.NewStoreConfig(),
				})
		}
		marshalledDevices, err := json.Marshal(configuredDevices)
		if err != nil {
			return v1.Container{}, errors.Wrapf(err, "failed to JSON marshal configured devices for PVC %q", osdProps.crushHostname)
		}
		envVars = append(envVars, dataDevicesEnvVar(string(marshalledDevices)))
		envVars = append(envVars, pvcBackedOSDEnvVar("true"))
		envVars = append(envVars, encryptedDeviceEnvVar(osdProps.encrypted))
		envVars = append(envVars, pvcNameEnvVar(osdProps.pvc.ClaimName))

		if osdProps.encrypted {
			// If a KMS is configured we populate
			if c.spec.Security.KeyManagementService.IsEnabled() {
				kmsProvider := kms.GetParam(c.spec.Security.KeyManagementService.ConnectionDetails, kms.Provider)
				if kmsProvider == secrets.TypeVault {
					_, volumeMountsTLS := kms.VaultVolumeAndMount(c.spec.Security.KeyManagementService.ConnectionDetails)
					volumeMounts = append(volumeMounts, volumeMountsTLS)
					envVars = append(envVars, kms.VaultConfigToEnvVar(c.spec)...)
				}
			} else {
				envVars = append(envVars, cephVolumeRawEncryptedEnvVarFromSecret(osdProps))
			}
		}
	}

	// run privileged always since we always mount /dev
	privileged := true
	runAsUser := int64(0)
	runAsNonRoot := false
	readOnlyRootFilesystem := false

	osdProvisionContainer := v1.Container{
		Command:      []string{path.Join(rookBinariesMountPath, "tini")},
		Args:         []string{"--", path.Join(rookBinariesMountPath, "rook"), "ceph", "osd", "provision"},
		Name:         "provision",
		Image:        c.spec.CephVersion.Image,
		VolumeMounts: volumeMounts,
		Env:          envVars,
		SecurityContext: &v1.SecurityContext{
			Privileged:             &privileged,
			RunAsUser:              &runAsUser,
			RunAsNonRoot:           &runAsNonRoot,
			ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
		},
		Resources: cephv1.GetPrepareOSDResources(c.spec.Resources),
	}

	return osdProvisionContainer, nil
}
