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

// DaemonVolumes returns the pod volumes used by all Ceph daemons.
func DaemonVolumes(dataPaths *config.DataPathMap, keyringResourceName string) []v1.Volume {
	vols := []v1.Volume{
		config.StoredFileVolume(),
		keyring.Volume().Resource(keyringResourceName),
		StoredLogVolume(dataPaths.HostLogDir),
	}
	if dataPaths.NoData {
		return vols
	}
	src := v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}
	if dataPaths.PersistData {
		src = v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: dataPaths.HostDataDir}}
	}
	return append(vols, v1.Volume{Name: "ceph-daemon-data", VolumeSource: src})
}

// DaemonVolumeMounts returns volume mounts which correspond to the DaemonVolumes. These
// volume mounts are shared by most all Ceph daemon containers, both init and standard.
func DaemonVolumeMounts(dataPaths *config.DataPathMap, keyringResourceName string) []v1.VolumeMount {
	mounts := []v1.VolumeMount{
		config.StoredFileVolumeMount(),
		keyring.VolumeMount().Resource(keyringResourceName),
		StoredLogVolumeMount(),
	}
	if dataPaths.NoData {
		return mounts
	}
	return append(mounts,
		v1.VolumeMount{Name: "ceph-daemon-data", MountPath: dataPaths.ContainerDataDir},
	)
}

// DaemonFlags returns the command line flags used by all Ceph daemons.
func DaemonFlags(cluster *cephconfig.ClusterInfo, daemonID string) []string {
	return append(
		config.DefaultFlags(cluster.FSID, keyring.VolumeMount().KeyringFilePath(), cluster.CephVersion),
		config.NewFlag("id", daemonID),
	)

}

// AdminFlags returns the command line flags used for Ceph commands requiring admin authentication.
func AdminFlags(cluster *cephconfig.ClusterInfo) []string {
	return config.DefaultFlags(cluster.FSID, keyring.VolumeMount().AdminKeyringFilePath(), cluster.CephVersion)
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
