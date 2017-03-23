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

	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/labels"
)

func MonSecretEnvVar() v1.EnvVar {
	return v1.EnvVar{Name: "ROOKD_MON_SECRET", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: appName}, Key: monSecretName}}}
}

func AdminSecretEnvVar() v1.EnvVar {
	return v1.EnvVar{Name: "ROOKD_ADMIN_SECRET", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: appName}, Key: adminSecretName}}}
}

func getLabels(clusterName string) map[string]string {
	return map[string]string{
		k8sutil.AppAttr: appName,
		monClusterAttr:  clusterName,
	}
}

func (c *Cluster) makeMonPod(config *MonConfig, clusterInfo *mon.ClusterInfo, antiAffinity bool) *v1.Pod {

	container := c.monContainer(config, clusterInfo)
	dataDirSource := v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}
	if c.dataDirHostPath != "" {
		dataDirSource = v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: c.dataDirHostPath}}
	}

	pod := &v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        config.Name,
			Namespace:   c.Namespace,
			Labels:      getLabels(clusterInfo.Name),
			Annotations: map[string]string{},
		},
		Spec: v1.PodSpec{
			Containers:    []v1.Container{container},
			RestartPolicy: v1.RestartPolicyAlways,
			Volumes: []v1.Volume{
				{Name: k8sutil.DataDirVolume, VolumeSource: dataDirSource},
			},
		},
	}

	k8sutil.SetPodVersion(pod, k8sutil.VersionAttr, c.Version)

	if antiAffinity {
		k8sutil.PodWithAntiAffinity(pod, monClusterAttr, clusterInfo.Name)
	}
	return pod
}

func (c *Cluster) monContainer(config *MonConfig, clusterInfo *mon.ClusterInfo) v1.Container {
	command := fmt.Sprintf("/usr/bin/rookd mon --data-dir=%s --name=%s --mon-endpoints=%s --port=%d --fsid=%s --cluster-name=%s",
		k8sutil.DataDir, config.Name, mon.FlattenMonEndpoints(clusterInfo.Monitors), config.Port, clusterInfo.FSID, clusterInfo.Name)

	return v1.Container{
		// TODO: fix "sleep 5".
		// Without waiting some time, there is highly probable flakes in network setup.
		Command: []string{"/bin/sh", "-c", fmt.Sprintf("sleep 5; %s", command)},
		Name:    appName,
		Image:   k8sutil.MakeRookImage(c.Version),
		Ports: []v1.ContainerPort{
			{
				Name:          "client",
				ContainerPort: config.Port,
				Protocol:      v1.ProtocolTCP,
			},
		},
		VolumeMounts: []v1.VolumeMount{
			{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
		},
		Env: []v1.EnvVar{
			{Name: k8sutil.PodIPEnvVar, ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "status.podIP"}}},
			MonSecretEnvVar(),
			AdminSecretEnvVar(),
		},
	}
}

func (c *Cluster) pollPods(clientset kubernetes.Interface, clusterName string) ([]*v1.Pod, []*v1.Pod, error) {
	podList, err := clientset.CoreV1().Pods(c.Namespace).List(listOptions(clusterName))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list running pods: %v", err)
	}

	var running []*v1.Pod
	var pending []*v1.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]

		switch pod.Status.Phase {
		case v1.PodRunning:
			running = append(running, pod)
		case v1.PodPending:
			pending = append(pending, pod)
		default:
			logger.Warningf("unknown pod %s status: %v", pod.Name, pod.Status.Phase)
		}
	}

	return running, pending, nil
}

func listOptions(clusterName string) v1.ListOptions {
	return v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			monClusterAttr:  clusterName,
			k8sutil.AppAttr: appName,
		}).String(),
	}
}
