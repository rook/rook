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
	"strconv"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	opmon "github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/ceph/version"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	dataDirsEnvVarName                  = "ROOK_DATA_DIRECTORIES"
	osdStoreEnvVarName                  = "ROOK_OSD_STORE"
	osdDatabaseSizeEnvVarName           = "ROOK_OSD_DATABASE_SIZE"
	osdWalSizeEnvVarName                = "ROOK_OSD_WAL_SIZE"
	osdJournalSizeEnvVarName            = "ROOK_OSD_JOURNAL_SIZE"
	osdsPerDeviceEnvVarName             = "ROOK_OSDS_PER_DEVICE"
	encryptedDeviceEnvVarName           = "ROOK_ENCRYPTED_DEVICE"
	osdMetadataDeviceEnvVarName         = "ROOK_METADATA_DEVICE"
	pvcBackedOSDVarName                 = "ROOK_PVC_BACKED_OSD"
	lvPathVarName                       = "ROOK_LV_PATH"
	lvBackedPVVarName                   = "ROOK_LV_BACKED_PV"
	rookBinariesMountPath               = "/rook"
	rookBinariesVolumeName              = "rook-binaries"
	activateOSDVolumeName               = "activate-osd"
	activateOSDMountPath                = "/var/lib/ceph/osd/ceph-"
	blockPVCMapperInitContainer         = "blkdevmapper"
	osdMemoryTargetSafetyFactor float32 = 0.8
	// CephDeviceSetLabelKey is the Rook device set label key
	CephDeviceSetLabelKey = "ceph.rook.io/DeviceSet"
	// CephSetIndexLabelKey is the Rook label key index
	CephSetIndexLabelKey = "ceph.rook.io/setIndex"
	// CephDeviceSetPVCIDLabelKey is the Rook PVC ID label key
	CephDeviceSetPVCIDLabelKey = "ceph.rook.io/DeviceSetPVCId"
	// OSDOverPVCLabelKey is the Rook PVC label key
	OSDOverPVCLabelKey = "ceph.rook.io/pvc"
)

const (
	activateOSDCode = `
set -ex

OSD_ID=%s
OSD_UUID=%s
OSD_STORE_FLAG="%s"
TMP_DIR=$(mktemp -d)
OSD_DATA_DIR=/var/lib/ceph/osd/ceph-"$OSD_ID"

# active the osd with ceph-volume
ceph-volume lvm activate --no-systemd "$OSD_STORE_FLAG" "$OSD_ID" "$OSD_UUID"

# copy the tmpfs directory to a temporary directory
# this is needed because when the init container exits, the tmpfs goes away and its content with it
# this will result in the emptydir to be empty when accessed by the main osd container
cp --verbose --no-dereference "$OSD_DATA_DIR"/* "$TMP_DIR"/

# unmount the tmpfs since we don't need it anymore
umount "$OSD_DATA_DIR"

# copy back the content of the tmpfs into the original osd directory
cp --verbose --no-dereference "$TMP_DIR"/* "$OSD_DATA_DIR"

# retain ownership of files to the ceph user/group
chown --verbose --recursive ceph:ceph "$OSD_DATA_DIR"

# remove the temporary directory
rm --recursive --force "$TMP_DIR"
`
)

// OSDs on PVC using a certain storage class need to do some tuning
const (
	osdRecoverySleep = "0.1"
	osdSnapTrimSleep = "2"
	osdDeleteSleep   = "2"
)

func (c *Cluster) makeJob(osdProps osdProperties, provisionConfig *provisionConfig) (*batch.Job, error) {

	podSpec, err := c.provisionPodTemplateSpec(osdProps, v1.RestartPolicyOnFailure, provisionConfig)
	if err != nil {
		return nil, err
	}

	if osdProps.pvc.ClaimName == "" {
		podSpec.Spec.NodeSelector = map[string]string{v1.LabelHostname: osdProps.crushHostname}
	} else {
		podSpec.Spec.InitContainers = append(podSpec.Spec.InitContainers, c.getPVCInitContainer(osdProps.pvc))
	}

	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sutil.TruncateNodeName(prepareAppNameFmt, osdProps.crushHostname),
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

	if len(osdProps.pvc.ClaimName) > 0 {
		k8sutil.AddLabelToJob(OSDOverPVCLabelKey, osdProps.pvc.ClaimName, job)
	}

	k8sutil.AddRookVersionLabelToJob(job)
	opspec.AddCephVersionLabelToJob(c.clusterInfo.CephVersion, job)
	k8sutil.SetOwnerRef(&job.ObjectMeta, &c.ownerRef)
	return job, nil
}

func (c *Cluster) makeDeployment(osdProps osdProperties, osd OSDInfo, provisionConfig *provisionConfig) (*apps.Deployment, error) {
	deploymentName := fmt.Sprintf(osdAppNameFmt, osd.ID)
	replicaCount := int32(1)
	volumeMounts := opspec.CephVolumeMounts(provisionConfig.DataPathMap, false)
	configVolumeMounts := opspec.RookVolumeMounts(provisionConfig.DataPathMap, false)
	volumes := opspec.PodVolumes(provisionConfig.DataPathMap, c.dataDirHostPath, false)
	failureDomainValue := osdProps.crushHostname
	doConfigInit := true       // initialize ceph.conf in init container?
	doBinaryCopyInit := true   // copy tini and rook binaries in an init container?
	doActivateOSDInit := false // run an init container to activate the osd?
	doChownDataPath := true    // chown the data path in an init container?

	var dataDir string
	var sourcePath string
	if osd.IsDirectory {
		// Mount the path to the directory-based osd
		// osd.DataPath includes the osd subdirectory, so we want to mount the parent directory
		//parentDir := filepath.Dir(osd.DataPath)
		//dataDir = parentDir
		dataDir = osd.DataPath
		// for directory osds, we completely overwrite the starting point from above.
		provisionConfig.DataPathMap.HostDataDir = dataDir
		provisionConfig.DataPathMap.ContainerDataDir = dataDir

		volumes = opspec.DaemonVolumes(provisionConfig.DataPathMap, deploymentName)
		volumeMounts = opspec.DaemonVolumeMounts(provisionConfig.DataPathMap, deploymentName)
		configVolumeMounts = opspec.DaemonVolumeMounts(provisionConfig.DataPathMap, deploymentName)
	} else if !osd.IsDirectory && osd.IsFileStore && !osd.CephVolumeInitiated {
		// for legacy filestore osds, sourcePath is mounted to the dataDir in the container.
		dataDir = osd.DataPath
		sourcePath = path.Join("/dev/disk/by-partuuid", osd.DevicePartUUID)
		provisionConfig.DataPathMap.HostDataDir = sourcePath
		provisionConfig.DataPathMap.ContainerDataDir = sourcePath

		volumes = opspec.DaemonVolumes(provisionConfig.DataPathMap, deploymentName)
		volumeMounts = opspec.DaemonVolumeMounts(provisionConfig.DataPathMap, deploymentName)
		configVolumeMounts = opspec.DaemonVolumeMounts(provisionConfig.DataPathMap, deploymentName)
	} else {
		dataDir = k8sutil.DataDir
		// Create volume config for /dev so the pod can access devices on the host
		devVolume := v1.Volume{Name: "devices", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/dev"}}}
		volumes = append(volumes, devVolume)
		devMount := v1.VolumeMount{Name: "devices", MountPath: "/dev"}
		volumeMounts = append(volumeMounts, devMount)
	}

	// If the OSD runs on PVC
	if osdProps.pvc.ClaimName != "" {
		// Create volume config for PVCs
		volumes = append(volumes, getPVCOSDVolumes(&osdProps)...)
	}

	if len(volumes) == 0 {
		return nil, errors.New("empty volumes")
	}

	storeType := config.Bluestore
	if osd.IsFileStore {
		storeType = config.Filestore
	}

	osdID := strconv.Itoa(osd.ID)
	tiniEnvVar := v1.EnvVar{Name: "TINI_SUBREAPER", Value: ""}
	envVars := append(c.getConfigEnvVars(osdProps, dataDir), []v1.EnvVar{
		tiniEnvVar,
	}...)
	envVars = append(envVars, k8sutil.ClusterDaemonEnvVars(c.cephVersion.Image)...)
	envVars = append(envVars, []v1.EnvVar{
		{Name: "ROOK_OSD_UUID", Value: osd.UUID},
		{Name: "ROOK_OSD_ID", Value: osdID},
		{Name: "ROOK_OSD_STORE_TYPE", Value: storeType},
		{Name: "ROOK_CEPH_MON_HOST",
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{
					Name: "rook-ceph-config"},
					Key: "mon_host"}}},
		{Name: "CEPH_ARGS", Value: "-m $(ROOK_CEPH_MON_HOST)"},
	}...)
	configEnvVars := append(c.getConfigEnvVars(osdProps, dataDir), []v1.EnvVar{
		tiniEnvVar,
		{Name: "ROOK_OSD_ID", Value: osdID},
		{Name: "ROOK_CEPH_VERSION", Value: c.clusterInfo.CephVersion.CephVersionFormatted()},
	}...)

	if !osd.IsDirectory {
		configEnvVars = append(configEnvVars, v1.EnvVar{Name: "ROOK_IS_DEVICE", Value: "true"})
	}

	// default args when the ceph cluster isn't initialized
	defaultArgs := []string{
		"--foreground",
		"--id", osdID,
		"--osd-data", osd.DataPath,
		"--keyring", osd.KeyringPath,
		"--cluster", osd.Cluster,
		"--osd-uuid", osd.UUID,
	}

	var commonArgs []string

	// If the OSD runs on PVC
	if osdProps.pvc.ClaimName != "" && osdProps.tuneSlowDeviceClass {
		// Append tuning flag if necessary
		err := c.osdRunFlagTuningOnPVC(osd.ID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to apply tuning on osd %q", strconv.Itoa(osd.ID))
		}
	}

	// Set osd memory target to the best appropriate value
	if !osd.IsFileStore {
		// As of Nautilus Ceph auto-tunes its osd_memory_target on the fly so we don't need to force it
		if !c.clusterInfo.CephVersion.IsAtLeastNautilus() && !c.resources.Limits.Memory().IsZero() {
			osdMemoryTargetValue := float32(c.resources.Limits.Memory().Value()) * osdMemoryTargetSafetyFactor
			commonArgs = append(commonArgs, fmt.Sprintf("--osd-memory-target=%d", int(osdMemoryTargetValue)))
		}
	}

	if c.clusterInfo.CephVersion.IsAtLeast(version.CephVersion{Major: 14, Minor: 2, Extra: 1}) {
		commonArgs = append(commonArgs, "--default-log-to-file", "false")
	}

	commonArgs = append(commonArgs, osdOnSDNFlag(c.Network, c.clusterInfo.CephVersion)...)

	var command []string
	var args []string
	if !osd.IsDirectory && osd.IsFileStore && !osd.CephVolumeInitiated {
		doConfigInit = false
		// All scenarios except one can call the ceph-osd daemon directly. The one different scenario is when
		// filestore is running on a device. Rook needs to mount the device, run the ceph-osd daemon, and then
		// when the daemon exits, rook needs to unmount the device. Since rook needs to be in the container
		// for this scenario, we will copy the binaries necessary to a mount, which will then be mounted
		// to the daemon container.
		doBinaryCopyInit = true
		// Since the data path is mounted from a different source for filestore disks, don't chown it
		doChownDataPath = false

		command = []string{path.Join(k8sutil.BinariesMountPath, "tini")}
		args = []string{
			"--", path.Join(k8sutil.BinariesMountPath, "rook"),
			"ceph", "osd", "filestore-device",
			"--source-path", sourcePath,
			"--mount-path", osd.DataPath,
			"--",
		}
		args = append(args,
			opspec.DaemonFlags(c.clusterInfo, osdID)...,
		)
		args = append(args,
			"--foreground",
			"--osd-data", osd.DataPath,
			"--osd-uuid", osd.UUID,
			"--osd-objectstore", storeType,
			"--osd-max-object-name-len", "256",
			"--osd-max-object-namespace-len", "64",
			"--crush-location", fmt.Sprintf("root=default host=%s", osdProps.crushHostname),
			// Set '--setuser-match-path' so that existing directory owned by root won't affect the daemon startup.
			// For existing data store owned by root, the daemon will continue to run as root
			"--setuser-match-path", osd.DataPath,
		)

		// If the Bluestore OSD was prepared with ceph-volume and not running on PVC
	} else if osd.CephVolumeInitiated && osdProps.pvc.ClaimName == "" && !osd.IsFileStore {
		doBinaryCopyInit = false
		doConfigInit = false
		doActivateOSDInit = true
		command = []string{"ceph-osd"}
		args = []string{
			"--foreground",
			"--id", osdID,
			"--fsid", c.clusterInfo.FSID,
			"--setuser", "ceph",
			"--setgroup", "ceph",
			fmt.Sprintf("--crush-location=%s", osd.Location),
		}

		// Needed so that the init chown container won't chown something that does not exist
		// We don't need this directory, it's empty
		osd.DataPath = ""

		// If the OSD was prepared with ceph-volume and running on PVC
	} else if osd.CephVolumeInitiated && osdProps.pvc.ClaimName != "" {
		// if the osd was provisioned by ceph-volume, we need to launch it with rook as the parent process
		command = []string{path.Join(rookBinariesMountPath, "tini")}
		args = []string{
			"--", path.Join(rookBinariesMountPath, "rook"),
			"ceph", "osd", "start",
			"--",
			"--foreground",
			"--id", osdID,
			"--fsid", c.clusterInfo.FSID,
			"--cluster", "ceph",
			"--setuser", "ceph",
			"--setgroup", "ceph",
			// Set '--setuser-match-path' so that existing directory owned by root won't affect the daemon startup.
			// For existing data store owned by root, the daemon will continue to run as root
			"--setuser-match-path", osd.DataPath,
			fmt.Sprintf("--crush-location=%s", osd.Location),
		}

		// mount /run/udev in the container so ceph-volume (via `lvs`)
		// can access the udev database
		volumes = append(volumes, v1.Volume{
			Name: "run-udev",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: "/run/udev"}}})

		volumeMounts = append(volumeMounts, v1.VolumeMount{
			Name:      "run-udev",
			MountPath: "/run/udev"})

	} else if osd.IsDirectory {
		// config for dir-based osds is gotten from the commandline or from the mon database
		doConfigInit = false
		doBinaryCopyInit = false

		storeType := "bluestore"
		if osd.IsFileStore {
			storeType = "filestore"
		}

		command = []string{"ceph-osd"}
		args = append(
			opspec.DaemonFlags(c.clusterInfo, osdID),
			"--foreground",
			"--osd-data", osd.DataPath,
			"--osd-uuid", osd.UUID,
			"--osd-objectstore", storeType,
			"--osd-max-object-name-len", "256",
			"--osd-max-object-namespace-len", "64",
			"--crush-location", fmt.Sprintf("root=default host=%s", osdProps.crushHostname),
		)
	} else {
		// other osds can launch the osd daemon directly
		command = []string{"ceph-osd"}
		args = defaultArgs
	}

	if osd.IsFileStore {
		args = append(args, fmt.Sprintf("--osd-journal=%s", osd.Journal))
	}

	// Add the volume to the spec and the mount to the daemon container
	copyBinariesVolume, copyBinariesContainer := c.getCopyBinariesContainer()
	if doBinaryCopyInit {
		volumes = append(volumes, copyBinariesVolume)
		volumeMounts = append(volumeMounts, copyBinariesContainer.VolumeMounts[0])
	}

	// Add the volume to the spec and the mount to the daemon container
	// so that it can pick the already mounted/activated osd metadata path
	// This container will activate the OSD and place the activated filesinto an empty dir
	// The empty dir will be shared by the "activate-osd" pod and the "osd" main pod
	activateOSDVolume, activateOSDContainer := c.getActivateOSDInitContainer(osdID, osd.UUID, osd.IsFileStore, osdProps)
	if doActivateOSDInit {
		volumes = append(volumes, activateOSDVolume)
		volumeMounts = append(volumeMounts, activateOSDContainer.VolumeMounts[0])
	}

	args = append(args, commonArgs...)

	if osdProps.pvc.ClaimName != "" {
		volumeMounts = append(volumeMounts, getPvcOSDBridgeMount(osdProps.pvc.ClaimName))
		envVars = append(envVars, pvcBackedOSDEnvVar("true"))
		envVars = append(envVars, lvPathEnvVariable(osd.LVPath))
		envVars = append(envVars, lvBackedPVEnvVar(strconv.FormatBool(osd.LVBackedPV)))
	}

	privileged := true
	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		Privileged:             &privileged,
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
	}

	// needed for luksOpen synchronization when devices are encrypted
	hostIPC := osdProps.storeConfig.EncryptedDevice

	DNSPolicy := v1.DNSClusterFirst
	if c.Network.IsHost() {
		DNSPolicy = v1.DNSClusterFirstWithHostNet
	}

	initContainers := make([]v1.Container, 0, 3)
	if doConfigInit {
		initContainers = append(initContainers,
			v1.Container{
				Args:            []string{"ceph", "osd", "init"},
				Name:            opspec.ConfigInitContainerName,
				Image:           k8sutil.MakeRookImage(c.rookVersion),
				VolumeMounts:    configVolumeMounts,
				Env:             configEnvVars,
				SecurityContext: securityContext,
			})
	}
	if doBinaryCopyInit {
		initContainers = append(initContainers, *copyBinariesContainer)
	}
	if doActivateOSDInit {
		initContainers = append(initContainers, *activateOSDContainer)
	}

	// Doing a chown in a post start lifecycle hook does not reliably complete before the OSD
	// process starts, which can cause the pod to fail without the lifecycle hook's chown command
	// completing. It can take an arbitrarily long time for a pod restart to successfully chown the
	// directory. This is a race condition for all OSDs; therefore, do this in an init container.
	// See more discussion here: https://github.com/rook/rook/pull/3594#discussion_r312279176
	dataPath := ""
	if doChownDataPath {
		dataPath = osd.DataPath
	}
	initContainers = append(initContainers,
		opspec.ChownCephDataDirsInitContainer(
			opconfig.DataPathMap{ContainerDataDir: dataPath},
			c.cephVersion.Image,
			volumeMounts,
			osdProps.resources,
			securityContext,
		))

	deployment := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: c.Namespace,
			Labels:    c.getOSDLabels(osd.ID, failureDomainValue, osdProps.portable),
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					k8sutil.AppAttr:     AppName,
					k8sutil.ClusterAttr: c.Namespace,
					OsdIdLabelKey:       fmt.Sprintf("%d", osd.ID),
				},
			},
			Strategy: apps.DeploymentStrategy{
				Type: apps.RecreateDeploymentStrategyType,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:   AppName,
					Labels: c.getOSDLabels(osd.ID, failureDomainValue, osdProps.portable),
				},
				Spec: v1.PodSpec{
					RestartPolicy:      v1.RestartPolicyAlways,
					ServiceAccountName: serviceAccountName,
					HostNetwork:        c.Network.IsHost(),
					HostPID:            true,
					HostIPC:            hostIPC,
					DNSPolicy:          DNSPolicy,
					PriorityClassName:  c.priorityClassName,
					InitContainers:     initContainers,
					Containers: []v1.Container{
						{
							Command:         command,
							Args:            args,
							Name:            "osd",
							Image:           c.cephVersion.Image,
							VolumeMounts:    volumeMounts,
							Env:             envVars,
							Resources:       osdProps.resources,
							SecurityContext: securityContext,
						},
					},
					Volumes: volumes,
				},
			},
			Replicas: &replicaCount,
		},
	}
	if osdProps.pvc.ClaimName != "" {
		deployment.Spec.Template.Spec.InitContainers = append(deployment.Spec.Template.Spec.InitContainers, c.getPVCInitContainer(osdProps.pvc))
		k8sutil.AddLabelToDeployement(OSDOverPVCLabelKey, osdProps.pvc.ClaimName, deployment)
		k8sutil.AddLabelToPod(OSDOverPVCLabelKey, osdProps.pvc.ClaimName, &deployment.Spec.Template)
	}
	if !osdProps.portable {
		deployment.Spec.Template.Spec.NodeSelector = map[string]string{v1.LabelHostname: osdProps.crushHostname}
	}
	// Replace default unreachable node toleration if the osd pod is portable and based in PVC
	if osdProps.pvc.ClaimName != "" && osdProps.portable {
		k8sutil.AddUnreachableNodeToleration(&deployment.Spec.Template.Spec)
	}

	k8sutil.AddRookVersionLabelToDeployment(deployment)
	c.annotations.ApplyToObjectMeta(&deployment.ObjectMeta)
	c.annotations.ApplyToObjectMeta(&deployment.Spec.Template.ObjectMeta)
	opspec.AddCephVersionLabelToDeployment(c.clusterInfo.CephVersion, deployment)
	opspec.AddCephVersionLabelToDeployment(c.clusterInfo.CephVersion, deployment)
	k8sutil.SetOwnerRef(&deployment.ObjectMeta, &c.ownerRef)
	if len(osdProps.pvc.ClaimName) == 0 {
		c.placement.ApplyToPodSpec(&deployment.Spec.Template.Spec)
	} else {
		osdProps.placement.ApplyToPodSpec(&deployment.Spec.Template.Spec)
	}

	return deployment, nil
}

// To get rook inside the container, the config init container needs to copy "tini" and "rook" binaries into a volume.
// Get the config flag so rook will copy the binaries and create the volume and mount that will be shared between
// the init container and the daemon container
func (c *Cluster) getCopyBinariesContainer() (v1.Volume, *v1.Container) {
	volume := v1.Volume{Name: rookBinariesVolumeName, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}}
	mount := v1.VolumeMount{Name: rookBinariesVolumeName, MountPath: rookBinariesMountPath}

	return volume, &v1.Container{
		Args: []string{
			"copy-binaries",
			"--copy-to-dir", rookBinariesMountPath},
		Name:         "copy-bins",
		Image:        k8sutil.MakeRookImage(c.rookVersion),
		VolumeMounts: []v1.VolumeMount{mount},
	}
}

// This container runs all the actions needed to activate an OSD before we can run the OSD process
func (c *Cluster) getActivateOSDInitContainer(osdID, osdUUID string, isFilestore bool, osdProps osdProperties) (v1.Volume, *v1.Container) {
	volume := v1.Volume{Name: activateOSDVolumeName, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}}
	envVars := osdActivateEnvVar()
	osdStore := "--bluestore"
	if isFilestore {
		osdStore = "--filestore"
	}

	// Build empty dir osd path to something like "/var/lib/ceph/osd/ceph-0"
	activateOSDMountPathID := activateOSDMountPath + osdID

	volMounts := []v1.VolumeMount{
		{Name: activateOSDVolumeName, MountPath: activateOSDMountPathID},
		{Name: "devices", MountPath: "/dev"},
		{Name: k8sutil.ConfigOverrideName, ReadOnly: true, MountPath: opconfig.EtcCephDir},
	}

	privileged := true
	securityContext := &v1.SecurityContext{
		Privileged: &privileged,
	}

	return volume, &v1.Container{
		Command: []string{
			"/bin/bash",
			"-c",
			fmt.Sprintf(activateOSDCode, osdID, osdUUID, osdStore),
		},
		Name:            "activate-osd",
		Image:           c.cephVersion.Image,
		VolumeMounts:    volMounts,
		SecurityContext: securityContext,
		Env:             envVars,
		Resources:       osdProps.resources,
	}
}

func (c *Cluster) provisionPodTemplateSpec(osdProps osdProperties, restart v1.RestartPolicy, provisionConfig *provisionConfig) (*v1.PodTemplateSpec, error) {

	copyBinariesVolume, copyBinariesContainer := c.getCopyBinariesContainer()

	// ceph-volume is currently set up to use /etc/ceph/ceph.conf; this means no user config
	// overrides will apply to ceph-volume, but this is unnecessary anyway
	volumes := append(opspec.PodVolumes(provisionConfig.DataPathMap, c.dataDirHostPath, true), copyBinariesVolume)

	// by default, don't define any volume config unless it is required
	if len(osdProps.devices) > 0 || osdProps.selection.DeviceFilter != "" || osdProps.selection.DevicePathFilter != "" || osdProps.selection.GetUseAllDevices() || osdProps.metadataDevice != "" || osdProps.pvc.ClaimName != "" {
		// create volume config for the data dir and /dev so the pod can access devices on the host
		devVolume := v1.Volume{Name: "devices", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/dev"}}}
		volumes = append(volumes, devVolume)
		udevVolume := v1.Volume{Name: "udev", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/run/udev"}}}
		volumes = append(volumes, udevVolume)
	}

	if osdProps.pvc.ClaimName != "" {
		// Create volume config for PVCs
		volumes = append(volumes, getPVCOSDVolumes(&osdProps)...)
	}

	// add each OSD directory as another host path volume source
	for _, d := range osdProps.selection.Directories {
		if c.skipVolumeForDirectory(d.Path) {
			// the dataDirHostPath has already been added as a volume
			continue
		}
		dirVolume := v1.Volume{
			Name:         k8sutil.PathToVolumeName(d.Path),
			VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: d.Path}},
		}
		volumes = append(volumes, dirVolume)
	}

	if len(volumes) == 0 {
		return nil, errors.New("empty volumes")
	}

	podSpec := v1.PodSpec{
		ServiceAccountName: serviceAccountName,
		InitContainers: []v1.Container{
			*copyBinariesContainer,
		},
		Containers: []v1.Container{
			c.provisionOSDContainer(osdProps, copyBinariesContainer.VolumeMounts[0], provisionConfig),
		},
		RestartPolicy:     restart,
		Volumes:           volumes,
		HostNetwork:       c.Network.IsHost(),
		PriorityClassName: c.priorityClassName,
	}
	if c.Network.IsHost() {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	if len(osdProps.pvc.ClaimName) == 0 {
		c.placement.ApplyToPodSpec(&podSpec)
	} else {
		osdProps.placement.ApplyToPodSpec(&podSpec)
	}

	podMeta := metav1.ObjectMeta{
		Name: AppName,
		Labels: map[string]string{
			k8sutil.AppAttr:     prepareAppName,
			k8sutil.ClusterAttr: c.Namespace,
			OSDOverPVCLabelKey:  osdProps.pvc.ClaimName,
		},
		Annotations: map[string]string{},
	}

	c.annotations.ApplyToObjectMeta(&podMeta)

	// ceph-volume --dmcrypt uses cryptsetup that synchronizes with udev on
	// host through semaphore
	podSpec.HostIPC = osdProps.storeConfig.EncryptedDevice

	return &v1.PodTemplateSpec{
		ObjectMeta: podMeta,
		Spec:       podSpec,
	}, nil
}

// Currently we can't mount a block mode pv directly to a privileged container
// So we mount it to a non privileged init container and then copy it to a common directory mounted inside init container
// and the privileged provision container.
func (c *Cluster) getPVCInitContainer(pvc v1.PersistentVolumeClaimVolumeSource) v1.Container {
	return v1.Container{
		Name:  blockPVCMapperInitContainer,
		Image: c.cephVersion.Image,
		Command: []string{
			"cp",
		},
		Args: []string{"-a", fmt.Sprintf("/%s", pvc.ClaimName), fmt.Sprintf("/mnt/%s", pvc.ClaimName)},
		VolumeDevices: []v1.VolumeDevice{
			{
				Name:       pvc.ClaimName,
				DevicePath: fmt.Sprintf("/%s", pvc.ClaimName),
			},
		},
		VolumeMounts: []v1.VolumeMount{
			{
				MountPath: "/mnt",
				Name:      fmt.Sprintf("%s-bridge", pvc.ClaimName),
			},
		},
		SecurityContext: opmon.PodSecurityContext(),
	}
}

func (c *Cluster) getConfigEnvVars(osdProps osdProperties, dataDir string) []v1.EnvVar {
	envVars := []v1.EnvVar{
		nodeNameEnvVar(osdProps.crushHostname),
		{Name: "ROOK_CLUSTER_ID", Value: string(c.ownerRef.UID)},
		k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
		k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
		opmon.ClusterNameEnvVar(c.Namespace),
		opmon.EndpointEnvVar(),
		opmon.SecretEnvVar(),
		opmon.AdminSecretEnvVar(),
		k8sutil.ConfigDirEnvVar(dataDir),
		k8sutil.ConfigOverrideEnvVar(),
		{Name: "ROOK_FSID", ValueFrom: &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				LocalObjectReference: v1.LocalObjectReference{Name: "rook-ceph-mon"},
				Key:                  "fsid",
			},
		}},
		k8sutil.NodeEnvVar(),
	}

	// Give a hint to the prepare pod for what the host in the CRUSH map should be
	crushmapHostname := osdProps.crushHostname
	if !osdProps.portable && osdProps.pvc.ClaimName != "" {
		// If it's a pvc that's not portable we only know what the host name should be when inside the osd prepare pod
		crushmapHostname = ""
	}
	envVars = append(envVars, v1.EnvVar{Name: "ROOK_CRUSHMAP_HOSTNAME", Value: crushmapHostname})

	// Append ceph-volume environment variables
	envVars = append(envVars, cephVolumeEnvVar()...)

	// deliberately skip setting osdStoreEnvVarName (ROOK_OSD_STORE) as a quick means to deprecate
	// creating new disk-based Filestore OSDs

	if osdProps.storeConfig.DatabaseSizeMB != 0 {
		envVars = append(envVars, v1.EnvVar{Name: osdDatabaseSizeEnvVarName, Value: strconv.Itoa(osdProps.storeConfig.DatabaseSizeMB)})
	}

	if osdProps.storeConfig.WalSizeMB != 0 {
		envVars = append(envVars, v1.EnvVar{Name: osdWalSizeEnvVarName, Value: strconv.Itoa(osdProps.storeConfig.WalSizeMB)})
	}

	if osdProps.storeConfig.JournalSizeMB != 0 {
		envVars = append(envVars, v1.EnvVar{Name: osdJournalSizeEnvVarName, Value: strconv.Itoa(osdProps.storeConfig.JournalSizeMB)})
	}

	if osdProps.storeConfig.OSDsPerDevice != 0 {
		envVars = append(envVars, v1.EnvVar{Name: osdsPerDeviceEnvVarName, Value: strconv.Itoa(osdProps.storeConfig.OSDsPerDevice)})
	}

	if osdProps.storeConfig.EncryptedDevice {
		envVars = append(envVars, v1.EnvVar{Name: encryptedDeviceEnvVarName, Value: "true"})
	}

	return envVars
}

func (c *Cluster) provisionOSDContainer(osdProps osdProperties, copyBinariesMount v1.VolumeMount, provisionConfig *provisionConfig) v1.Container {

	envVars := c.getConfigEnvVars(osdProps, k8sutil.DataDir)

	devMountNeeded := false
	if osdProps.pvc.ClaimName != "" {
		devMountNeeded = true
	}
	privileged := false

	// only 1 of device list, device filter, device path filter and use all devices can be specified.  We prioritize in that order.
	if len(osdProps.devices) > 0 {
		deviceNames := make([]string, len(osdProps.devices))
		for i, device := range osdProps.devices {
			devSuffix := ""
			if count, ok := device.Config[config.OSDsPerDeviceKey]; ok {
				logger.Infof("%s osds requested on device %s (node %s)", count, device.Name, osdProps.crushHostname)
				devSuffix += ":" + count
			} else {
				devSuffix += ":1"
			}
			if databaseSizeMB, ok := device.Config[config.DatabaseSizeMBKey]; ok {
				logger.Infof("osd %s requested with DB size %sMB (node %s)", device.Name, databaseSizeMB, osdProps.crushHostname)
				devSuffix += ":" + databaseSizeMB
			} else {
				devSuffix += ":"
			}
			if deviceClass, ok := device.Config[config.DeviceClassKey]; ok {
				logger.Infof("osd %s requested with deviceClass %s (node %s)", device.Name, deviceClass, osdProps.crushHostname)
				devSuffix += ":" + deviceClass
			} else {
				devSuffix += ":"
			}
			if md, ok := device.Config[config.MetadataDeviceKey]; ok {
				logger.Infof("osd %s requested with metadataDevice %s (node %s)", device.Name, md, osdProps.crushHostname)
				devSuffix += ":" + md
			} else {
				devSuffix += ":"
			}
			deviceNames[i] = device.Name + devSuffix
		}
		envVars = append(envVars, dataDevicesEnvVar(strings.Join(deviceNames, ",")))
		devMountNeeded = true
	} else if osdProps.selection.DeviceFilter != "" {
		envVars = append(envVars, deviceFilterEnvVar(osdProps.selection.DeviceFilter))
		devMountNeeded = true
	} else if osdProps.selection.DevicePathFilter != "" {
		envVars = append(envVars, devicePathFilterEnvVar(osdProps.selection.DevicePathFilter))
		devMountNeeded = true
	} else if osdProps.selection.GetUseAllDevices() {
		envVars = append(envVars, deviceFilterEnvVar("all"))
		devMountNeeded = true
	}
	envVars = append(envVars, v1.EnvVar{Name: "ROOK_CEPH_VERSION", Value: c.clusterInfo.CephVersion.CephVersionFormatted()})

	if osdProps.metadataDevice != "" {
		envVars = append(envVars, metadataDeviceEnvVar(osdProps.metadataDevice))
		devMountNeeded = true
	}

	// ceph-volume is currently set up to use /etc/ceph/ceph.conf; this means no user config
	// overrides will apply to ceph-volume, but this is unnecessary anyway
	volumeMounts := append(opspec.CephVolumeMounts(provisionConfig.DataPathMap, true), copyBinariesMount)
	if devMountNeeded {
		devMount := v1.VolumeMount{Name: "devices", MountPath: "/dev"}
		volumeMounts = append(volumeMounts, devMount)
		udevMount := v1.VolumeMount{Name: "udev", MountPath: "/run/udev"}
		volumeMounts = append(volumeMounts, udevMount)
	}

	if osdProps.pvc.ClaimName != "" {
		volumeMounts = append(volumeMounts, getPvcOSDBridgeMount(osdProps.pvc.ClaimName))
		envVars = append(envVars, dataDevicesEnvVar(strings.Join([]string{fmt.Sprintf("/mnt/%s", osdProps.pvc.ClaimName)}, ",")))
		envVars = append(envVars, pvcBackedOSDEnvVar("true"))
	}

	if len(osdProps.selection.Directories) > 0 {
		// for each directory the user has specified, create a volume mount and pass it to the pod via cmd line arg
		dirPaths := make([]string, len(osdProps.selection.Directories))
		for i := range osdProps.selection.Directories {
			dpath := osdProps.selection.Directories[i].Path
			dirPaths[i] = dpath
			if c.skipVolumeForDirectory(dpath) {
				// the dataDirHostPath has already been added as a volume mount
				continue
			}
			volumeMounts = append(volumeMounts, v1.VolumeMount{Name: k8sutil.PathToVolumeName(dpath), MountPath: dpath})
		}
		envVars = append(envVars, dataDirectoriesEnvVar(strings.Join(dirPaths, ",")))
	}

	// elevate to be privileged if it is going to mount devices or if running in a restricted environment such as openshift
	if devMountNeeded || os.Getenv("ROOK_HOSTPATH_REQUIRES_PRIVILEGED") == "true" || osdProps.pvc.ClaimName != "" {
		privileged = true
	}
	runAsUser := int64(0)
	runAsNonRoot := false
	readOnlyRootFilesystem := false

	osdProvisionContainer := v1.Container{
		Command:      []string{path.Join(rookBinariesMountPath, "tini")},
		Args:         []string{"--", path.Join(rookBinariesMountPath, "rook"), "ceph", "osd", "provision"},
		Name:         "provision",
		Image:        c.cephVersion.Image,
		VolumeMounts: volumeMounts,
		Env:          envVars,
		SecurityContext: &v1.SecurityContext{
			Privileged:             &privileged,
			RunAsUser:              &runAsUser,
			RunAsNonRoot:           &runAsNonRoot,
			ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
		},
		Resources: c.prepareResources,
	}

	return osdProvisionContainer
}

func getPvcOSDBridgeMount(claimName string) v1.VolumeMount {
	return v1.VolumeMount{Name: fmt.Sprintf("%s-bridge", claimName), MountPath: "/mnt"}
}

func (c *Cluster) skipVolumeForDirectory(path string) bool {
	// If attempting to add a directory at /var/lib/rook, we need to skip the volume and volume mount
	// since the dataDirHostPath is always mounting at /var/lib/rook
	return path == k8sutil.DataDir
}

func getPVCOSDVolumes(osdProps *osdProperties) []v1.Volume {
	return []v1.Volume{
		{
			Name: osdProps.pvc.ClaimName,
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &osdProps.pvc,
			},
		},
		{
			// We need a bridge mount which is basically a common volume mount between the non privileged init container
			// and the privileged provision container or osd daemon container
			// The reason for this is mentioned in the comment for getPVCInitContainer() method
			Name: fmt.Sprintf("%s-bridge", osdProps.pvc.ClaimName),
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{
					Medium: "Memory",
				},
			},
		},
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

func devicePathFilterEnvVar(filter string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_DATA_DEVICE_PATH_FILTER", Value: filter}
}

func metadataDeviceEnvVar(metadataDevice string) v1.EnvVar {
	return v1.EnvVar{Name: osdMetadataDeviceEnvVarName, Value: metadataDevice}
}

func dataDirectoriesEnvVar(dataDirectories string) v1.EnvVar {
	return v1.EnvVar{Name: dataDirsEnvVarName, Value: dataDirectories}
}

func pvcBackedOSDEnvVar(pvcBacked string) v1.EnvVar {
	return v1.EnvVar{Name: pvcBackedOSDVarName, Value: pvcBacked}
}

func lvPathEnvVariable(lvPath string) v1.EnvVar {
	return v1.EnvVar{Name: lvPathVarName, Value: lvPath}
}

func lvBackedPVEnvVar(lvBackedPV string) v1.EnvVar {
	return v1.EnvVar{Name: lvBackedPVVarName, Value: lvBackedPV}
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

func osdOnSDNFlag(network cephv1.NetworkSpec, v cephver.CephVersion) []string {
	var args []string
	// OSD fails to find the right IP to bind to when running on SDN
	// for more details: https://github.com/rook/rook/issues/3140
	if !network.IsHost() {
		if v.IsAtLeast(cephver.CephVersion{Major: 14, Minor: 2, Extra: 2}) {
			args = append(args, "--ms-learn-addr-from-peer=false")
		}
	}

	return args
}

func makeStorageClassDeviceSetPVCID(storageClassDeviceSetName string, setIndex, pvcIndex int) (pvcID, pvcLabelSelector string) {
	pvcStorageClassDeviceSetPVCId := fmt.Sprintf("%s-%v", storageClassDeviceSetName, setIndex)
	return pvcStorageClassDeviceSetPVCId, fmt.Sprintf("%s=%s", CephDeviceSetPVCIDLabelKey, pvcStorageClassDeviceSetPVCId)
}

func makeStorageClassDeviceSetPVCLabel(storageClassDeviceSetName, pvcStorageClassDeviceSetPVCId string, pvcIndex, setIndex int) map[string]string {
	return map[string]string{
		CephDeviceSetLabelKey:      storageClassDeviceSetName,
		CephSetIndexLabelKey:       fmt.Sprintf("%v", setIndex),
		CephDeviceSetPVCIDLabelKey: pvcStorageClassDeviceSetPVCId,
	}
}

func (c *Cluster) getOSDLabels(osdID int, failureDomainValue string, portable bool) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     AppName,
		k8sutil.ClusterAttr: c.Namespace,
		OsdIdLabelKey:       fmt.Sprintf("%d", osdID),
		FailureDomainKey:    failureDomainValue,
		portableKey:         strconv.FormatBool(portable),
	}
}

func cephVolumeEnvVar() []v1.EnvVar {
	return []v1.EnvVar{
		{Name: "CEPH_VOLUME_DEBUG", Value: "1"},
		{Name: "CEPH_VOLUME_SKIP_RESTORECON", Value: "1"},
		// LVM will avoid interaction with udev.
		// LVM will manage the relevant nodes in /dev directly.
		{Name: "DM_DISABLE_UDEV", Value: "1"},
	}
}

func osdActivateEnvVar() []v1.EnvVar {
	monEnvVars := []v1.EnvVar{
		{Name: "ROOK_CEPH_MON_HOST",
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{
					Name: "rook-ceph-config"},
					Key: "mon_host"}}},
		{Name: "CEPH_ARGS", Value: "-m $(ROOK_CEPH_MON_HOST)"},
	}

	return append(cephVolumeEnvVar(), monEnvVars...)
}

func (c *Cluster) osdRunFlagTuningOnPVC(osdID int) error {
	who := fmt.Sprintf("osd.%d", osdID)
	do := make(map[string]string)

	// Time in seconds to sleep before next recovery or backfill op
	do["osd_recovery_sleep"] = osdRecoverySleep
	// Time in seconds to sleep before next snap trim
	do["osd_snap_trim_sleep"] = osdSnapTrimSleep
	// Time in seconds to sleep before next removal transaction
	do["osd_delete_sleep"] = osdDeleteSleep

	monStore := opconfig.GetMonStore(c.context, c.Namespace)

	for flag, val := range do {
		err := monStore.Set(who, flag, val)
		if err != nil {
			return errors.Wrapf(err, "failed to set %q to %q on %q", flag, val, who)
		}
	}

	return nil
}
