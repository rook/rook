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
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/display"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ConfigInitContainerName is the name which is given to the config initialization container
	// in all Ceph pods.
	ConfigInitContainerName                 = "config-init"
	logVolumeName                           = "rook-ceph-log"
	volumeMountSubPath                      = "data"
	crashVolumeName                         = "rook-ceph-crash"
	daemonSocketDir                         = "/run/ceph"
	livenessProbeInitialDelaySeconds  int32 = 10
	startupProbeFailuresDaemonDefault int32 = 6 // multiply by 10 = effective startup timeout
	startupProbeFailuresDaemonOSD     int32 = 9 // multiply by 10 = effective startup timeout
	logCollector                            = "log-collector"
	DaemonIDLabel                           = "ceph_daemon_id"
	daemonTypeLabel                         = "ceph_daemon_type"
	ExternalMgrAppName                      = "rook-ceph-mgr-external"
	ServiceExternalMetricName               = "http-external-metrics"
)

type daemonConfig struct {
	daemonType string
	daemonID   string
}

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "ceph-spec")

var (
	cronLogRotate = `
CEPH_CLIENT_ID=%s
PERIODICITY=%s
LOG_ROTATE_CEPH_FILE=/etc/logrotate.d/ceph

if [ -z "$PERIODICITY" ]; then
	PERIODICITY=24h
fi

# edit the logrotate file to only rotate a specific daemon log
# otherwise we will logrotate log files without reloading certain daemons
# this might happen when multiple daemons run on the same machine
sed -i "s|*.log|$CEPH_CLIENT_ID.log|" "$LOG_ROTATE_CEPH_FILE"

while true; do
	sleep "$PERIODICITY"
	echo "starting log rotation"
	logrotate --verbose --force "$LOG_ROTATE_CEPH_FILE"
	echo "I am going to sleep now, see you in $PERIODICITY"
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
	mode := int32(0444)
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
		configVolume, _ = ConfGeneratedInPodVolumeAndMount()
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
		_, configMount = ConfGeneratedInPodVolumeAndMount()
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
	return CephVolumeMounts(dataPaths, confGeneratedInPod)

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
func DaemonFlags(cluster *client.ClusterInfo, spec *cephv1.ClusterSpec, daemonID string) []string {
	flags := append(
		config.DefaultFlags(cluster.FSID, keyring.VolumeMount().KeyringFilePath()),
		config.NewFlag("id", daemonID),
		// Ceph daemons in Rook will run as 'ceph' instead of 'root'
		// If we run on a version of Ceph does not these flags it will simply ignore them
		//run ceph daemon process under the 'ceph' user
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

	// As of Pacific, Ceph supports dual-stack, so setting IPv6 family without disabling IPv4 binding actually enables dual-stack
	// This is likely not user's intent, so on Pacific let's make sure to disable IPv4 when IPv6 is selected
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
		if cluster.CephVersion.IsAtLeastPacific() {
			args = append(args, config.NewFlag("ms-bind-ipv4", "true"))
			args = append(args, config.NewFlag("ms-bind-ipv6", "true"))
		} else {
			logger.Info("dual-stack is only supported on ceph pacific")
			// Still acknowledge IPv6, nothing to do for IPv4 since it will always be "on"
			if spec.Network.IPFamily == cephv1.IPv6 {
				args = append(args, config.NewFlag("ms-bind-ipv6", "true"))
			}
		}
	}

	return args
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
		if uint64(podMemoryLimit.Value()) < display.MbTob(cephPodMinimumMemory) {
			// allow the configuration if less than the min, but print a warning
			logger.Warningf("running the %q daemon(s) with %dMB of ram, but at least %dMB is recommended", name, display.BToMb(uint64(podMemoryLimit.Value())), cephPodMinimumMemory)
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

// GenerateMinimalCephConfInitContainer returns an init container that will generate the most basic
// Ceph config for connecting non-Ceph daemons to a Ceph cluster (e.g., nfs-ganesha). Effectively
// what this means is that it generates '/etc/ceph/ceph.conf' with 'mon_host' populated and a
// keyring path associated with the user given. 'mon_host' is determined by the 'ROOK_CEPH_MON_HOST'
// env var present in other Ceph daemon pods, and the keyring is expected to be mounted into the
// container with a Kubernetes pod volume+mount.
func GenerateMinimalCephConfInitContainer(
	username, keyringPath string,
	containerImage string,
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

	return &v1.Probe{
		ProbeHandler: v1.ProbeHandler{
			Exec: &v1.ExecAction{
				// Run with env -i to clean env variables in the exec context
				// This avoids conflict with the CEPH_ARGS env
				//
				// Example:
				// env -i sh -c "ceph --admin-daemon /run/ceph/ceph-osd.0.asok status"
				Command: []string{
					"env",
					"-i",
					"sh",
					"-c",
					fmt.Sprintf("ceph --admin-daemon %s %s", confDaemon.buildSocketPath(), confDaemon.buildAdminSocketCommand()),
				},
			},
		},
		InitialDelaySeconds: livenessProbeInitialDelaySeconds,
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
	}
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

	return sec
}

// LogCollectorContainer runs a cron job to rotate logs
func LogCollectorContainer(daemonID, ns string, c cephv1.ClusterSpec) *v1.Container {
	return &v1.Container{
		Name: logCollector,
		Command: []string{
			"/bin/bash",
			"-x", // Print commands and their arguments as they are executed
			"-e", // Exit immediately if a command exits with a non-zero status.
			"-m", // Terminal job control, allows job to be terminated by SIGTERM
			"-c", // Command to run
			fmt.Sprintf(cronLogRotate, daemonID, c.LogCollector.Periodicity),
		},
		Image:           c.CephVersion.Image,
		VolumeMounts:    DaemonVolumeMounts(config.NewDatalessDaemonDataPathMap(ns, c.DataDirHostPath), ""),
		SecurityContext: PodSecurityContext(),
		Resources:       cephv1.GetLogCollectorResources(c.Resources),
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
