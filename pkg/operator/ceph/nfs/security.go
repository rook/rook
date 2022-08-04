/*
Copyright 2022 The Rook Authors. All rights reserved.

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

package nfs

import (
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcileCephNFS) addSecurityConfigsToPod(nfs *cephv1.CephNFS, pod *v1.PodSpec) error {
	nsName := types.NamespacedName{Namespace: nfs.Namespace, Name: nfs.Name}

	sec := nfs.Spec.Security
	if sec == nil {
		return nil
	}

	if err := sec.Validate(); err != nil {
		return errors.Wrapf(err, "failed to set up security for CephNFS %q", nsName)
	}

	if sec.SSSD != nil {
		logger.Debugf("configuring system security services daemon (SSSD) for CephNFS %q", nsName)
		addSSSDConfigsToPod(r, nfs, pod)
	}

	return nil
}

func addSSSDConfigsToPod(r *ReconcileCephNFS, nfs *cephv1.CephNFS, pod *v1.PodSpec) {
	nsName := types.NamespacedName{Namespace: nfs.Namespace, Name: nfs.Name}

	// generate /etc/nsswitch.conf file for the nfs-ganesha pod
	nssCfgInitContainer, nssCfgVol, nssCfgMount := generateSssdNsswitchConfResources(r, nfs)

	pod.InitContainers = append(pod.InitContainers, *nssCfgInitContainer)
	pod.Volumes = append(pod.Volumes, *nssCfgVol)
	// assume the first container is the NFS-Ganesha container
	pod.Containers[0].VolumeMounts = append(pod.Containers[0].VolumeMounts, *nssCfgMount)

	sidecarCfg := nfs.Spec.Security.SSSD.Sidecar
	if sidecarCfg != nil {
		logger.Debugf("configuring SSSD sidecar for CephNFS %q", nsName)
		init, sidecar, vols, mounts := generateSssdSidecarResources(sidecarCfg)

		pod.InitContainers = append(pod.InitContainers, *init)
		pod.Containers = append(pod.Containers, *sidecar)
		pod.Volumes = append(pod.Volumes, vols...)
		// assume the first container is the NFS-Ganesha container
		pod.Containers[0].VolumeMounts = append(pod.Containers[0].VolumeMounts, mounts...)
	}
}

func generateSssdSidecarResources(sidecarCfg *cephv1.SSSDSidecar) (
	init *v1.Container,
	sidecar *v1.Container,
	volumes []v1.Volume, // add these volumes to the pod
	ganeshaMounts []v1.VolumeMount, // add these volume mounts to the nfs-ganesha container
) {
	socketVolName := "sssd-sockets"
	mmapCacheVolName := "sssd-mmap-cache"

	socketVol := v1.Volume{
		Name: socketVolName,
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}
	socketMount := v1.VolumeMount{
		Name:      socketVolName,
		MountPath: "/var/lib/sss/pipes",
	}

	mmapCacheVol := v1.Volume{
		Name: mmapCacheVolName,
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}
	mmapCacheMount := v1.VolumeMount{
		Name:      mmapCacheVolName,
		MountPath: "/var/lib/sss/mc",
	}

	volumes = []v1.Volume{socketVol, mmapCacheVol}

	// conf file mount not needed in the ganesha pod, only the SSSD sidecar
	ganeshaMounts = []v1.VolumeMount{socketMount, mmapCacheMount}

	sssdMounts := []v1.VolumeMount{socketMount, mmapCacheMount}

	volSource := sidecarCfg.SSSDConfigFile.VolumeSource
	if volSource != nil {
		vol, mount := configVolAndMount(*volSource)

		volumes = append(volumes, vol)
		sssdMounts = append(sssdMounts, mount)
	}

	// the init container is needed to copy the starting content from the /var/lib/sss/pipes
	// directory into the shared sockets dir so that SSSD has the content it needs to start up
	init = &v1.Container{
		Name: "copy-sssd-sockets",
		Command: []string{
			"bash", "-c",
			`set -ex
cp --archive --verbose /var/lib/sss/pipes/* /tmp/var/lib/sss/pipes/.
ls --all --recursive /tmp/var/lib/sss/pipes`,
		},
		VolumeMounts: []v1.VolumeMount{
			{Name: socketVolName, MountPath: "/tmp/var/lib/sss/pipes"},
		},
		Image:     sidecarCfg.Image,
		Resources: sidecarCfg.Resources,
	}

	sidecar = &v1.Container{
		Name: "sssd",
		Command: []string{
			"sssd",
		},
		Args: []string{
			"--interactive",
			"--logger=stderr",
		},
		VolumeMounts: sssdMounts,
		Image:        sidecarCfg.Image,
		Resources:    sidecarCfg.Resources,
	}

	if sidecarCfg.DebugLevel > 0 {
		sidecar.Args = append(sidecar.Args, fmt.Sprintf("--debug-level=%d", sidecarCfg.DebugLevel))
	}

	return init, sidecar, volumes, ganeshaMounts
}

func configVolAndMount(volSource v1.VolumeSource) (v1.Volume, v1.VolumeMount) {
	volName := "sssd-conf"
	vol := v1.Volume{
		Name:         volName,
		VolumeSource: volSource,
	}
	mount := v1.VolumeMount{
		Name:      volName,
		MountPath: "/etc/sssd/sssd.conf",
		SubPath:   "sssd.conf",
	}

	return vol, mount
}

func generateSssdNsswitchConfResources(r *ReconcileCephNFS, nfs *cephv1.CephNFS) (*v1.Container, *v1.Volume, *v1.VolumeMount) {
	volName := "nsswitch-conf"

	podVol := &v1.Volume{
		Name: volName,
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}

	nfsGaneshaContainerMount := &v1.VolumeMount{
		Name:      volName,
		MountPath: "/etc/nsswitch.conf",
		SubPath:   "nsswitch.conf",
	}

	// what happens here is that an empty dir is mounted to /tmp/etc, and this init container
	// creates the nsswitch.conf file in it. Once the file is created, subsequent containers can
	// mount the nsswitch.conf file to /etc/nsswitch.conf using 'subPath'
	init := &v1.Container{
		Name: "generate-nsswitch-conf",
		Command: []string{
			"bash", "-c",
			`set -ex
cat << EOF > /tmp/etc/nsswitch.conf
passwd: sss
group: sss
netgroup: sss
EOF
chmod 444 /tmp/etc/nsswitch.conf
cat /tmp/etc/nsswitch.conf`,
		},
		VolumeMounts: []v1.VolumeMount{
			{Name: volName, MountPath: "/tmp/etc"},
		},

		// use CephCluster image and NFS server resources here because this container should be used
		// to configure /etc/nsswitch.conf even if the SSSD sidecar isn't configured
		Image:     r.cephClusterSpec.CephVersion.Image,
		Resources: nfs.Spec.Server.Resources,
	}

	return init, podVol, nfsGaneshaContainerMount
}
