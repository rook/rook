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

// Package controller provides Kubernetes controller/pod/container spec items used for many Ceph daemons
package controller

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path"
	"reflect"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/display"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// ConfigInitContainerName is the name which is given to the config initialization container
	// in all Ceph pods.
	ConfigInitContainerName                 = "config-init"
	logVolumeName                           = "rook-ceph-log"
	volumeMountSubPath                      = "data"
	crashVolumeName                         = "rook-ceph-crash"
	daemonSocketDir                         = "/run/ceph"
	daemonSocketsSubPath                    = "/exporter"
	logCollector                            = "log-collector"
	DaemonIDLabel                           = "ceph_daemon_id"
	daemonTypeLabel                         = "ceph_daemon_type"
	ExternalMgrAppName                      = "rook-ceph-mgr-external"
	ExternalCephExporterName                = "rook-ceph-exporter-external"
	ServiceExternalMetricName               = "http-external-metrics"
	CephUserID                              = int64(167)
	livenessProbeTimeoutSeconds       int32 = 5
	livenessProbeInitialDelaySeconds  int32 = 10
	startupProbeFailuresDaemonDefault int32 = 6 // multiply by 10 = effective startup timeout
	// The OSD requires a long timeout in case the OSD is taking extra time to
	// scrub data during startup. We don't want the probe to disrupt the OSD update
	// and restart the OSD prematurely. So we set a really long timeout to avoid
	// disabling the startup and liveness probes completely.
	// The default is two hours after multiplying by the 10s retry interval.
	startupProbeFailuresDaemonOSD int32 = 12 * 60
)

type daemonConfig struct {
	daemonType string
	daemonID   string
}

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "ceph-spec")

var (
	osdLivenessProbeScript = `
outp="$(ceph --admin-daemon %s %s 2>&1)"
rc=$?
if [ $rc -ne 0 ] && [ ! -f /tmp/osd-sleep ]; then
	echo "ceph daemon health check failed with the following output:"
	echo "$outp" | sed -e 's/^/> /g'
	exit $rc
fi
`

	livenessProbeScript = `
outp="$(ceph --admin-daemon %s %s 2>&1)"
rc=$?
if [ $rc -ne 0 ]; then
	echo "ceph daemon health check failed with the following output:"
	echo "$outp" | sed -e 's/^/> /g'
	exit $rc
fi
`

	cronLogRotate = `
CEPH_CLIENT_ID=%s
PERIODICITY=%s
LOG_ROTATE_CEPH_FILE=/etc/logrotate.d/ceph
LOG_MAX_SIZE=%s
ROTATE=%s
ADDITIONAL_LOG_FILES=%s

# edit the logrotate file to only rotate a specific daemon log
# otherwise we will logrotate log files without reloading certain daemons
# this might happen when multiple daemons run on the same machine
sed -i "s|*.log|$CEPH_CLIENT_ID.log $ADDITIONAL_LOG_FILES|" "$LOG_ROTATE_CEPH_FILE"

# replace default daily with given user input
sed --in-place "s/daily/$PERIODICITY/g" "$LOG_ROTATE_CEPH_FILE"

# replace rotate count, default 7 for all ceph daemons other than rbd-mirror
sed --in-place "s/rotate 7/rotate $ROTATE/g" "$LOG_ROTATE_CEPH_FILE"

if [ "$LOG_MAX_SIZE" != "0" ]; then
	# adding maxsize $LOG_MAX_SIZE at the 4th line of the logrotate config file with 4 spaces to maintain indentation
	sed --in-place "4i \ \ \ \ maxsize $LOG_MAX_SIZE" "$LOG_ROTATE_CEPH_FILE"
fi

while true; do
	# we don't force the logrorate but we let the logrotate binary handle the rotation based on user's input for periodicity and size
	logrotate --verbose "$LOG_ROTATE_CEPH_FILE"
	sleep 15m
done
`
)

// return the volume and matching volume mount for mounting the config override ConfigMap into
// containers as "/etc/ceph/ceph.conf".
func configOverrideConfigMapVolumeAndMount() (v1.Volume, v1.VolumeMount) {
	secretAndConfigMapVolumeProjections := []v1.VolumeProjection{}
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
	mode := int32(0o444)
	projectionConfigMap := &v1.ConfigMapProjection{Items: []v1.KeyToPath{{Key: k8sutil.ConfigOverrideVal, Path: file, Mode: &mode}}}
	projectionConfigMap.Name = name
	configMapProjection := v1.VolumeProjection{
		ConfigMap: projectionConfigMap,
	}
	secretAndConfigMapVolumeProjections = append(secretAndConfigMapVolumeProjections, configMapProjection)

	v := v1.Volume{
		Name: name,
		VolumeSource: v1.VolumeSource{
			Projected: &v1.ProjectedVolumeSource{
				Sources: secretAndConfigMapVolumeProjections,
			},
		},
	}

	// configmap's "config" to "/etc/ceph/ceph.conf"
	m := v1.VolumeMount{
		Name:      name,
		ReadOnly:  true, // should be no reason to write to the config in pods, so enforce this
		MountPath: dir,
	}

	return v, m
}

// ConfGeneratedInPodVolumeAndMount generate an empty dir of /etc/ceph
func ConfGeneratedInPodVolumeAndMount() (v1.Volume, v1.VolumeMount) {
	name := "ceph-conf-emptydir"
	dir := config.EtcCephDir
	v := v1.Volume{Name: name, VolumeSource: v1.VolumeSource{
		EmptyDir: &v1.EmptyDirVolumeSource{},
	}}
	// configmap's "config" to "/etc/ceph/ceph.conf"
	m := v1.VolumeMount{
		Name:      name,
		MountPath: dir,
	}
	return v, m
}

// PodVolumes fills in the volumes parameter with the common list of Kubernetes volumes for use in Ceph pods.
// This function is only used for OSDs.
func PodVolumes(dataPaths *config.DataPathMap, dataDirHostPath string, exporterHostPath string, confGeneratedInPod bool) []v1.Volume {
	dataDirSource := v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}
	if dataDirHostPath != "" {
		dataDirSource = v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: dataDirHostPath}}
	}
	hostPathType := v1.HostPathDirectoryOrCreate
	sockDirSource := v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: path.Join(exporterHostPath, daemonSocketsSubPath), Type: &hostPathType}}
	configVolume, _ := configOverrideConfigMapVolumeAndMount()
	if confGeneratedInPod {
		configVolume, _ = ConfGeneratedInPodVolumeAndMount()
	}

	v := []v1.Volume{
		{Name: k8sutil.DataDirVolume, VolumeSource: dataDirSource},
		configVolume,
	}
	v = append(v, v1.Volume{Name: "ceph-daemons-sock-dir", VolumeSource: sockDirSource})
	v = append(v, StoredLogAndCrashVolume(dataPaths.HostLogDir(), dataPaths.HostCrashDir())...)

	return v
}

// CephVolumeMounts returns the common list of Kubernetes volume mounts for Ceph containers.
// This function is only used for OSDs.
func CephVolumeMounts(dataPaths *config.DataPathMap, confGeneratedInPod bool) []v1.VolumeMount {
	_, configMount := configOverrideConfigMapVolumeAndMount()
	if confGeneratedInPod {
		_, configMount = ConfGeneratedInPodVolumeAndMount()
	}

	v := []v1.VolumeMount{
		{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
		configMount,
		// Rook doesn't run in ceph containers, so it doesn't need the config override mounted
	}
	v = append(v, v1.VolumeMount{Name: "ceph-daemons-sock-dir", MountPath: daemonSocketDir})
	v = append(v, StoredLogAndCrashVolumeMount(dataPaths.ContainerLogDir(), dataPaths.ContainerCrashDir())...)

	return v
}

// RookVolumeMounts returns the common list of Kubernetes volume mounts for Rook containers.
// This function is only used by OSDs.
func RookVolumeMounts(dataPaths *config.DataPathMap, confGeneratedInPod bool) []v1.VolumeMount {
	return CephVolumeMounts(dataPaths, confGeneratedInPod)
}

// DaemonVolumesBase returns the common / static set of volumes.
func DaemonVolumesBase(dataPaths *config.DataPathMap, keyringResourceName string, dataDirHostPath string) []v1.Volume {
	configOverrideVolume, _ := configOverrideConfigMapVolumeAndMount()
	vols := []v1.Volume{
		configOverrideVolume,
	}
	if keyringResourceName != "" {
		vols = append(vols, keyring.Volume().Resource(keyringResourceName))
	}
	// data is persisted to host
	if dataDirHostPath != "" {
		hostPathType := v1.HostPathDirectoryOrCreate
		src := v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: path.Join(dataDirHostPath, daemonSocketsSubPath), Type: &hostPathType}}
		vols = append(vols, v1.Volume{Name: "ceph-daemons-sock-dir", VolumeSource: src})
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
func DaemonVolumes(dataPaths *config.DataPathMap, keyringResourceName string, dataDirHostPath string) []v1.Volume {
	vols := DaemonVolumesBase(dataPaths, keyringResourceName, dataDirHostPath)
	vols = append(vols, DaemonVolumesDataHostPath(dataPaths)...)
	return vols
}

// DaemonVolumeMounts returns volume mounts which correspond to the DaemonVolumes. These
// volume mounts are shared by most all Ceph daemon containers, both init and standard. If keyring
// resource name is empty, there will be no keyring mounted in the container.
func DaemonVolumeMounts(dataPaths *config.DataPathMap, keyringResourceName string, dataDirHostPath string) []v1.VolumeMount {
	_, configOverrideMount := configOverrideConfigMapVolumeAndMount()
	mounts := []v1.VolumeMount{
		configOverrideMount,
	}
	if dataDirHostPath != "" {
		mounts = append(mounts, v1.VolumeMount{Name: "ceph-daemons-sock-dir", MountPath: daemonSocketDir})
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
func DaemonFlags(cluster *client.ClusterInfo, spec *cephv1.ClusterSpec, daemonID string) []string {
	flags := append(
		config.DefaultFlags(cluster.FSID, keyring.VolumeMount().KeyringFilePath()),
		config.NewFlag("id", daemonID),
		// Ceph daemons in Rook will run as 'ceph' instead of 'root'
		// If we run on a version of Ceph does not these flags it will simply ignore them
		// run ceph daemon process under the 'ceph' user
		config.NewFlag("setuser", "ceph"),
		// run ceph daemon process under the 'ceph' group
		config.NewFlag("setgroup", "ceph"),
	)
	flags = append(flags, NetworkBindingFlags(cluster, spec)...)

	return flags
}

// AdminFlags returns the command line flags used for Ceph commands requiring admin authentication.
func AdminFlags(cluster *client.ClusterInfo) []string {
	return append(
		config.DefaultFlags(cluster.FSID, keyring.VolumeMount().AdminKeyringFilePath()),
		config.NewFlag("setuser", "ceph"),
		config.NewFlag("setgroup", "ceph"),
	)
}

func NetworkBindingFlags(cluster *client.ClusterInfo, spec *cephv1.ClusterSpec) []string {
	var args []string

	// Ceph supports dual-stack, so setting IPv6 family without disabling IPv4 binding actually enables dual-stack
	// This is likely not user's intent, so let's make sure to disable IPv4 when IPv6 is selected
	if !spec.Network.DualStack {
		switch spec.Network.IPFamily {
		case cephv1.IPv4:
			args = append(args, config.NewFlag("ms-bind-ipv4", "true"))
			args = append(args, config.NewFlag("ms-bind-ipv6", "false"))

		case cephv1.IPv6:
			args = append(args, config.NewFlag("ms-bind-ipv4", "false"))
			args = append(args, config.NewFlag("ms-bind-ipv6", "true"))
		}
	} else {
		args = append(args, config.NewFlag("ms-bind-ipv4", "true"))
		args = append(args, config.NewFlag("ms-bind-ipv6", "true"))
	}

	return args
}

// ContainerEnvVarReference returns a reference to a Kubernetes container env var of the given name
// which can be used in command or argument fields.
func ContainerEnvVarReference(envVarName string) string {
	return fmt.Sprintf("$(%s)", envVarName)
}

// DaemonEnvVars returns the container environment variables used by all Ceph daemons.
func DaemonEnvVars(cephClusterSpec *cephv1.ClusterSpec) []v1.EnvVar {
	networkEnv := ApplyNetworkEnv(cephClusterSpec)
	cephDaemonsEnvVars := append(k8sutil.ClusterDaemonEnvVars(cephClusterSpec.CephVersion.Image), networkEnv...)

	return append(
		cephDaemonsEnvVars,
		config.StoredMonHostEnvVars()...,
	)
}

func ApplyNetworkEnv(cephClusterSpec *cephv1.ClusterSpec) []v1.EnvVar {
	if cephClusterSpec.Network.Connections != nil {
		msgr2Required := false
		encryptionEnabled := false
		compressionEnabled := false
		if cephClusterSpec.Network.Connections.RequireMsgr2 {
			msgr2Required = true
		}
		if cephClusterSpec.Network.Connections.Encryption != nil && cephClusterSpec.Network.Connections.Encryption.Enabled {
			encryptionEnabled = true
		}
		if cephClusterSpec.Network.Connections.Compression != nil && cephClusterSpec.Network.Connections.Compression.Enabled {
			compressionEnabled = true
		}
		envVarValue := fmt.Sprintf("msgr2_%t_encryption_%t_compression_%t", msgr2Required, encryptionEnabled, compressionEnabled)

		rookMsgr2Env := []v1.EnvVar{{
			Name:  "ROOK_MSGR2",
			Value: envVarValue,
		}}
		return rookMsgr2Env
	}
	return []v1.EnvVar{}
}

// AppLabels returns labels common for all Rook-Ceph applications which may be useful for admins.
// App name is the name of the application: e.g., 'rook-ceph-mon', 'rook-ceph-mgr', etc.
func AppLabels(appName, namespace string) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: namespace,
	}
}

// CephDaemonAppLabels returns pod labels common to all Rook-Ceph pods which may be useful for admins.
// App name is the name of the application: e.g., 'rook-ceph-mon', 'rook-ceph-mgr', etc
// Daemon type is the Ceph daemon type: "mon", "mgr", "osd", "mds", "rgw"
// Daemon ID is the ID portion of the Ceph daemon name: "a" for "mon.a"; "c" for "mds.c"
// ParentName is the resource metadata.name: "rook-ceph", "my-cluster", etc
// ResourceKind is the CR type: "CephCluster", "CephFilesystem", etc
func CephDaemonAppLabels(appName, namespace, daemonType, daemonID, parentName, resourceKind string, includeNewLabels bool) map[string]string {
	labels := AppLabels(appName, namespace)

	// New labels cannot be applied to match selectors during upgrade
	if includeNewLabels {
		labels[daemonTypeLabel] = daemonType
		k8sutil.AddRecommendedLabels(labels, "ceph-"+daemonType, parentName, resourceKind, daemonID)
	}
	labels[DaemonIDLabel] = daemonID
	// Also report the daemon id keyed by its daemon type: "mon: a", "mds: c", etc.
	labels[daemonType] = daemonID
	return labels
}

// CheckPodMemory verify pod's memory limit is valid
func CheckPodMemory(name string, resources v1.ResourceRequirements, cephPodMinimumMemory uint64) error {
	// Ceph related PR: https://github.com/ceph/ceph/pull/26856
	podMemoryLimit := resources.Limits.Memory()
	podMemoryRequest := resources.Requests.Memory()

	// If nothing was provided let's just return
	// This means no restrictions on pod's resources
	if podMemoryLimit.IsZero() && podMemoryRequest.IsZero() {
		return nil
	}

	if !podMemoryLimit.IsZero() {
		// This means LIMIT and REQUEST are either identical or different but still we use LIMIT as a reference
		// nolint:gosec // G115 int64 to uint64 conversion is reasonabe here
		upodMemoryLimit := uint64(podMemoryLimit.Value())
		if upodMemoryLimit < display.MbTob(cephPodMinimumMemory) {
			// allow the configuration if less than the min, but print a warning
			logger.Warningf("running the %q daemon(s) with %dMB of ram, but at least %dMB is recommended", name, display.BToMb(upodMemoryLimit), cephPodMinimumMemory)
		}

		// This means LIMIT < REQUEST
		// Kubernetes will refuse to schedule that pod however it's still valuable to indicate that user's input was incorrect
		// nolint:gosec // G115 int64 to uint64 conversion is reasonabe here
		upodMemoryRequest := uint64(podMemoryRequest.Value())
		if upodMemoryLimit < upodMemoryRequest {
			extraErrorLine := `\n
			User has specified a pod memory limit %dmb below the pod memory request %dmb in the cluster CR.\n
			Rook will create pods that are expected to fail to serve as a more apparent error indicator to the user.`

			return errors.Errorf(extraErrorLine, display.BToMb(upodMemoryLimit), display.BToMb(upodMemoryRequest))
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
	containerImagePullPolicy v1.PullPolicy,
	volumeMounts []v1.VolumeMount,
	resources v1.ResourceRequirements,
	securityContext *v1.SecurityContext,
	configDir string,
) v1.Container {
	args := make([]string, 0, 5)
	args = append(args,
		"--verbose",
		"--recursive",
		"ceph:ceph",
		config.VarLogCephDir,
		config.VarLibCephCrashDir,
		daemonSocketDir,
	)
	if configDir != "" {
		args = append(args, configDir)
	}

	if dpm.ContainerDataDir != "" {
		args = append(args, dpm.ContainerDataDir)
	}
	return v1.Container{
		Name:            "chown-container-data-dir",
		Command:         []string{"chown"},
		Args:            args,
		Image:           containerImage,
		ImagePullPolicy: containerImagePullPolicy,
		VolumeMounts:    volumeMounts,
		Resources:       resources,
		SecurityContext: securityContext,
	}
}

// GenerateMinimalCephConfInitContainer returns an init container that will generate the most basic
// Ceph config for connecting non-Ceph daemons to a Ceph cluster (e.g., nfs-ganesha). Effectively
// what this means is that it generates '/etc/ceph/ceph.conf' with 'mon_host' populated and a
// keyring path associated with the user given. 'mon_host' is determined by the 'ROOK_CEPH_MON_HOST'
// env var present in other Ceph daemon pods, and the keyring is expected to be mounted into the
// container with a Kubernetes pod volume+mount.
func GenerateMinimalCephConfInitContainer(
	username, keyringPath string,
	containerImage string,
	containerImagePullPolicy v1.PullPolicy,
	volumeMounts []v1.VolumeMount,
	resources v1.ResourceRequirements,
	securityContext *v1.SecurityContext,
) v1.Container {
	cfgPath := client.DefaultConfigFilePath()
	// Note that parameters like $(PARAM) will be replaced by Kubernetes with env var content before
	// container creation.
	confScript := `
set -xEeuo pipefail

cat << EOF > ` + cfgPath + `
[global]
mon_host = $(ROOK_CEPH_MON_HOST)

[` + username + `]
keyring = ` + keyringPath + `
EOF

chmod 444 ` + cfgPath + `

cat ` + cfgPath + `
`
	return v1.Container{
		Name:            "generate-minimal-ceph-conf",
		Command:         []string{"/bin/bash", "-c", confScript},
		Args:            []string{},
		Image:           containerImage,
		ImagePullPolicy: containerImagePullPolicy,
		VolumeMounts:    volumeMounts,
		Env:             config.StoredMonHostEnvVars(),
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

// GenerateLivenessProbeExecDaemon generates a liveness probe that makes sure a daemon has a socket,
// that it can be called, and that it returns 0
func GenerateLivenessProbeExecDaemon(daemonType, daemonID string) *v1.Probe {
	confDaemon := getDaemonConfig(daemonType, daemonID)
	probeScript := livenessProbeScript
	if daemonType == opconfig.OsdType {
		probeScript = osdLivenessProbeScript
	}

	return &v1.Probe{
		ProbeHandler: v1.ProbeHandler{
			Exec: &v1.ExecAction{
				// Run with env -i to clean env variables in the exec context
				// This avoids conflict with the CEPH_ARGS env
				//
				// Example:
				// env -i sh -c "ceph --admin-daemon /run/ceph/ceph-osd.0.asok status"
				//
				// Ceph gives pretty un-diagnostic error message when `ceph status` or `ceph mon_status` command fails.
				// Add a clear message after Ceph's to help.
				// ref: https://github.com/rook/rook/issues/9846
				Command: []string{
					"env",
					"-i",
					"sh",
					"-c",
					fmt.Sprintf(probeScript, confDaemon.buildSocketPath(), confDaemon.buildAdminSocketCommand()),
				},
			},
		},
		InitialDelaySeconds: livenessProbeInitialDelaySeconds,
		TimeoutSeconds:      livenessProbeTimeoutSeconds,
	}
}

// GenerateStartupProbeExecDaemon generates a startup probe that makes sure a daemon has a socket,
// that it can be called, and that it returns 0
func GenerateStartupProbeExecDaemon(daemonType, daemonID string) *v1.Probe {
	// startup probe is the same as the liveness probe, but with modified thresholds
	probe := GenerateLivenessProbeExecDaemon(daemonType, daemonID)

	// these are hardcoded to 10 so that the failure threshold can be easily multiplied by 10 to
	// give the effective startup timeout
	probe.InitialDelaySeconds = 10
	probe.PeriodSeconds = 10

	if daemonType == config.OsdType {
		probe.FailureThreshold = startupProbeFailuresDaemonOSD
	} else {
		probe.FailureThreshold = startupProbeFailuresDaemonDefault
	}

	return probe
}

func getDaemonConfig(daemonType, daemonID string) *daemonConfig {
	return &daemonConfig{
		daemonType: string(daemonType),
		daemonID:   daemonID,
	}
}

func (c *daemonConfig) buildSocketName() string {
	return fmt.Sprintf("ceph-%s.%s.asok", c.daemonType, c.daemonID)
}

func (c *daemonConfig) buildSocketPath() string {
	return path.Join(daemonSocketDir, c.buildSocketName())
}

func (c *daemonConfig) buildAdminSocketCommand() string {
	command := "status"
	if c.daemonType == config.MonType {
		command = "mon_status"
	}

	return command
}

func HostPathRequiresPrivileged() bool {
	return os.Getenv("ROOK_HOSTPATH_REQUIRES_PRIVILEGED") == "true"
}

// PodSecurityContext detects if the pod needs privileges to run
func PodSecurityContext() *v1.SecurityContext {
	privileged := HostPathRequiresPrivileged()

	return &v1.SecurityContext{
		Privileged: &privileged,
		Capabilities: &v1.Capabilities{
			Add: []v1.Capability{},
			Drop: []v1.Capability{
				"NET_RAW",
			},
		},
	}
}

// PodSecurityContext detects if the pod needs privileges to run
func CephSecurityContext() *v1.SecurityContext {
	context := PodSecurityContext()
	cephUserID := CephUserID
	context.RunAsUser = &cephUserID
	context.RunAsGroup = &cephUserID
	return context
}

// PrivilegedContext returns a privileged Pod security context
func PrivilegedContext(runAsRoot bool) *v1.SecurityContext {
	privileged := true
	rootUser := int64(0)

	sec := &v1.SecurityContext{
		Privileged: &privileged,
	}

	if runAsRoot {
		sec.RunAsUser = &rootUser
	}

	sec.Capabilities = &v1.Capabilities{
		Add: []v1.Capability{},
		Drop: []v1.Capability{
			"NET_RAW",
		},
	}
	return sec
}

func GetLogRotateConfig(c cephv1.ClusterSpec) (resource.Quantity, string) {
	var maxLogSize resource.Quantity
	if c.LogCollector.MaxLogSize != nil {
		size := c.LogCollector.MaxLogSize.Value() / 1000 / 1000
		if size == 0 {
			size = 1
			logger.Info("maxLogSize is 0M setting to minimum of 1M")
		}

		maxLogSize = resource.MustParse(fmt.Sprintf("%dM", size))
	}

	var periodicity string
	switch c.LogCollector.Periodicity {
	case "1h", "hourly":
		periodicity = "hourly"
	case "weekly", "monthly":
		periodicity = c.LogCollector.Periodicity
	default:
		periodicity = "daily"
	}

	return maxLogSize, periodicity
}

// LogCollectorContainer rotate logs
func LogCollectorContainer(daemonID, ns string, c cephv1.ClusterSpec, additionalLogFiles ...string) *v1.Container {
	maxLogSize, periodicity := GetLogRotateConfig(c)
	rotation := "7"

	if strings.Contains(daemonID, "-client.rbd-mirror") {
		rotation = "28"
	}

	// Convert the variadic string slice into a space-separated string
	additionalLogs := strings.Join(additionalLogFiles, " ")
	logger.Debugf("additional log file %q will be used for logCollector", additionalLogs)
	logger.Debugf("setting periodicity to %q. Supported periodicity are hourly, daily, weekly and monthly", periodicity)

	return &v1.Container{
		Name: logCollector,
		Command: []string{
			"/bin/bash",
			"-x", // Print commands and their arguments as they are executed
			"-e", // Exit immediately if a command exits with a non-zero status.
			"-m", // Terminal job control, allows job to be terminated by SIGTERM
			"-c", // Command to run
			fmt.Sprintf(cronLogRotate, daemonID, periodicity, maxLogSize.String(), rotation, additionalLogs),
		},
		Image:           c.CephVersion.Image,
		ImagePullPolicy: GetContainerImagePullPolicy(c.CephVersion.ImagePullPolicy),
		VolumeMounts:    DaemonVolumeMounts(config.NewDatalessDaemonDataPathMap(ns, c.DataDirHostPath), "", c.DataDirHostPath),
		SecurityContext: PodSecurityContext(),
		Resources:       cephv1.GetLogCollectorResources(c.Resources),
		// We need a TTY for the bash job control (enabled by -m)
		TTY: true,
	}
}

// rgw operations will be logged in sidecar ops-log
func RgwOpsLogSidecarContainer(opsLogFile, ns string, c cephv1.ClusterSpec, Resources v1.ResourceRequirements) *v1.Container {
	return &v1.Container{
		Name: "ops-log",
		Command: []string{
			"bash",
			"-x", // Enable debugging mode
			"-c", // Run the following command
			fmt.Sprintf("tail -n+1 -F %s", path.Join(config.VarLogCephDir, opsLogFile)),
		},
		Image:           c.CephVersion.Image,
		ImagePullPolicy: GetContainerImagePullPolicy(c.CephVersion.ImagePullPolicy),
		VolumeMounts:    DaemonVolumeMounts(config.NewDatalessDaemonDataPathMap(ns, c.DataDirHostPath), "", c.DataDirHostPath),
		SecurityContext: PodSecurityContext(),
		Resources:       Resources,
		// We need a TTY for the bash job control (enabled by -m)
		TTY: true,
	}
}

// CreateExternalMetricsEndpoints creates external metric endpoint
func createExternalMetricsEndpoints(namespace string, monitoringSpec cephv1.MonitoringSpec, ownerInfo *k8sutil.OwnerInfo) (*v1.Endpoints, error) {
	labels := AppLabels("rook-ceph-mgr", namespace)

	endpoints := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ExternalMgrAppName,
			Namespace: namespace,
			Labels:    labels,
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: monitoringSpec.ExternalMgrEndpoints,
				Ports: []v1.EndpointPort{
					{
						Name:     ServiceExternalMetricName,
						Port:     int32(monitoringSpec.ExternalMgrPrometheusPort),
						Protocol: v1.ProtocolTCP,
					},
				},
			},
		},
	}

	err := ownerInfo.SetControllerReference(endpoints)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set owner reference to metric endpoints %q", endpoints.Name)
	}

	return endpoints, nil
}

func ConfigureExternalMetricsEndpoint(ctx *clusterd.Context, monitoringSpec cephv1.MonitoringSpec, clusterInfo *client.ClusterInfo, ownerInfo *k8sutil.OwnerInfo) error {
	if len(monitoringSpec.ExternalMgrEndpoints) == 0 {
		logger.Debug("no metric endpoint configured, doing nothing")
		return nil
	}

	// Get active mgr
	var activeMgrAddr string
	// We use mgr dump and not stat because we want the IP address
	mgrMap, err := client.CephMgrMap(ctx, clusterInfo)
	if err != nil {
		logger.Errorf("failed to get mgr map. %v", err)
	} else {
		activeMgrAddr = extractMgrIP(mgrMap.ActiveAddr)
	}
	logger.Debugf("active mgr addr %q", activeMgrAddr)

	// If the active manager is different than the one in the spec we override it
	// This happens when a standby manager becomes active
	if activeMgrAddr != monitoringSpec.ExternalMgrEndpoints[0].IP {
		monitoringSpec.ExternalMgrEndpoints[0].IP = activeMgrAddr
	}

	// Create external monitoring Endpoints
	endpoint, err := createExternalMetricsEndpoints(clusterInfo.Namespace, monitoringSpec, ownerInfo)
	if err != nil {
		return err
	}

	// Get the endpoint to see if anything needs to be updated
	currentEndpoints, err := ctx.Clientset.CoreV1().Endpoints(clusterInfo.Namespace).Get(clusterInfo.Context, endpoint.Name, metav1.GetOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to fetch endpoints")
	}

	// If endpoints are identical there is nothing to do
	// First check for nil pointers otherwise dereferencing a nil pointer will cause a panic
	if endpoint != nil && currentEndpoints != nil {
		if reflect.DeepEqual(*currentEndpoints, *endpoint) {
			return nil
		}
	}
	logger.Debugf("diff between current endpoint and newly generated one: %v \n", cmp.Diff(currentEndpoints, endpoint, cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })))

	_, err = k8sutil.CreateOrUpdateEndpoint(clusterInfo.Context, ctx.Clientset, clusterInfo.Namespace, endpoint)
	if err != nil {
		return errors.Wrap(err, "failed to create or update mgr endpoint")
	}

	return nil
}

func extractMgrIP(rawActiveAddr string) string {
	return strings.Split(rawActiveAddr, ":")[0]
}

func GetContainerImagePullPolicy(containerImagePullPolicy v1.PullPolicy) v1.PullPolicy {
	if containerImagePullPolicy == "" {
		return v1.PullIfNotPresent
	}

	return containerImagePullPolicy
}

// GenerateLivenessProbeTcpPort generates a liveness probe that makes sure a daemon has
// TCP a socket binded to specific port, and may create new connection.
func GenerateLivenessProbeTcpPort(port, failureThreshold int32) *v1.Probe {
	return &v1.Probe{
		ProbeHandler: v1.ProbeHandler{
			TCPSocket: &v1.TCPSocketAction{
				Port: intstr.IntOrString{IntVal: port},
			},
		},
		InitialDelaySeconds: livenessProbeInitialDelaySeconds,
		TimeoutSeconds:      livenessProbeTimeoutSeconds,
		FailureThreshold:    failureThreshold,
	}
}

// GenerateLivenessProbeViaRpcinfo creates a liveness probe using 'rpcinfo' shell
// command which checks that the local NFS daemon has TCP a socket binded to
// specific port, and it has valid reply to NULL RPC request.
func GenerateLivenessProbeViaRpcinfo(port uint16, failureThreshold int32) *v1.Probe {
	bb := make([]byte, 2)
	binary.BigEndian.PutUint16(bb, port) // port-num in network-order
	servAddr := fmt.Sprintf("127.0.0.1.%d.%d", bb[0], bb[1])
	return &v1.Probe{
		ProbeHandler: v1.ProbeHandler{
			Exec: &v1.ExecAction{
				Command: []string{"rpcinfo", "-a", servAddr, "-T", "tcp", "nfs", "4"},
			},
		},
		InitialDelaySeconds: livenessProbeInitialDelaySeconds,
		TimeoutSeconds:      livenessProbeTimeoutSeconds,
		FailureThreshold:    failureThreshold,
	}
}

func GetDaemonsToSkipReconcile(ctx context.Context, clusterd *clusterd.Context, namespace, daemonName, label string) (sets.Set[string], error) {
	listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s,%s", k8sutil.AppAttr, label, cephv1.SkipReconcileLabelKey)}

	deployments, err := clusterd.Clientset.AppsV1().Deployments(namespace).List(ctx, listOpts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to query %q to skip reconcile", daemonName)
	}

	result := sets.New[string]()
	for _, deployment := range deployments.Items {
		if daemonID, ok := deployment.Labels[daemonName]; ok {
			logger.Infof("found %s %q pod to skip reconcile", daemonID, daemonName)
			result.Insert(daemonID)
		}
	}

	return result, nil
}
