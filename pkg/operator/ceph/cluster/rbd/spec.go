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

// Package rbd for mirroring
package rbd

import (
	opmon "github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (m *Mirroring) makeDeployment(resourceName, daemonName string) *apps.Deployment {
	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   resourceName,
			Labels: opspec.PodLabels(appName, m.Namespace, "rbdmirror", daemonName),
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				m.makeConfigInitContainer(resourceName, daemonName),
			},
			Containers: []v1.Container{
				m.makeMirroringDaemonContainer(daemonName),
			},
			RestartPolicy: v1.RestartPolicyAlways,
			Volumes:       opspec.PodVolumes(""),
			HostNetwork:   m.hostNetwork,
		},
	}
	if m.hostNetwork {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	m.placement.ApplyToPodSpec(&podSpec.Spec)

	replicas := int32(1)
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: m.Namespace,
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: podSpec.Labels,
			},
			Template: podSpec,
			Replicas: &replicas,
		},
	}
	k8sutil.SetOwnerRef(m.context.Clientset, m.Namespace, &d.ObjectMeta, &m.ownerRef)
	return d
}

func (m *Mirroring) makeConfigInitContainer(resourceName, daemonName string) v1.Container {
	return v1.Container{
		Name: opspec.ConfigInitContainerName,
		Args: []string{
			"ceph",
			"config-init",
		},
		Image: k8sutil.MakeRookImage(m.rookVersion),
		Env: []v1.EnvVar{
			{Name: "ROOK_USERNAME", Value: fullDaemonName(daemonName)},
			{Name: "ROOK_KEYRING",
				ValueFrom: &v1.EnvVarSource{
					SecretKeyRef: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{Name: resourceName},
						Key:                  opspec.KeyringSecretKeyName,
					}}},
			k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
			k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
			opmon.EndpointEnvVar(),
			k8sutil.ConfigOverrideEnvVar(),
		},
		VolumeMounts: opspec.RookVolumeMounts(),
		Resources:    m.resources,
	}
}

func (m *Mirroring) makeMirroringDaemonContainer(daemonName string) v1.Container {
	container := v1.Container{
		Name: "rbdmirror",
		Command: []string{
			"rbd-mirror",
		},
		Args: []string{
			"--foreground",
			"-n", fullDaemonName(daemonName),
			"--conf", "/etc/ceph/ceph.conf",
			"--keyring", "/etc/ceph/keyring",
		},
		Image:        m.cephVersion.Image,
		VolumeMounts: opspec.CephVolumeMounts(),
		Env:          k8sutil.ClusterDaemonEnvVars(),
		Resources:    m.resources,
	}
	return container
}
