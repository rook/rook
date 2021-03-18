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

package rbd

import (
	"fmt"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *ReconcileCephRBDMirror) makeDeployment(daemonConfig *daemonConfig, rbdMirror *cephv1.CephRBDMirror) (*apps.Deployment, error) {
	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   daemonConfig.ResourceName,
			Labels: controller.CephDaemonAppLabels(AppName, rbdMirror.Namespace, config.RbdMirrorType, daemonConfig.DaemonID, true),
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				r.makeChownInitContainer(daemonConfig, rbdMirror),
			},
			Containers: []v1.Container{
				r.makeMirroringDaemonContainer(daemonConfig, rbdMirror),
			},
			RestartPolicy:     v1.RestartPolicyAlways,
			Volumes:           controller.DaemonVolumes(daemonConfig.DataPathMap, daemonConfig.ResourceName),
			HostNetwork:       r.cephClusterSpec.Network.IsHost(),
			PriorityClassName: rbdMirror.Spec.PriorityClassName,
		},
	}

	// If the log collector is enabled we add the side-car container
	if r.cephClusterSpec.LogCollector.Enabled {
		shareProcessNamespace := true
		podSpec.Spec.ShareProcessNamespace = &shareProcessNamespace
		podSpec.Spec.Containers = append(podSpec.Spec.Containers, *controller.LogCollectorContainer(fmt.Sprintf("ceph-client.rbd-mirror.%s", daemonConfig.DaemonID), r.clusterInfo.Namespace, *r.cephClusterSpec))
	}

	// Replace default unreachable node toleration
	k8sutil.AddUnreachableNodeToleration(&podSpec.Spec)
	rbdMirror.Spec.Annotations.ApplyToObjectMeta(&podSpec.ObjectMeta)
	rbdMirror.Spec.Labels.ApplyToObjectMeta(&podSpec.ObjectMeta)

	if r.cephClusterSpec.Network.IsHost() {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if r.cephClusterSpec.Network.IsMultus() {
		if err := k8sutil.ApplyMultus(r.cephClusterSpec.Network.NetworkSpec, &podSpec.ObjectMeta); err != nil {
			return nil, err
		}
	}
	rbdMirror.Spec.Placement.ApplyToPodSpec(&podSpec.Spec)

	// If the rbd mirror has a peer we must add the relevant ceph config file and key to connect to it
	// Both cm and secret have been created already, so it's fine to just reference them
	if rbdMirror.Spec.Peers.HasPeers() {
		// Add the config map and secret
		// We only use the first peer in the list because peers are all the same, just the pool differs
		firstPeer := rbdMirror.Spec.Peers.SecretNames[0]
		volProjection := peerConfigMapAndSecretVolumeAndMount(r.peers[firstPeer].info.Peers[0].SiteName, r.peers[firstPeer].info.Peers[0].ClientName, r.peers[firstPeer].info.Peers[0].UUID)
		podSpec.Spec.Volumes[0].VolumeSource.Projected.Sources = append(podSpec.Spec.Volumes[0].VolumeSource.Projected.Sources, volProjection...)
	}

	replicas := int32(rbdMirror.Spec.Count)
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        daemonConfig.ResourceName,
			Namespace:   rbdMirror.Namespace,
			Annotations: rbdMirror.Spec.Annotations,
			Labels:      controller.CephDaemonAppLabels(AppName, rbdMirror.Namespace, config.RbdMirrorType, daemonConfig.DaemonID, true),
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: podSpec.Labels,
			},
			Template: podSpec,
			Replicas: &replicas,
		},
	}
	k8sutil.AddRookVersionLabelToDeployment(d)
	controller.AddCephVersionLabelToDeployment(r.clusterInfo.CephVersion, d)
	rbdMirror.Spec.Annotations.ApplyToObjectMeta(&d.ObjectMeta)
	rbdMirror.Spec.Labels.ApplyToObjectMeta(&d.ObjectMeta)

	return d, nil
}

func (r *ReconcileCephRBDMirror) makeChownInitContainer(daemonConfig *daemonConfig, rbdMirror *cephv1.CephRBDMirror) v1.Container {
	return controller.ChownCephDataDirsInitContainer(
		*daemonConfig.DataPathMap,
		r.cephClusterSpec.CephVersion.Image,
		controller.DaemonVolumeMounts(daemonConfig.DataPathMap, daemonConfig.ResourceName),
		rbdMirror.Spec.Resources,
		controller.PodSecurityContext(),
	)
}

func (r *ReconcileCephRBDMirror) makeMirroringDaemonContainer(daemonConfig *daemonConfig, rbdMirror *cephv1.CephRBDMirror) v1.Container {
	container := v1.Container{
		Name: "rbd-mirror",
		Command: []string{
			"rbd-mirror",
		},
		Args: append(
			controller.DaemonFlags(r.clusterInfo, r.cephClusterSpec, daemonConfig.DaemonID),
			"--foreground",
			"--name="+fullDaemonName(daemonConfig.DaemonID),
		),
		Image:           r.cephClusterSpec.CephVersion.Image,
		VolumeMounts:    controller.DaemonVolumeMounts(daemonConfig.DataPathMap, daemonConfig.ResourceName),
		Env:             controller.DaemonEnvVars(r.cephClusterSpec.CephVersion.Image),
		Resources:       rbdMirror.Spec.Resources,
		SecurityContext: controller.PodSecurityContext(),
		WorkingDir:      config.VarLogCephDir,
		// TODO:
		// Not implemented at this point since the socket name is '/run/ceph/ceph-client.rbd-mirror.a.1.94362516231272.asok'
		// Also the command to run will be:
		// ceph --admin-daemon /run/ceph/ceph-client.rbd-mirror.a.1.94362516231272.asok rbd mirror status
		// LivenessProbe:   controller.GenerateLivenessProbeExecDaemon(config.RbdMirrorType, daemonConfig.DaemonID),
	}

	return container
}

// return the volume and matching volume mount for mounting the config map into /etc/ceph/
func peerConfigMapAndSecretVolumeAndMount(siteName, clientName, peerSiteUUID string) []v1.VolumeProjection {
	// Projection list
	secretAndConfigMapVolumeProjections := []v1.VolumeProjection{}

	// CM
	configMapName := generatePeerCephConfigFileConfigMapName(peerSiteUUID) // configmap name and name of volume
	configMapFile := fmt.Sprintf("%s.conf", siteName)
	mode := int32(0444)
	projectionConfigMap := &v1.ConfigMapProjection{Items: []v1.KeyToPath{{Key: peerCephConfigKey, Path: configMapFile, Mode: &mode}}}
	projectionConfigMap.Name = configMapName

	// Secret
	secretName := generatePeerKeyringSecretName(peerSiteUUID) // secret name and name of volume
	secretFile := fmt.Sprintf("%s.%s.keyring", siteName, clientName)
	projectionSecret := &v1.SecretProjection{Items: []v1.KeyToPath{{Key: peerCephKeyringKey, Path: secretFile, Mode: &mode}}}
	projectionSecret.Name = secretName

	configMapProjection := v1.VolumeProjection{
		ConfigMap: projectionConfigMap,
	}
	secretAndConfigMapVolumeProjections = append(secretAndConfigMapVolumeProjections, configMapProjection)
	secretProjection := v1.VolumeProjection{
		Secret: projectionSecret,
	}
	secretAndConfigMapVolumeProjections = append(secretAndConfigMapVolumeProjections, secretProjection)

	return secretAndConfigMapVolumeProjections
}
