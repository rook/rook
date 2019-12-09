/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package spec provides Kubernetes controller/pod/container spec items used for many Ceph daemons
package spec

import (
	"fmt"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/display"
	v1 "k8s.io/api/core/v1"
)

const (
	// ConfigInitContainerName is the name which is given to the config initialization container
	// in all Ceph pods.
	ConfigInitContainerName = "config-init"
	logVolumeName           = "rook-ceph-log"
	volumeMountSubPath      = "data"
	crashVolumeName         = "rook-ceph-crash"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "ceph-spec")

// return the volume and matching volume mount for mounting the config override ConfigMap into
// containers as "/rook/ceph/ceph.conf".
func configOverrideConfigMapVolumeAndMount() (v1.Volume, v1.VolumeMount) {
	name := k8sutil.ConfigOverrideName // configmap name and name of volume
	dir := config.EtcCephDir
	file := "ceph.conf"
	// TL;DR: mount the configmap's "config" to a file called "ceph.conf" with 0444 permissions
	// security: allow to be read by everyone since now ceph processes run as 'ceph' and not 'root' user
	// Further investigation needs to be done to copy the ceph.conf and change its ownership
	// since configuring a owner of a ConfigMap secret is currently impossible
	// This also works around the following issue: https://tracker.ceph.com/issues/38606
	//
	// This design choice avoids the crash/restart situation in Rook
	// If we don't set 0444 to the ceph.conf configuration file during its respawn (with exec) the ceph-mgr
	// won't be able to read the ceph.conf and the container will die, the "restart" count will increase in k8s
	// This will mislead users thinking something won't wrong but that a false positive
	mode := int32(0444)
	v := v1.Volume{Name: name, VolumeSource: v1.VolumeSource{
		ConfigMap: &v1.ConfigMapVolumeSource{LocalObjectReference: v1.LocalObjectReference{
			Name: name},
			Items: []v1.KeyToPath{
				{Key: k8sutil.ConfigOverrideVal, Path: file, Mode: &mode},
			},
		}}}

	// configmap's "config" to "/etc/ceph/ceph.conf"
	m := v1.VolumeMount{
		Name:      name,
		ReadOnly:  true, // should be no reason to write to the config in pods, so enforce this
		MountPath: dir,
	}

	return v, m
}

func confGeneratedInPodVolumeAndMount() (v1.Volume, v1.VolumeMount) {
	name := "ceph-conf-emptydir"
	dir := config.EtcCephDir
	v := v1.Volume{Name: name, VolumeSource: v1.VolumeSource{
		EmptyDir: &v1.EmptyDirVolumeSource{}}}
	// configmap's "config" to "/etc/ceph/ceph.conf"
	m := v1.VolumeMount{
		Name:      name,
		MountPath: dir,
	}
	return v, m
}

// PodVolumes fills in the volumes parameter with the common list of Kubernetes volumes for use in Ceph pods.
// This function is only used for OSDs.
func PodVolumes(dataPaths *config.DataPathMap, dataDirHostPath string, confGeneratedInPod bool) []v1.Volume {

	dataDirSource := v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}
	if dataDirHostPath != "" {
		dataDirSource = v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: dataDirHostPath}}
	}
	configVolume, _ := configOverrideConfigMapVolumeAndMount()
	if confGeneratedInPod {
		configVolume, _ = confGeneratedInPodVolumeAndMount()
	}

	v := []v1.Volume{
		{Name: k8sutil.DataDirVolume, VolumeSource: dataDirSource},
		configVolume,
	}
	v = append(v, StoredLogAndCrashVolume(dataPaths.HostLogDir(), dataPaths.HostCrashDir())...)

	return v
}

// CephVolumeMounts returns the common list of Kubernetes volume mounts for Ceph containers.
// This function is only used for OSDs.
func CephVolumeMounts(dataPaths *config.DataPathMap, confGeneratedInPod bool) []v1.VolumeMount {
	_, configMount := configOverrideConfigMapVolumeAndMount()
	if confGeneratedInPod {
		_, configMount = confGeneratedInPodVolumeAndMount()
	}

	v := []v1.VolumeMount{
		{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
		configMount,
		// Rook doesn't run in ceph containers, so it doesn't need the config override mounted
	}
	v = append(v, StoredLogAndCrashVolumeMount(dataPaths.ContainerLogDir(), dataPaths.ContainerCrashDir())...)

	return v
}

// RookVolumeMounts returns the common list of Kubernetes volume mounts for Rook containers.
// This function is only used by OSDs.
func RookVolumeMounts(dataPaths *config.DataPathMap, confGeneratedInPod bool) []v1.VolumeMount {
	return append(
		CephVolumeMounts(dataPaths, confGeneratedInPod),
	)
}

// DaemonVolumesBase returns the common / static set of volumes.
func DaemonVolumesBase(dataPaths *config.DataPathMap, keyringResourceName string) []v1.Volume {
	configOverrideVolume, _ := configOverrideConfigMapVolumeAndMount()
	vols := []v1.Volume{
		configOverrideVolume,
	}
	if keyringResourceName != "" {
		vols = append(vols, keyring.Volume().Resource(keyringResourceName))
	}
	if dataPaths.HostLogAndCrashDir != "" {
		// logs are not persisted to host
		vols = append(vols, StoredLogAndCrashVolume(dataPaths.HostLogDir(), dataPaths.HostCrashDir())...)
	}
	return vols
}

// DaemonVolumesDataPVC returns a PVC volume source for daemon container data.
func DaemonVolumesDataPVC(pvcName string) v1.Volume {
	return v1.Volume{
		Name: "ceph-daemon-data",
		VolumeSource: v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvcName,
			},
		},
	}
}

// DaemonVolumesDataHostPath returns HostPath volume source for daemon container
// data.
func DaemonVolumesDataHostPath(dataPaths *config.DataPathMap) []v1.Volume {
	vols := []v1.Volume{}
	if dataPaths.ContainerDataDir == "" {
		// no data is stored in container, and therefore no data can be persisted to host
		return vols
	}
	// when data is not persisted to host, the data may still be shared between init/run containers
	src := v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}
	if dataPaths.HostDataDir != "" {
		// data is persisted to host
		src = v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: dataPaths.HostDataDir}}
	}
	return append(vols, v1.Volume{Name: "ceph-daemon-data", VolumeSource: src})
}

// DaemonVolumesContainsPVC returns true if a volume exists with a volume source
// configured with a persistent volume claim.
func DaemonVolumesContainsPVC(volumes []v1.Volume) bool {
	for _, volume := range volumes {
		if volume.VolumeSource.PersistentVolumeClaim != nil {
			return true
		}
	}
	return false
}

// DaemonVolumes returns the pod volumes used by all Ceph daemons. If keyring resource name is
// empty, there will be no keyring volume created from a secret.
func DaemonVolumes(dataPaths *config.DataPathMap, keyringResourceName string) []v1.Volume {
	vols := DaemonVolumesBase(dataPaths, keyringResourceName)
	vols = append(vols, DaemonVolumesDataHostPath(dataPaths)...)
	return vols
}

// DaemonVolumeMounts returns volume mounts which correspond to the DaemonVolumes. These
// volume mounts are shared by most all Ceph daemon containers, both init and standard. If keyring
// resource name is empty, there will be no keyring mounted in the container.
func DaemonVolumeMounts(dataPaths *config.DataPathMap, keyringResourceName string) []v1.VolumeMount {
	_, configOverrideMount := configOverrideConfigMapVolumeAndMount()
	mounts := []v1.VolumeMount{
		configOverrideMount,
	}
	if keyringResourceName != "" {
		mounts = append(mounts, keyring.VolumeMount().Resource(keyringResourceName))
	}
	if dataPaths.HostLogAndCrashDir != "" {
		// logs are not persisted to host, so no mount is needed
		mounts = append(mounts, StoredLogAndCrashVolumeMount(dataPaths.ContainerLogDir(), dataPaths.ContainerCrashDir())...)
	}
	if dataPaths.ContainerDataDir == "" {
		// no data is stored in container, so there are no more mounts
		return mounts
	}
	return append(mounts,
		v1.VolumeMount{Name: "ceph-daemon-data", MountPath: dataPaths.ContainerDataDir},
	)
}

// see AddVolumeMountSubPath
func addVolumeMountSubPathContainer(c *v1.Container, volumeMountName string) {
	for i := range c.VolumeMounts {
		v := &c.VolumeMounts[i]
		if v.Name == volumeMountName {
			v.SubPath = volumeMountSubPath
		}
	}
}

// AddVolumeMountSubPath updates each init and regular container of the podspec
// such that each volume mount attached to a container is mounted under a
// subpath in the source volume. This is important because some daemons may not
// start if the volume mount directory is non-empty. When the volume is the root
// of an ext4 file system, one may find a "lost+found" directory.
func AddVolumeMountSubPath(podSpec *v1.PodSpec, volumeMountName string) {
	for i := range podSpec.InitContainers {
		c := &podSpec.InitContainers[i]
		addVolumeMountSubPathContainer(c, volumeMountName)
	}
	for i := range podSpec.Containers {
		c := &podSpec.Containers[i]
		addVolumeMountSubPathContainer(c, volumeMountName)
	}
}

// DaemonFlags returns the command line flags used by all Ceph daemons.
func DaemonFlags(cluster *cephconfig.ClusterInfo, daemonID string) []string {
	return append(
		config.DefaultFlags(cluster.FSID, keyring.VolumeMount().KeyringFilePath(), cluster.CephVersion),
		config.NewFlag("id", daemonID),
		// Ceph daemons in Rook will run as 'ceph' instead of 'root'
		// If we run on a version of Ceph does not these flags it will simply ignore them
		//run ceph daemon process under the 'ceph' user
		config.NewFlag("setuser", "ceph"),
		// run ceph daemon process under the 'ceph' group
		config.NewFlag("setgroup", "ceph"),
	)

}

// AdminFlags returns the command line flags used for Ceph commands requiring admin authentication.
func AdminFlags(cluster *cephconfig.ClusterInfo) []string {
	return append(
		config.DefaultFlags(cluster.FSID, keyring.VolumeMount().AdminKeyringFilePath(), cluster.CephVersion),
		config.NewFlag("setuser", "ceph"),
		config.NewFlag("setgroup", "ceph"),
	)
}

// ContainerEnvVarReference returns a reference to a Kubernetes container env var of the given name
// which can be used in command or argument fields.
func ContainerEnvVarReference(envVarName string) string {
	return fmt.Sprintf("$(%s)", envVarName)
}

// DaemonEnvVars returns the container environment variables used by all Ceph daemons.
func DaemonEnvVars(image string) []v1.EnvVar {
	return append(
		k8sutil.ClusterDaemonEnvVars(image),
		config.StoredMonHostEnvVars()...,
	)
}

// AppLabels returns labels common for all Rook-Ceph applications which may be useful for admins.
// App name is the name of the application: e.g., 'rook-ceph-mon', 'rook-ceph-mgr', etc.
func AppLabels(appName, namespace string) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: namespace,
	}
}

// PodLabels returns pod labels common to all Rook-Ceph pods which may be useful for admins.
// App name is the name of the application: e.g., 'rook-ceph-mon', 'rook-ceph-mgr', etc.
// Daemon type is the Ceph daemon type: "mon", "mgr", "osd", "mds", "rgw"
// Daemon ID is the ID portion of the Ceph daemon name: "a" for "mon.a"; "c" for "mds.c"
func PodLabels(appName, namespace, daemonType, daemonID string) map[string]string {
	labels := AppLabels(appName, namespace)
	labels["ceph_daemon_id"] = daemonID
	// Also report the daemon id keyed by its daemon type: "mon: a", "mds: c", etc.
	labels[daemonType] = daemonID
	return labels
}

// CheckPodMemory verify pod's memory limit is valid
func CheckPodMemory(resources v1.ResourceRequirements, cephPodMinimumMemory uint64) error {
	// Ceph related PR: https://github.com/ceph/ceph/pull/26856
	podMemoryLimit := resources.Limits.Memory()
	podMemoryRequest := resources.Requests.Memory()
	errorMessage := `refuse to run the pod with %dmb of ram, provide at least %dmb.`

	// If nothing was provided let's just return
	// This means no restrictions on pod's resources
	if podMemoryLimit.IsZero() && podMemoryRequest.IsZero() {
		return nil
	}

	if !podMemoryLimit.IsZero() {
		// This means LIMIT and REQUEST are either identical or different but still we use LIMIT as a reference
		if uint64(podMemoryLimit.Value()) < display.MbTob(cephPodMinimumMemory) {
			return errors.Errorf(errorMessage, display.BToMb(uint64(podMemoryLimit.Value())), cephPodMinimumMemory)
		}

		// This means LIMIT < REQUEST
		// Kubernetes will refuse to schedule that pod however it's still valuable to indicate that user's input was incorrect
		if uint64(podMemoryLimit.Value()) < uint64(podMemoryRequest.Value()) {
			extraErrorLine := `\n
			User has specified a pod memory limit %dmb below the pod memory request %dmb in the cluster CR.\n
			Rook will create pods that are expected to fail to serve as a more apparent error indicator to the user.`

			return errors.Errorf(extraErrorLine, display.BToMb(uint64(podMemoryLimit.Value())), display.BToMb(uint64(podMemoryRequest.Value())))
		}
	}

	return nil
}

// ChownCephDataDirsInitContainer returns an init container which `chown`s the given data
// directories as the `ceph:ceph` user in the container. It also `chown`s the Ceph log dir in the
// container automatically.
// Doing a chown in a post start lifecycle hook does not reliably complete before the OSD
// process starts, which can cause the pod to fail without the lifecycle hook's chown command
// completing. It can take an arbitrarily long time for a pod restart to successfully chown the
// directory. This is a race condition for all daemons; therefore, do this in an init container.
// See more discussion here: https://github.com/rook/rook/pull/3594#discussion_r312279176
func ChownCephDataDirsInitContainer(
	dpm config.DataPathMap,
	containerImage string,
	volumeMounts []v1.VolumeMount,
	resources v1.ResourceRequirements,
	securityContext *v1.SecurityContext,
) v1.Container {
	args := make([]string, 0, 5)
	args = append(args,
		"--verbose",
		"--recursive",
		"ceph:ceph",
		config.VarLogCephDir,
		config.VarLibCephCrashDir,
	)
	if dpm.ContainerDataDir != "" {
		args = append(args, dpm.ContainerDataDir)
	}
	return v1.Container{
		Name:            "chown-container-data-dir",
		Command:         []string{"chown"},
		Args:            args,
		Image:           containerImage,
		VolumeMounts:    volumeMounts,
		Resources:       resources,
		SecurityContext: securityContext,
	}
}

// StoredLogAndCrashVolume returns a pod volume sourced from the stored log and crashes files.
func StoredLogAndCrashVolume(hostLogDir, hostCrashDir string) []v1.Volume {
	return []v1.Volume{
		{
			Name: logVolumeName,
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: hostLogDir},
			},
		},
		{
			Name: crashVolumeName,
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: hostCrashDir},
			},
		},
	}
}

// StoredLogAndCrashVolumeMount returns a pod volume sourced from the stored log and crashes files.
func StoredLogAndCrashVolumeMount(varLogCephDir, varLibCephCrashDir string) []v1.VolumeMount {
	return []v1.VolumeMount{
		{
			Name:      logVolumeName,
			ReadOnly:  false,
			MountPath: varLogCephDir,
		},
		{
			Name:      crashVolumeName,
			ReadOnly:  false,
			MountPath: varLibCephCrashDir,
		},
	}
}
