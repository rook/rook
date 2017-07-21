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
package mon

import (
	"fmt"

	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

func ClusterNameEnvVar(name string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOKD_CLUSTER_NAME", Value: name}
}

func MonEndpointEnvVar() v1.EnvVar {
	ref := &v1.ConfigMapKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: monConfigMapName}, Key: monEndpointKey}
	return v1.EnvVar{Name: "ROOKD_MON_ENDPOINTS", ValueFrom: &v1.EnvVarSource{ConfigMapKeyRef: ref}}
}

func MonSecretEnvVar() v1.EnvVar {
	ref := &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: appName}, Key: monSecretName}
	return v1.EnvVar{Name: "ROOKD_MON_SECRET", ValueFrom: &v1.EnvVarSource{SecretKeyRef: ref}}
}

func AdminSecretEnvVar() v1.EnvVar {
	ref := &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: appName}, Key: adminSecretName}
	return v1.EnvVar{Name: "ROOKD_ADMIN_SECRET", ValueFrom: &v1.EnvVarSource{SecretKeyRef: ref}}
}

func (c *Cluster) getLabels(name string) map[string]string {
	return map[string]string{
		k8sutil.AppAttr: appName,
		"mon":           name,
		monClusterAttr:  c.Namespace,
	}
}

func (c *Cluster) makeReplicaSet(config *MonConfig, nodeName string) *extensions.ReplicaSet {

	rs := &extensions.ReplicaSet{}
	rs.Name = config.Name
	rs.Namespace = c.Namespace

	pod := c.makeMonPod(config, nodeName)
	replicaCount := int32(1)
	rs.Spec = extensions.ReplicaSetSpec{
		Template: v1.PodTemplateSpec{
			ObjectMeta: pod.ObjectMeta,
			Spec:       pod.Spec,
		},
		Replicas: &replicaCount,
	}

	return rs
}

func (c *Cluster) makeMonPod(config *MonConfig, nodeName string) *v1.Pod {
	dataDirSource := v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}
	if c.dataDirHostPath != "" {
		dataDirSource = v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: c.dataDirHostPath}}
	}

	container := c.monContainer(config, c.clusterInfo.FSID)
	podSpec := v1.PodSpec{
		Containers:    []v1.Container{container},
		RestartPolicy: v1.RestartPolicyAlways,
		NodeSelector:  map[string]string{apis.LabelHostname: nodeName},
		Volumes: []v1.Volume{
			{Name: k8sutil.DataDirVolume, VolumeSource: dataDirSource},
			k8sutil.ConfigOverrideVolume(),
		},
	}
	c.placement.ApplyToPodSpec(&podSpec)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        config.Name,
			Namespace:   c.Namespace,
			Labels:      c.getLabels(config.Name),
			Annotations: map[string]string{},
		},
		Spec: podSpec,
	}

	k8sutil.SetPodVersion(pod, k8sutil.VersionAttr, c.Version)
	return pod
}

func (c *Cluster) monContainer(config *MonConfig, fsid string) v1.Container {

	return v1.Container{
		Args: []string{
			"mon",
			fmt.Sprintf("--config-dir=%s", k8sutil.DataDir),
			fmt.Sprintf("--name=%s", config.Name),
			fmt.Sprintf("--port=%d", config.Port),
			fmt.Sprintf("--fsid=%s", fsid),
		},
		Name:  appName,
		Image: k8sutil.MakeRookImage(c.Version),
		Ports: []v1.ContainerPort{
			{
				Name:          "client",
				ContainerPort: config.Port,
				Protocol:      v1.ProtocolTCP,
			},
		},
		VolumeMounts: []v1.VolumeMount{
			{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
			k8sutil.ConfigOverrideMount(),
		},
		Env: []v1.EnvVar{
			{Name: k8sutil.PodIPEnvVar, ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "status.podIP"}}},
			ClusterNameEnvVar(c.Namespace),
			MonEndpointEnvVar(),
			MonSecretEnvVar(),
			AdminSecretEnvVar(),
			k8sutil.ConfigOverrideEnvVar(),
		},
	}
}
