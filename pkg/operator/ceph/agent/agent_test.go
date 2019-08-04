/*
Copyright 2017 The Rook Authors. All rights reserved.

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

// Package agent to manage Kubernetes storage attach events.
package agent

import (
	"os"
	"testing"

	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStartAgentDaemonset(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.PodNameEnvVar, "rook-operator")
	defer os.Unsetenv(k8sutil.PodNameEnvVar)

	os.Setenv(agentDaemonsetPriorityClassNameEnv, "my-priority-class")
	defer os.Unsetenv(agentDaemonsetPriorityClassNameEnv)

	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-operator",
			Namespace: "rook-system",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "mypodContainer",
					Image: "rook/test",
				},
			},
		},
	}
	clientset.CoreV1().Pods("rook-system").Create(&pod)

	namespace := "ns"
	a := New(clientset)

	// start a basic cluster
	err := a.Start(namespace, "rook/rook:myversion", "mysa")
	assert.Nil(t, err)

	// check daemonset parameters
	agentDS, err := clientset.AppsV1().DaemonSets(namespace).Get("rook-ceph-agent", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, namespace, agentDS.Namespace)
	assert.Equal(t, "rook-ceph-agent", agentDS.Name)
	assert.Equal(t, "my-priority-class", agentDS.Spec.Template.Spec.PriorityClassName)
	assert.Equal(t, "mysa", agentDS.Spec.Template.Spec.ServiceAccountName)
	assert.True(t, *agentDS.Spec.Template.Spec.Containers[0].SecurityContext.Privileged)
	volumes := agentDS.Spec.Template.Spec.Volumes
	assert.Equal(t, 4, len(volumes))
	volumeMounts := agentDS.Spec.Template.Spec.Containers[0].VolumeMounts
	assert.Equal(t, 4, len(volumeMounts))
	envs := agentDS.Spec.Template.Spec.Containers[0].Env
	assert.Equal(t, 5, len(envs))
	image := agentDS.Spec.Template.Spec.Containers[0].Image
	assert.Equal(t, "rook/rook:myversion", image)
	assert.Nil(t, agentDS.Spec.Template.Spec.Tolerations)
}

func TestGetContainerImage(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "Default")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.PodNameEnvVar, "mypod")
	defer os.Unsetenv(k8sutil.PodNameEnvVar)

	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mypod",
			Namespace: "Default",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "mypodContainer",
					Image: "rook/test",
				},
			},
		},
	}
	clientset.CoreV1().Pods("Default").Create(&pod)

	// start a basic cluster
	returnPod, err := k8sutil.GetRunningPod(clientset)
	assert.Nil(t, err)
	assert.Equal(t, "mypod", returnPod.Name)
}

func TestGetContainerImageMultipleContainers(t *testing.T) {

	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mypod",
			Namespace: "Default",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "mypodContainer",
					Image: "rook/test",
				},
				{
					Name:  "otherPodContainer",
					Image: "rook/test2",
				},
			},
		},
	}

	// start a basic cluster
	container, err := k8sutil.GetContainerImage(&pod, "foo")
	assert.NotNil(t, err)
	assert.Equal(t, "", container)
	assert.Equal(t, "failed to find image for container foo", err.Error())
}

func TestStartAgentDaemonsetWithToleration(t *testing.T) {
	clientset := test.New(3)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.PodNameEnvVar, "rook-operator")
	defer os.Unsetenv(k8sutil.PodNameEnvVar)

	os.Setenv(agentDaemonsetTolerationEnv, "NoSchedule")
	defer os.Unsetenv(agentDaemonsetTolerationEnv)

	os.Setenv(agentDaemonsetTolerationKeyEnv, "example")
	defer os.Unsetenv(agentDaemonsetTolerationKeyEnv)

	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-operator",
			Namespace: "rook-system",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "mypodContainer",
					Image: "rook/test",
				},
			},
		},
	}
	clientset.CoreV1().Pods("rook-system").Create(&pod)

	namespace := "ns"
	a := New(clientset)

	// start a basic cluster
	err := a.Start(namespace, "rook/test", "mysa")
	assert.Nil(t, err)

	// check daemonset toleration
	agentDS, err := clientset.AppsV1().DaemonSets(namespace).Get("rook-ceph-agent", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(agentDS.Spec.Template.Spec.Tolerations))
	assert.Equal(t, "mysa", agentDS.Spec.Template.Spec.ServiceAccountName)
	assert.Equal(t, "NoSchedule", string(agentDS.Spec.Template.Spec.Tolerations[0].Effect))
	assert.Equal(t, "example", string(agentDS.Spec.Template.Spec.Tolerations[0].Key))
	assert.Equal(t, "Exists", string(agentDS.Spec.Template.Spec.Tolerations[0].Operator))
}

func TestDiscoverFlexDir(t *testing.T) {
	path, source := getDefaultFlexvolumeDir()
	assert.Equal(t, "default", source)
	assert.Equal(t, "/usr/libexec/kubernetes/kubelet-plugins/volume/exec/", path)

	os.Setenv(flexvolumePathDirEnv, "/my/flex/path/")
	defer os.Unsetenv(flexvolumePathDirEnv)
	path, source = getDefaultFlexvolumeDir()
	assert.Equal(t, "env var", source)
	assert.Equal(t, "/my/flex/path/", path)
}
