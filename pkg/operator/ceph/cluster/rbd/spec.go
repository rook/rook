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
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (m *Mirroring) makeDeployment(daemonConfig *daemonConfig) *apps.Deployment {
	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   daemonConfig.ResourceName,
			Labels: opspec.PodLabels(AppName, m.Namespace, string(config.RbdMirrorType), daemonConfig.DaemonID),
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				m.makeChownInitContainer(daemonConfig),
			},
			Containers: []v1.Container{
				m.makeMirroringDaemonContainer(daemonConfig),
			},
			RestartPolicy:     v1.RestartPolicyAlways,
			Volumes:           opspec.DaemonVolumes(daemonConfig.DataPathMap, daemonConfig.ResourceName),
			HostNetwork:       m.Network.IsHost(),
			PriorityClassName: m.priorityClassName,
		},
	}
	// Replace default unreachable node toleration
	k8sutil.AddUnreachableNodeToleration(&podSpec.Spec)

	if m.Network.IsHost() {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	m.placement.ApplyToPodSpec(&podSpec.Spec)

	replicas := int32(1)
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        daemonConfig.ResourceName,
			Namespace:   m.Namespace,
			Annotations: m.annotations,
			Labels:      opspec.PodLabels(AppName, m.Namespace, string(config.RbdMirrorType), daemonConfig.DaemonID),
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
	opspec.AddCephVersionLabelToDeployment(m.ClusterInfo.CephVersion, d)
	k8sutil.SetOwnerRef(&d.ObjectMeta, &m.ownerRef)
	return d
}

func (m *Mirroring) makeChownInitContainer(daemonConfig *daemonConfig) v1.Container {
	return opspec.ChownCephDataDirsInitContainer(
		*daemonConfig.DataPathMap,
		m.cephVersion.Image,
		opspec.DaemonVolumeMounts(daemonConfig.DataPathMap, daemonConfig.ResourceName),
		m.resources,
		mon.PodSecurityContext(),
	)
}

func (m *Mirroring) makeMirroringDaemonContainer(daemonConfig *daemonConfig) v1.Container {
	container := v1.Container{
		Name: "rbd-mirror",
		Command: []string{
			"rbd-mirror",
		},
		Args: append(
			opspec.DaemonFlags(m.ClusterInfo, daemonConfig.DaemonID),
			"--foreground",
			"--name="+fullDaemonName(daemonConfig.DaemonID),
		),
		Image:           m.cephVersion.Image,
		VolumeMounts:    opspec.DaemonVolumeMounts(daemonConfig.DataPathMap, daemonConfig.ResourceName),
		Env:             opspec.DaemonEnvVars(m.cephVersion.Image),
		Resources:       m.resources,
		SecurityContext: mon.PodSecurityContext(),
	}
	return container
}
