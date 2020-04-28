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
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *ReconcileCephRBDMirror) makeDeployment(daemonConfig *daemonConfig, rbdMirror *cephv1.CephRBDMirror) *apps.Deployment {
	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   daemonConfig.ResourceName,
			Labels: controller.PodLabels(AppName, rbdMirror.Namespace, config.RbdMirrorType, daemonConfig.DaemonID),
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
	// Replace default unreachable node toleration
	k8sutil.AddUnreachableNodeToleration(&podSpec.Spec)

	if r.cephClusterSpec.Network.IsHost() {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if r.cephClusterSpec.Network.IsMultus() {
		k8sutil.ApplyMultus(r.cephClusterSpec.Network.NetworkSpec, &podSpec.ObjectMeta)
	}
	rbdMirror.Spec.Placement.ApplyToPodSpec(&podSpec.Spec)

	replicas := int32(1)
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        daemonConfig.ResourceName,
			Namespace:   rbdMirror.Namespace,
			Annotations: rbdMirror.Spec.Annotations,
			Labels:      controller.PodLabels(AppName, rbdMirror.Namespace, config.RbdMirrorType, daemonConfig.DaemonID),
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

	return d
}

func (r *ReconcileCephRBDMirror) makeChownInitContainer(daemonConfig *daemonConfig, rbdMirror *cephv1.CephRBDMirror) v1.Container {
	return controller.ChownCephDataDirsInitContainer(
		*daemonConfig.DataPathMap,
		r.cephClusterSpec.CephVersion.Image,
		controller.DaemonVolumeMounts(daemonConfig.DataPathMap, daemonConfig.ResourceName),
		rbdMirror.Spec.Resources,
		mon.PodSecurityContext(),
	)
}

func (r *ReconcileCephRBDMirror) makeMirroringDaemonContainer(daemonConfig *daemonConfig, rbdMirror *cephv1.CephRBDMirror) v1.Container {
	container := v1.Container{
		Name: "rbd-mirror",
		Command: []string{
			"rbd-mirror",
		},
		Args: append(
			controller.DaemonFlags(r.clusterInfo, daemonConfig.DaemonID),
			"--foreground",
			"--name="+fullDaemonName(daemonConfig.DaemonID),
		),
		Image:           r.cephClusterSpec.CephVersion.Image,
		VolumeMounts:    controller.DaemonVolumeMounts(daemonConfig.DataPathMap, daemonConfig.ResourceName),
		Env:             controller.DaemonEnvVars(r.cephClusterSpec.CephVersion.Image),
		Resources:       rbdMirror.Spec.Resources,
		SecurityContext: mon.PodSecurityContext(),
		// TODO:
		// Not implemented at this point since the socket name is '/run/ceph/ceph-client.rbd-mirror.a.1.94362516231272.asok'
		// Also the command to run will be:
		// ceph --admin-daemon /run/ceph/ceph-client.rbd-mirror.a.1.94362516231272.asok rbd mirror status
		// LivenessProbe:   controller.GenerateLivenessProbeExecDaemon(config.RbdMirrorType, daemonConfig.DaemonID),
	}
	return container
}
