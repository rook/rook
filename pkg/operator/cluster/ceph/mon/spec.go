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

// Package mon for the Ceph monitors.
package mon

import (
	"fmt"

	"github.com/rook/rook/pkg/daemon/ceph/mon"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/apps/v1beta1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ClusterNameEnvVar is the cluster name environment var
func ClusterNameEnvVar(name string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_CLUSTER_NAME", Value: name}
}

// EndpointsEnvVar is the mon endpoint environment var
func EndpointsEnvVar() v1.EnvVar {
	ref := &v1.ConfigMapKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: EndpointConfigMapName}, Key: MonEndpointKey}
	return v1.EnvVar{Name: "ROOK_MON_ENDPOINTS", ValueFrom: &v1.EnvVarSource{ConfigMapKeyRef: ref}}
}

// EndpointEnvVar is the mon dns name endpoint environment var
func EndpointEnvVar() v1.EnvVar {
	ref := &v1.ConfigMapKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: EndpointConfigMapName}, Key: EndpointKey}
	return v1.EnvVar{Name: "ROOK_MON_ENDPOINTS", ValueFrom: &v1.EnvVarSource{ConfigMapKeyRef: ref}}
}

// SecretEnvVar is the mon secret environment var
func SecretEnvVar() v1.EnvVar {
	ref := &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: appName}, Key: monSecretName}
	return v1.EnvVar{Name: "ROOK_MON_SECRET", ValueFrom: &v1.EnvVarSource{SecretKeyRef: ref}}
}

// AdminSecretEnvVar is the admin secret environment var
func AdminSecretEnvVar() v1.EnvVar {
	ref := &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: appName}, Key: adminSecretName}
	return v1.EnvVar{Name: "ROOK_ADMIN_SECRET", ValueFrom: &v1.EnvVarSource{SecretKeyRef: ref}}
}

// PodNameEnvVar sets the Pod name as the name
func PodNameEnvVar() v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_NAME", ValueFrom: &v1.EnvVarSource{
		FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.name"},
	}}
}

func (c *Cluster) getLabels() map[string]string {
	return map[string]string{
		k8sutil.AppAttr: appName,
		monClusterAttr:  c.Namespace,
	}
}

func (c *Cluster) makeStatefulSet(replicas int32) *v1beta1.StatefulSet {
	pod := c.makeMonPod()
	sts := &v1beta1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            appName,
			Namespace:       c.Namespace,
			OwnerReferences: []metav1.OwnerReference{c.ownerRef},
		},
		Spec: v1beta1.StatefulSetSpec{
			UpdateStrategy: v1beta1.StatefulSetUpdateStrategy{
				Type: v1beta1.RollingUpdateStatefulSetStrategyType,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: pod.ObjectMeta,
				Spec:       pod.Spec,
			},
			Replicas:    &replicas,
			ServiceName: appName,
		},
	}

	return sts
}

func (c *Cluster) makeMonPod() *v1.Pod {
	dataDirSource := v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}
	if c.dataDirHostPath != "" {
		dataDirSource = v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: c.dataDirHostPath}}
	}

	labels := c.getLabels()

	container := c.monContainer(c.clusterInfo.FSID)
	podSpec := v1.PodSpec{
		Containers:    []v1.Container{container},
		RestartPolicy: v1.RestartPolicyAlways,
		Volumes: []v1.Volume{
			{Name: k8sutil.DataDirVolume, VolumeSource: dataDirSource},
			k8sutil.ConfigOverrideVolume(),
		},
		HostNetwork: c.HostNetwork,
	}
	if c.HostNetwork {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}

	// The user needs to set an own PodAntiAffinity if he wants to make sure the mons
	// are "correctly" distributed
	c.placement.ApplyToPodSpec(&podSpec)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        appName,
			Namespace:   c.Namespace,
			Labels:      labels,
			Annotations: map[string]string{},
		},
		Spec: podSpec,
	}

	return pod
}

func (c *Cluster) monContainer(fsid string) v1.Container {
	return v1.Container{
		Args: []string{
			"mon",
			fmt.Sprintf("--config-dir=%s", k8sutil.DataDir),
			fmt.Sprintf("--fsid=%s", fsid),
			fmt.Sprintf("--port=%d", mon.DefaultPort),
		},
		Name:  appName,
		Image: k8sutil.MakeRookImage(c.Version),
		Ports: []v1.ContainerPort{
			{
				Name:          "client",
				ContainerPort: mon.DefaultPort,
				Protocol:      v1.ProtocolTCP,
			},
		},
		VolumeMounts: []v1.VolumeMount{
			{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
			k8sutil.ConfigOverrideMount(),
		},
		Env: []v1.EnvVar{
			PodNameEnvVar(),
			k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
			k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
			ClusterNameEnvVar(c.Namespace),
			EndpointsEnvVar(),
			SecretEnvVar(),
			AdminSecretEnvVar(),
			k8sutil.ConfigOverrideEnvVar(),
		},
		Resources: c.resources,
		// TCP probes to check if the mon is alive. The quorum is checked outside the mons.
		ReadinessProbe: &v1.Probe{
			Handler: v1.Handler{
				TCPSocket: &v1.TCPSocketAction{
					Port: intstr.IntOrString{IntVal: int32(mon.DefaultPort)},
				},
			},
			InitialDelaySeconds: int32(3),
			FailureThreshold:    5,
			PeriodSeconds:       3,
			SuccessThreshold:    1,
			TimeoutSeconds:      2,
		},
		LivenessProbe: &v1.Probe{
			Handler: v1.Handler{
				TCPSocket: &v1.TCPSocketAction{
					Port: intstr.IntOrString{IntVal: int32(mon.DefaultPort)},
				},
			},
			InitialDelaySeconds: int32(10),
			FailureThreshold:    5,
			PeriodSeconds:       3,
			SuccessThreshold:    1,
			TimeoutSeconds:      2,
		},
	}
}

func (c *Cluster) makeService() *v1.Service {
	labels := c.getLabels()
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            appName,
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{c.ownerRef},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       appName,
					Port:       mon.DefaultPort,
					TargetPort: intstr.FromInt(mon.DefaultPort),
					Protocol:   v1.ProtocolTCP,
				},
			},
			Selector: labels,
			// ClusterIPNone allows us to use Ceph DNS discovery which is nice
			ClusterIP: v1.ClusterIPNone,
		},
	}
}
