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
	"github.com/coreos/pkg/capnslog"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
)

const (
	// ConfigInitContainerName is the name which is given to the config initialization container
	// in all Ceph pods.
	ConfigInitContainerName = "config-init"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "ceph-spec")

// PodVolumes fills in the volumes parameter with the common list of Kubernetes volumes for use in Ceph pods.
func PodVolumes(dataDirHostPath string) []v1.Volume {
	dataDirSource := v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}
	if dataDirHostPath != "" {
		dataDirSource = v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: dataDirHostPath}}
	}
	return []v1.Volume{
		{Name: k8sutil.DataDirVolume, VolumeSource: dataDirSource},
		cephconfig.DefaultConfigVolume(),
		k8sutil.ConfigOverrideVolume(),
	}
}

// CephVolumeMounts returns the common list of Kubernetes volume mounts for Ceph containers.
func CephVolumeMounts() []v1.VolumeMount {
	return []v1.VolumeMount{
		{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
		cephconfig.DefaultConfigMount(),
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
