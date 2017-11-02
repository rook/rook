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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
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
	err := a.Start(namespace)
	assert.Nil(t, err)

	// check clusters rbac roles
	_, err = clientset.CoreV1().ServiceAccounts(namespace).Get("rook-agent", metav1.GetOptions{})
	assert.Nil(t, err)

	role, err := clientset.RbacV1beta1().ClusterRoles().Get("rook-agent", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 3, len(role.Rules))

	binding, err := clientset.RbacV1beta1().ClusterRoleBindings().Get("rook-agent", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "rook-agent", binding.Subjects[0].Name)
	assert.Equal(t, "ServiceAccount", binding.Subjects[0].Kind)

	// check daemonset parameters
	agentDS, err := clientset.Extensions().DaemonSets(namespace).Get("rook-agent", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, namespace, agentDS.Namespace)
	assert.Equal(t, "rook-agent", agentDS.Name)
	assert.True(t, *agentDS.Spec.Template.Spec.Containers[0].SecurityContext.Privileged)
	volumes := agentDS.Spec.Template.Spec.Volumes
	assert.Equal(t, 6, len(volumes))
	volumeMounts := agentDS.Spec.Template.Spec.Containers[0].VolumeMounts
	assert.Equal(t, 6, len(volumeMounts))
	envs := agentDS.Spec.Template.Spec.Containers[0].Env
	assert.Equal(t, 2, len(envs))
	image := agentDS.Spec.Template.Spec.Containers[0].Image
	assert.Equal(t, "rook/test", image)
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
	image, err := getContainerImage(clientset)
	assert.Nil(t, err)
	assert.Equal(t, "rook/test", image)
}

func TestGetContainerImageMultipleContainers(t *testing.T) {
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
				{
					Name:  "otherPodContainer",
					Image: "rook/test2",
				},
			},
		},
	}
	clientset.CoreV1().Pods("Default").Create(&pod)

	// start a basic cluster
	_, err := getContainerImage(clientset)
	assert.NotNil(t, err)
	assert.Equal(t, "failed to get container image. There should only be exactly one container in this pod", err.Error())
}
