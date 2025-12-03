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

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/util/log"
	v1 "k8s.io/api/core/v1"
)

func (r *ReconcileCephNFS) addSecurityConfigsToPod(nfs *cephv1.CephNFS, pod *v1.PodSpec) error {
	nsName := controller.NsName(nfs.Namespace, nfs.Name)

	sec := nfs.Spec.Security
	if sec == nil {
		return nil
	}

	if sec.SSSD != nil {
		log.NamedDebug(nsName, logger, "configuring system security services daemon (SSSD) for CephNFS")
		addSSSDConfigsToPod(r, nfs, pod)
	}

	if sec.Kerberos != nil {
		log.NamedDebug(nsName, logger, "configuring Kerberos for CephNFS")
		addKerberosConfigsToPod(r, nfs, pod)
	}

	return nil
}

func addSSSDConfigsToPod(r *ReconcileCephNFS, nfs *cephv1.CephNFS, pod *v1.PodSpec) {
	nsName := controller.NsName(nfs.Namespace, nfs.Name)

	// generate /etc/nsswitch.conf file for the nfs-ganesha pod
	nssCfgInitContainer, nssCfgVol, nssCfgMount := generateSssdNsswitchConfResources(r, nfs)

	pod.InitContainers = append(pod.InitContainers, *nssCfgInitContainer)
	pod.Volumes = append(pod.Volumes, *nssCfgVol)
	// assume the first container is the NFS-Ganesha container
	pod.Containers[0].VolumeMounts = append(pod.Containers[0].VolumeMounts, *nssCfgMount)

	sidecarCfg := nfs.Spec.Security.SSSD.Sidecar
	if sidecarCfg != nil {
		log.NamedDebug(nsName, logger, "configuring SSSD sidecar for CephNFS")
		init, sidecar, vols, mounts := generateSssdSidecarResources(nfs, sidecarCfg)

		pod.InitContainers = append(pod.InitContainers, *init)
		pod.Containers = append(pod.Containers, *sidecar)
		pod.Volumes = append(pod.Volumes, vols...)
		// assume the first container is the NFS-Ganesha container
		pod.Containers[0].VolumeMounts = append(pod.Containers[0].VolumeMounts, mounts...)
	}
}

func addKerberosConfigsToPod(r *ReconcileCephNFS, nfs *cephv1.CephNFS, pod *v1.PodSpec) {
	init, volume, ganeshaMounts := generateKrbConfResources(r, nfs)

	pod.InitContainers = append(pod.InitContainers, *init)
	pod.Volumes = append(pod.Volumes, *volume)
	// assume the first container is the NFS-Ganesha container
	for _, m := range ganeshaMounts {
		pod.Containers[0].VolumeMounts = append(pod.Containers[0].VolumeMounts, *m)
	}

	configVolSrc := nfs.Spec.Security.Kerberos.ConfigFiles.VolumeSource
	if configVolSrc != nil {
		vol, mnt := kerberosConfigFilesVolAndMount(*configVolSrc)

		pod.Volumes = append(pod.Volumes, vol)
		pod.Containers[0].VolumeMounts = append(pod.Containers[0].VolumeMounts, mnt)
	}

	keytabVolSrc := nfs.Spec.Security.Kerberos.KeytabFile.VolumeSource
	if keytabVolSrc != nil {
		vol, mnt := keytabVolAndMount(*keytabVolSrc)

		pod.Volumes = append(pod.Volumes, vol)
		pod.Containers[0].VolumeMounts = append(pod.Containers[0].VolumeMounts, mnt)
	}
}

func generateSssdSidecarResources(nfs *cephv1.CephNFS, sidecarCfg *cephv1.SSSDSidecar) (
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
		vol, mount := sssdConfigVolAndMount(*volSource.ToKubernetesVolumeSource())

		volumes = append(volumes, vol)
		sssdMounts = append(sssdMounts, mount)
	}

	genericVols, genericMounts := sidecarCfg.AdditionalFiles.GenerateVolumesAndMounts("/etc/sssd/rook-additional/")
	volumes = append(volumes, genericVols...)
	sssdMounts = append(sssdMounts, genericMounts...)

	// The volumes for krb5.conf and krb5.keytab are created separately
	// for the nfs-ganesha container. We reuse it here.
	if nfs.Spec.Security.Kerberos != nil {
		krb5ConfVolName := "krb5-conf-d"
		generatedKrbConfVolName := "generated-krb5-conf"

		krb5ConfD := v1.VolumeMount{
			Name:      krb5ConfVolName,
			MountPath: "/etc/krb5.conf.rook/",
		}
		krbConfMount := v1.VolumeMount{
			Name:      generatedKrbConfVolName,
			MountPath: "/etc/krb5.conf",
			SubPath:   "krb5.conf",
		}
		sssdMounts = append(sssdMounts, krb5ConfD, krbConfMount)

		if nfs.Spec.Security.Kerberos.KeytabFile.VolumeSource != nil {
			volName := "krb5-keytab"
			keytabMount := v1.VolumeMount{
				Name:      volName,
				MountPath: "/etc/krb5.keytab",
				SubPath:   "krb5.keytab",
			}
			sssdMounts = append(sssdMounts, keytabMount)
		}
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

func generateKrbConfResources(r *ReconcileCephNFS, nfs *cephv1.CephNFS) (
	init *v1.Container,
	volume *v1.Volume, // add these volumes to the pod
	ganeshaMounts []*v1.VolumeMount, // add these volume mounts to the nfs-ganesha container
) {
	generatedKrbConfVolName := "generated-krb5-conf"
	kerberosDomainName := nfs.Spec.Security.Kerberos.DomainName

	volume = &v1.Volume{
		Name: generatedKrbConfVolName,
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}
	krbConfMount := &v1.VolumeMount{
		Name:      generatedKrbConfVolName,
		MountPath: "/etc/krb5.conf",
		SubPath:   "krb5.conf",
	}
	ganeshaMounts = append(ganeshaMounts, krbConfMount)

	domainNameCommand := ""
	domainName := nfs.Spec.Security.Kerberos.DomainName
	if domainName != "" {
		domainNameCommand = `
cat << EOF > /tmp/etc/idmapd.conf
[General]
Domain = ` + kerberosDomainName + `
EOF
cat /etc/idmapd.conf`
		idmapdConfMount := &v1.VolumeMount{
			Name:      generatedKrbConfVolName,
			MountPath: "/etc/idmapd.conf",
			SubPath:   "idmapd.conf",
		}
		ganeshaMounts = append(ganeshaMounts, idmapdConfMount)
	}

	// the init container is needed to copy the starting content from the /var/lib/sss/pipes
	// directory into the shared sockets dir so that SSSD has the content it needs to start up
	init = &v1.Container{
		Name: "generate-krb5-conf",
		Command: []string{
			"bash", "-c",
			`set -ex
cat << EOF > /tmp/etc/krb5.conf
[logging]
default = STDERR

includedir /etc/krb5.conf.rook/
EOF
cat /tmp/etc/krb5.conf
` + domainNameCommand,
		},
		VolumeMounts: []v1.VolumeMount{
			{Name: generatedKrbConfVolName, MountPath: "/tmp/etc"},
		},
		Image:     r.cephClusterSpec.CephVersion.Image,
		Resources: nfs.Spec.Server.Resources,
	}

	return init, volume, ganeshaMounts
}

func sssdConfigVolAndMount(volSource v1.VolumeSource) (v1.Volume, v1.VolumeMount) {
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
passwd: files sss
group: files sss
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

func kerberosConfigFilesVolAndMount(volSource cephv1.ConfigFileVolumeSource) (v1.Volume, v1.VolumeMount) {
	volName := "krb5-conf-d"
	vol := v1.Volume{
		Name:         volName,
		VolumeSource: *volSource.ToKubernetesVolumeSource(),
	}
	mount := v1.VolumeMount{
		Name:      volName,
		MountPath: "/etc/krb5.conf.rook/",
	}

	return vol, mount
}

func keytabVolAndMount(volSource cephv1.ConfigFileVolumeSource) (v1.Volume, v1.VolumeMount) {
	volName := "krb5-keytab"
	vol := v1.Volume{
		Name:         volName,
		VolumeSource: *volSource.ToKubernetesVolumeSource(),
	}
	mount := v1.VolumeMount{
		Name:      volName,
		MountPath: "/etc/krb5.keytab",
		SubPath:   "krb5.keytab",
	}

	return vol, mount
}
