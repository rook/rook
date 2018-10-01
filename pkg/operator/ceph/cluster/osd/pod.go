/*
Copyright 2016 The Rook Authors. All rights reserved.

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

// Package osd for the Ceph OSDs.
package osd

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	opmon "github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"

	batch "k8s.io/api/batch/v1"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

const (
	dataDirsEnvVarName          = "ROOK_DATA_DIRECTORIES"
	osdStoreEnvVarName          = "ROOK_OSD_STORE"
	osdDatabaseSizeEnvVarName   = "ROOK_OSD_DATABASE_SIZE"
	osdWalSizeEnvVarName        = "ROOK_OSD_WAL_SIZE"
	osdJournalSizeEnvVarName    = "ROOK_OSD_JOURNAL_SIZE"
	osdMetadataDeviceEnvVarName = "ROOK_METADATA_DEVICE"
)

func (c *Cluster) makeJob(nodeName string, devices []rookalpha.Device,
	selection rookalpha.Selection, resources v1.ResourceRequirements, storeConfig config.StoreConfig, metadataDevice, location string) (*batch.Job, error) {

	podSpec, err := c.provisionPodTemplateSpec(devices, selection, resources, storeConfig, metadataDevice, nodeName, location, v1.RestartPolicyOnFailure)
	if err != nil {
		return nil, err
	}
	podSpec.Spec.NodeSelector = map[string]string{apis.LabelHostname: nodeName}

	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sutil.TruncateNodeName(prepareAppNameFmt, nodeName),
			Namespace: c.Namespace,
			Labels: map[string]string{
				k8sutil.AppAttr:     prepareAppName,
				k8sutil.ClusterAttr: c.Namespace,
			},
		},
		Spec: batch.JobSpec{
			Template: *podSpec,
		},
	}
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &job.ObjectMeta, &c.ownerRef)
	return job, nil
}

func (c *Cluster) makeDeployment(nodeName string, devices []rookalpha.Device, selection rookalpha.Selection, resources v1.ResourceRequirements,
	storeConfig config.StoreConfig, metadataDevice, location string, osd OSDInfo) (*extensions.Deployment, error) {

	replicaCount := int32(1)
	var volumeMounts []v1.VolumeMount
	configVolumeMounts := []v1.VolumeMount{
		{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
		k8sutil.ConfigOverrideMount(),
	}
	volumes := []v1.Volume{k8sutil.ConfigOverrideVolume()}
	if c.dataDirHostPath != "" {
		// the user has specified a host path to use for the data dir, use that instead
		dataDirSource := v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: c.dataDirHostPath}}
		volumes = append(volumes, v1.Volume{Name: k8sutil.DataDirVolume, VolumeSource: dataDirSource})
	} else {
		logger.Infof("no data dir provided")
	}

	var dataDir string
	if osd.IsDirectory {
		// Mount the path to the directory-based osd
		// osd.DataPath includes the osd subdirectory, so we want to mount the parent directory
		parentDir := filepath.Dir(osd.DataPath)
		dataDir = parentDir
		// Skip the mount if this is the default directory being mounted. Inside the container, the path
		// will be mounted at "/var/lib/rook" even if the dataDirHostPath is a different path on the host.
		if parentDir != k8sutil.DataDir {
			volumeName := k8sutil.PathToVolumeName(parentDir)
			dataDirSource := v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: parentDir}}
			volumes = append(volumes, v1.Volume{Name: volumeName, VolumeSource: dataDirSource})
			configVolumeMounts = append(configVolumeMounts, v1.VolumeMount{Name: volumeName, MountPath: parentDir})
		}
		volumeMounts = configVolumeMounts
	} else {
		// Mount the required device
		dataDir = k8sutil.DataDir

		// create volume config for the data dir and /dev so the pod can access devices on the host
		devVolume := v1.Volume{Name: "devices", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/dev"}}}
		volumes = append(volumes, devVolume)
		devMount := v1.VolumeMount{Name: "devices", MountPath: "/dev"}
		volumeMounts = append(configVolumeMounts, devMount)
	}

	if len(volumes) == 0 {
		return nil, fmt.Errorf("empty volumes")
	}

	osdID := strconv.Itoa(osd.ID)
	tiniEnvVar := v1.EnvVar{Name: "TINI_SUBREAPER", Value: ""}
	envVars := []v1.EnvVar{
		nodeNameEnvVar(nodeName),
		k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
		k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
		tiniEnvVar,
	}
	configEnvVars := append(c.getConfigEnvVars(storeConfig, dataDir, nodeName, location), []v1.EnvVar{
		tiniEnvVar,
		{Name: "ROOK_OSD_ID", Value: osdID},
	}...)

	commonArgs := []string{
		"--foreground",
		"--id", osdID,
		"--conf", osd.Config,
		"--osd-data", osd.DataPath,
		"--keyring", osd.KeyringPath,
		"--cluster", osd.Cluster,
		"--osd-uuid", osd.UUID,
	}
	if osd.IsFileStore {
		commonArgs = append(commonArgs, fmt.Sprintf("--osd-journal=%s", osd.Journal))
	}

	var command []string
	var args []string
	if !osd.IsDirectory && osd.IsFileStore {
		// filestore on a device requires indirection through the rook entrypoint so we can mount the image
		sourcePath := path.Join("/dev/disk/by-partuuid", osd.DevicePartUUID)
		args = append([]string{
			"ceph", "osd", "filestore-device",
			"--source-path", sourcePath,
			"--mount-path", osd.DataPath,
			"--",
		}, commonArgs...)
	} else {
		// other osds can launch the osd daemon directly
		command = append([]string{"/tini", "--", "ceph-osd",
			fmt.Sprintf("--public-addr=$(%s)", k8sutil.PublicIPEnvVar),
			fmt.Sprintf("--cluster-addr=$(%s)", k8sutil.PrivateIPEnvVar),
		}, commonArgs...)
	}

	privileged := true
	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		Privileged:             &privileged,
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
	}

	DNSPolicy := v1.DNSClusterFirst
	if c.HostNetwork {
		DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	deployment := &extensions.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(osdAppNameFmt, osd.ID),
			Namespace: c.Namespace,
			Labels: map[string]string{
				k8sutil.AppAttr:     appName,
				k8sutil.ClusterAttr: c.Namespace,
				osdLabelKey:         fmt.Sprintf("%d", osd.ID),
			},
		},
		Spec: extensions.DeploymentSpec{
			Strategy: extensions.DeploymentStrategy{
				Type: extensions.RecreateDeploymentStrategyType,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: appName,
					Labels: map[string]string{
						k8sutil.AppAttr:     appName,
						k8sutil.ClusterAttr: c.Namespace,
						osdLabelKey:         fmt.Sprintf("%d", osd.ID),
					},
					Annotations: map[string]string{},
				},
				Spec: v1.PodSpec{
					NodeSelector:       map[string]string{apis.LabelHostname: nodeName},
					RestartPolicy:      v1.RestartPolicyAlways,
					ServiceAccountName: c.serviceAccount,
					HostNetwork:        c.HostNetwork,
					HostPID:            true,
					DNSPolicy:          DNSPolicy,
					InitContainers: []v1.Container{
						{
							Args:            []string{"ceph", "osd", "config"},
							Name:            "osd-init-config",
							Image:           k8sutil.MakeRookImage(c.Version),
							VolumeMounts:    configVolumeMounts,
							Env:             configEnvVars,
							SecurityContext: securityContext,
						},
					},
					Containers: []v1.Container{
						{
							Command:         command,
							Args:            args,
							Name:            appName,
							Image:           k8sutil.MakeRookImage(c.Version),
							VolumeMounts:    volumeMounts,
							Env:             envVars,
							Resources:       resources,
							SecurityContext: securityContext,
						},
					},
					Volumes: volumes,
				},
			},
			Replicas: &replicaCount,
		},
	}
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &deployment.ObjectMeta, &c.ownerRef)
	c.placement.ApplyToPodSpec(&deployment.Spec.Template.Spec)
	return deployment, nil
}

func (c *Cluster) provisionPodTemplateSpec(devices []rookalpha.Device, selection rookalpha.Selection, resources v1.ResourceRequirements,
	storeConfig config.StoreConfig, metadataDevice, nodeName, location string, restart v1.RestartPolicy) (*v1.PodTemplateSpec, error) {
	volumes := []v1.Volume{k8sutil.ConfigOverrideVolume()}

	if c.dataDirHostPath != "" {
		// the user has specified a host path to use for the data dir, use that instead
		dataDirSource := v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: c.dataDirHostPath}}
		volumes = append(volumes, v1.Volume{Name: k8sutil.DataDirVolume, VolumeSource: dataDirSource})
	}

	// by default, don't define any volume config unless it is required
	if len(devices) > 0 || selection.DeviceFilter != "" || selection.GetUseAllDevices() || metadataDevice != "" {
		// create volume config for the data dir and /dev so the pod can access devices on the host
		devVolume := v1.Volume{Name: "devices", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/dev"}}}
		volumes = append(volumes, devVolume)
		udevVolume := v1.Volume{Name: "udev", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/run/udev"}}}
		volumes = append(volumes, udevVolume)
	}

	// add each OSD directory as another host path volume source
	for _, d := range selection.Directories {
		dirVolume := v1.Volume{
			Name:         k8sutil.PathToVolumeName(d.Path),
			VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: d.Path}},
		}
		volumes = append(volumes, dirVolume)
	}

	if len(volumes) == 0 {
		return nil, fmt.Errorf("empty volumes")
	}

	podSpec := v1.PodSpec{
		ServiceAccountName: c.serviceAccount,
		Containers:         []v1.Container{c.provisionOSDContainer(devices, selection, resources, storeConfig, metadataDevice, nodeName, location)},
		RestartPolicy:      restart,
		Volumes:            volumes,
		HostNetwork:        c.HostNetwork,
	}
	if c.HostNetwork {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	c.placement.ApplyToPodSpec(&podSpec)

	return &v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name: appName,
			Labels: map[string]string{
				k8sutil.AppAttr:     prepareAppName,
				k8sutil.ClusterAttr: c.Namespace,
			},
			Annotations: map[string]string{},
		},
		Spec: podSpec,
	}, nil
}

func (c *Cluster) getConfigEnvVars(storeConfig config.StoreConfig, dataDir, nodeName, location string) []v1.EnvVar {
	envVars := []v1.EnvVar{
		nodeNameEnvVar(nodeName),
		{Name: "ROOK_CLUSTER_ID", Value: string(c.ownerRef.UID)},
		k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
		k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
		opmon.ClusterNameEnvVar(c.Namespace),
		opmon.EndpointEnvVar(),
		opmon.SecretEnvVar(),
		opmon.AdminSecretEnvVar(),
		k8sutil.ConfigDirEnvVar(dataDir),
		k8sutil.ConfigOverrideEnvVar(),
	}

	if storeConfig.StoreType != "" {
		envVars = append(envVars, osdStoreEnvVar(storeConfig.StoreType))
	}

	if storeConfig.DatabaseSizeMB != 0 {
		envVars = append(envVars, osdDatabaseSizeEnvVar(storeConfig.DatabaseSizeMB))
	}

	if storeConfig.WalSizeMB != 0 {
		envVars = append(envVars, osdWalSizeEnvVar(storeConfig.WalSizeMB))
	}

	if storeConfig.JournalSizeMB != 0 {
		envVars = append(envVars, osdJournalSizeEnvVar(storeConfig.JournalSizeMB))
	}

	if location != "" {
		envVars = append(envVars, rookalpha.LocationEnvVar(location))
	}

	return envVars
}

func (c *Cluster) provisionOSDContainer(devices []rookalpha.Device, selection rookalpha.Selection, resources v1.ResourceRequirements,
	storeConfig config.StoreConfig, metadataDevice, nodeName, location string) v1.Container {

	envVars := c.getConfigEnvVars(storeConfig, k8sutil.DataDir, nodeName, location)
	devMountNeeded := false
	privileged := false

	// only 1 of device list, device filter and use all devices can be specified.  We prioritize in that order.
	if len(devices) > 0 {
		deviceNames := make([]string, len(devices))
		for i := range devices {
			deviceNames[i] = devices[i].Name
		}
		envVars = append(envVars, dataDevicesEnvVar(strings.Join(deviceNames, ",")))
		devMountNeeded = true
	} else if selection.DeviceFilter != "" {
		envVars = append(envVars, deviceFilterEnvVar(selection.DeviceFilter))
		devMountNeeded = true
	} else if selection.GetUseAllDevices() {
		envVars = append(envVars, deviceFilterEnvVar("all"))
		devMountNeeded = true
	}

	if metadataDevice != "" {
		envVars = append(envVars, metadataDeviceEnvVar(metadataDevice))
		devMountNeeded = true
	}

	volumeMounts := []v1.VolumeMount{
		{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
		k8sutil.ConfigOverrideMount(),
	}
	if devMountNeeded {
		devMount := v1.VolumeMount{Name: "devices", MountPath: "/dev"}
		volumeMounts = append(volumeMounts, devMount)
		udevMount := v1.VolumeMount{Name: "udev", MountPath: "/run/udev"}
		volumeMounts = append(volumeMounts, udevMount)
	}

	if len(selection.Directories) > 0 {
		// for each directory the user has specified, create a volume mount and pass it to the pod via cmd line arg
		dirPaths := make([]string, len(selection.Directories))
		for i := range selection.Directories {
			dpath := selection.Directories[i].Path
			dirPaths[i] = dpath
			volumeMounts = append(volumeMounts, v1.VolumeMount{Name: k8sutil.PathToVolumeName(dpath), MountPath: dpath})
		}

		if !IsRemovingNode(selection.DeviceFilter) {
			envVars = append(envVars, dataDirectoriesEnvVar(strings.Join(dirPaths, ",")))
		}
	}

	// elevate to be privileged if it is going to mount devices or if running in a restricted environment such as openshift
	if devMountNeeded || os.Getenv("ROOK_HOSTPATH_REQUIRES_PRIVILEGED") == "true" {
		privileged = true
	}
	runAsUser := int64(0)
	// don't set runAsNonRoot explicitly when it is false, Kubernetes version < 1.6.4 has
	// an issue with this fixed in https://github.com/kubernetes/kubernetes/pull/47009
	// runAsNonRoot := false
	readOnlyRootFilesystem := false
	return v1.Container{
		// Set the hostname so we have the pod's host in the crush map rather than the pod container name
		Args:         []string{"ceph", "osd", "provision"},
		Name:         appName,
		Image:        k8sutil.MakeRookImage(c.Version),
		VolumeMounts: volumeMounts,
		Env:          envVars,
		SecurityContext: &v1.SecurityContext{
			Privileged: &privileged,
			RunAsUser:  &runAsUser,
			// don't set runAsNonRoot explicitly when it is false, Kubernetes version < 1.6.4 has
			// an issue with this fixed in https://github.com/kubernetes/kubernetes/pull/47009
			// RunAsNonRoot:           &runAsNonRoot,
			ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
		},
		Resources: resources,
	}
}

func nodeNameEnvVar(name string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_NODE_NAME", Value: name}
}

func dataDevicesEnvVar(dataDevices string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_DATA_DEVICES", Value: dataDevices}
}

func deviceFilterEnvVar(filter string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_DATA_DEVICE_FILTER", Value: filter}
}

func metadataDeviceEnvVar(metadataDevice string) v1.EnvVar {
	return v1.EnvVar{Name: osdMetadataDeviceEnvVarName, Value: metadataDevice}
}

func dataDirectoriesEnvVar(dataDirectories string) v1.EnvVar {
	return v1.EnvVar{Name: dataDirsEnvVarName, Value: dataDirectories}
}

func osdStoreEnvVar(osdStore string) v1.EnvVar {
	return v1.EnvVar{Name: osdStoreEnvVarName, Value: osdStore}
}

func osdDatabaseSizeEnvVar(databaseSize int) v1.EnvVar {
	return v1.EnvVar{Name: osdDatabaseSizeEnvVarName, Value: strconv.Itoa(databaseSize)}
}

func osdWalSizeEnvVar(walSize int) v1.EnvVar {
	return v1.EnvVar{Name: osdWalSizeEnvVarName, Value: strconv.Itoa(walSize)}
}

func osdJournalSizeEnvVar(journalSize int) v1.EnvVar {
	return v1.EnvVar{Name: osdJournalSizeEnvVarName, Value: strconv.Itoa(journalSize)}
}

func getDirectoriesFromContainer(osdContainer v1.Container) []rookalpha.Directory {
	var dirsArg string
	for _, envVar := range osdContainer.Env {
		if envVar.Name == dataDirsEnvVarName {
			dirsArg = envVar.Value
		}
	}

	var dirsList []string
	if dirsArg != "" {
		dirsList = strings.Split(dirsArg, ",")
	}

	dirs := make([]rookalpha.Directory, len(dirsList))
	for dirNum, dir := range dirsList {
		dirs[dirNum] = rookalpha.Directory{Path: dir}
	}

	return dirs
}

func getConfigFromContainer(osdContainer v1.Container) map[string]string {
	cfg := map[string]string{}

	for _, envVar := range osdContainer.Env {
		switch envVar.Name {
		case osdStoreEnvVarName:
			cfg[config.StoreTypeKey] = envVar.Value
		case osdDatabaseSizeEnvVarName:
			cfg[config.DatabaseSizeMBKey] = envVar.Value
		case osdWalSizeEnvVarName:
			cfg[config.WalSizeMBKey] = envVar.Value
		case osdJournalSizeEnvVarName:
			cfg[config.JournalSizeMBKey] = envVar.Value
		case osdMetadataDeviceEnvVarName:
			cfg[config.MetadataDeviceKey] = envVar.Value
		}
	}

	return cfg
}
