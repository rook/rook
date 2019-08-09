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
	"path"

	"github.com/coreos/pkg/capnslog"
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
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "ceph-spec")

// PodVolumes fills in the volumes parameter with the common list of Kubernetes volumes for use in Ceph pods.
func PodVolumes(dataDirHostPath, namespace string) []v1.Volume {
	dataDirSource := v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}
	if dataDirHostPath != "" {
		dataDirSource = v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: dataDirHostPath}}
	}
	return []v1.Volume{
		{Name: k8sutil.DataDirVolume, VolumeSource: dataDirSource},
		cephconfig.DefaultConfigVolume(),
		k8sutil.ConfigOverrideVolume(),
		StoredLogVolume(path.Join(dataDirHostPath, "log", namespace)),
	}
}

// CephVolumeMounts returns the common list of Kubernetes volume mounts for Ceph containers.
func CephVolumeMounts() []v1.VolumeMount {
	return []v1.VolumeMount{
		{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
		cephconfig.DefaultConfigMount(),
		StoredLogVolumeMount(),
		// Rook doesn't run in ceph containers, so it doesn't need the config override mounted
	}
}

// RookVolumeMounts returns the common list of Kubernetes volume mounts for Rook containers.
func RookVolumeMounts() []v1.VolumeMount {
	return append(
		CephVolumeMounts(),
		k8sutil.ConfigOverrideMount(),
	)
}

// DaemonVolumesBase returns the common / static set of volumes.
func DaemonVolumesBase(dataPaths *config.DataPathMap, keyringResourceName string) []v1.Volume {
	vols := []v1.Volume{
		config.StoredFileVolume(),
	}
	if keyringResourceName != "" {
		vols = append(vols, keyring.Volume().Resource(keyringResourceName))
	}
	if dataPaths.HostLogDir != "" {
		// logs are not persisted to host
		vols = append(vols, StoredLogVolume(dataPaths.HostLogDir))
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
	mounts := []v1.VolumeMount{
		config.StoredFileVolumeMount(),
	}
	if keyringResourceName != "" {
		mounts = append(mounts, keyring.VolumeMount().Resource(keyringResourceName))
	}
	if dataPaths.HostLogDir != "" {
		// logs are not persisted to host, so no mount is needed
		mounts = append(mounts, StoredLogVolumeMount())
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
			return fmt.Errorf(errorMessage, display.BToMb(uint64(podMemoryLimit.Value())), cephPodMinimumMemory)
		}

		// This means LIMIT < REQUEST
		// Kubernetes will refuse to schedule that pod however it's still valuable to indicate that user's input was incorrect
		if uint64(podMemoryLimit.Value()) < uint64(podMemoryRequest.Value()) {
			extraErrorLine := `\n
			User has specified a pod memory limit %dmb below the pod memory request %dmb in the cluster CR.\n
			Rook will create pods that are expected to fail to serve as a more apparent error indicator to the user.`

			return fmt.Errorf(extraErrorLine, display.BToMb(uint64(podMemoryLimit.Value())), display.BToMb(uint64(podMemoryRequest.Value())))
		}
	}

	return nil
}

// StoredLogVolume returns a pod volume sourced from the stored log files.
func StoredLogVolume(HostLogDir string) v1.Volume {
	return v1.Volume{
		Name: logVolumeName,
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{Path: HostLogDir},
		},
	}
}

// StoredLogVolumeMount returns a pod volume sourced from the stored log files.
func StoredLogVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      logVolumeName,
		ReadOnly:  false,
		MountPath: config.VarLogCephDir,
	}
}

func generateLifeCycleCmd(dataDirHostPath string) []string {
	cmd := config.ContainerPostStartCmd

	if dataDirHostPath != "" {
		cmd = append(cmd, dataDirHostPath)
	}

	return cmd
}

// PodLifeCycle returns a pod lifecycle resource to execute actions before a pod starts
func PodLifeCycle(dataDirHostPath string) *v1.Lifecycle {
	cmd := generateLifeCycleCmd(dataDirHostPath)

	return &v1.Lifecycle{
		PostStart: &v1.Handler{
			Exec: &v1.ExecAction{
				Command: cmd,
			},
		},
	}
}
