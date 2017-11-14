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
	"encoding/json"
	"fmt"
	"os"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/agent/flexvolume/crd"
	opcluster "github.com/rook/rook/pkg/operator/cluster"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/api/rbac/v1beta1"
	kserrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	agentDaemonsetName       = "rook-agent"
	flexvolumePathDirEnv     = "FLEXVOLUME_DIR_PATH"
	flexvolumeDefaultDirPath = "/usr/libexec/kubernetes/kubelet-plugins/volume/exec/"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-agent")

var clusterAccessRules = []v1beta1.PolicyRule{
	{
		APIGroups: []string{""},
		Resources: []string{"pods", "secrets", "configmaps", "persistentvolumes", "nodes", "nodes/proxy"},
		Verbs:     []string{"get", "list"},
	},
	{
		APIGroups: []string{k8sutil.CustomResourceGroup},
		Resources: []string{crd.CustomResourceNamePlural},
		Verbs:     []string{"get", "list", "create", "delete", "update"},
	},
	{
		APIGroups: []string{k8sutil.CustomResourceGroup},
		Resources: []string{opcluster.CustomResourceNamePlural},
		Verbs:     []string{"get", "list", "watch"},
	},
	{
		APIGroups: []string{"storage.k8s.io"},
		Resources: []string{"storageclasses"},
		Verbs:     []string{"get"},
	},
}

// New creates an instance of Agent
func New(clientset kubernetes.Interface) *Agent {
	return &Agent{
		clientset: clientset,
	}
}

// Start the agent
func (a *Agent) Start(namespace string) error {

	err := k8sutil.MakeClusterRole(a.clientset, namespace, agentDaemonsetName, clusterAccessRules)
	if err != nil {
		return fmt.Errorf("failed to init RBAC for rook-agents. %+v", err)
	}

	err = a.createAgentDaemonSet(namespace)
	if err != nil {
		return fmt.Errorf("Error starting agent daemonset: %v", err)
	}
	return nil
}

func (a *Agent) createAgentDaemonSet(namespace string) error {

	// Using the rook-operator image to deploy the rook-agents
	agentImage, err := getContainerImage(a.clientset)
	if err != nil {
		return err
	}
	privileged := true
	ds := &extensions.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: agentDaemonsetName,
		},
		Spec: extensions.DaemonSetSpec{
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": agentDaemonsetName,
					},
				},
				Spec: v1.PodSpec{
					ServiceAccountName: agentDaemonsetName,
					Containers: []v1.Container{
						{
							Name:  agentDaemonsetName,
							Image: agentImage,
							Args:  []string{"agent"},
							SecurityContext: &v1.SecurityContext{
								Privileged: &privileged,
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "flexvolume",
									MountPath: "/flexmnt",
								},
								{
									Name:      "dev",
									MountPath: "/dev",
								},
								{
									Name:      "sys",
									MountPath: "/sys",
								},
								{
									Name:      "libmodules",
									MountPath: "/lib/modules",
								},
								{
									Name:      "modprobe",
									MountPath: "/sbin/modprobe",
								},
								{
									Name:      "modinfo",
									MountPath: "/sbin/modinfo",
								},
							},
							Env: []v1.EnvVar{
								k8sutil.NamespaceEnvVar(),
								k8sutil.NodeEnvVar(),
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "flexvolume",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: a.discoverFlexvolumeDir(),
								},
							},
						},
						{
							Name: "dev",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: "/dev",
								},
							},
						},
						{
							Name: "sys",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: "/sys",
								},
							},
						},
						{
							Name: "libmodules",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: "/lib/modules",
								},
							},
						},
						{
							Name: "modprobe",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: "/sbin/modprobe",
								},
							},
						},
						{
							Name: "modinfo",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: "/sbin/modinfo",
								},
							},
						},
					},
					HostNetwork: true,
				},
			},
		},
	}

	_, err = a.clientset.Extensions().DaemonSets(namespace).Create(ds)
	if err != nil {
		if !kserrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create rook-agent daemon set. %+v", err)
		}
		logger.Infof("rook-agent daemonset already exists")
	} else {
		logger.Infof("rook-agent daemonset started")
	}
	return nil

}

func (a *Agent) discoverFlexvolumeDir() string {
	//copy flexvolume to flexvolume dir
	nodeName := os.Getenv(k8sutil.NodeNameEnvVar)
	if nodeName == "" {
		logger.Warningf("cannot detect the node name. Please provide using the downward API in the rook operator manifest file")
		return getDefaultFlexvolumeDir()
	}

	// determining where the path of the flexvolume dir on the node
	var flexvolumeDirPath string
	nodeConfigURI, err := k8sutil.NodeConfigURI()
	if err != nil {
		logger.Warningf(err.Error())
		return getDefaultFlexvolumeDir()
	}
	nodeConfig, err := a.clientset.Core().RESTClient().Get().RequestURI(nodeConfigURI).DoRaw()
	if err != nil {
		logger.Warningf("unable to query node configuration: %v", err)
		return getDefaultFlexvolumeDir()
	}

	// unmarshall to a NodeConfigKubelet
	configKubelet := NodeConfigKubelet{}
	if err := json.Unmarshal(nodeConfig, &configKubelet); err != nil {
		logger.Warningf("unable to parse node config from Kubelet: %+v", err)
		return getDefaultFlexvolumeDir()
	}
	flexvolumeDirPath = configKubelet.ComponentConfig.VolumePluginDir
	if flexvolumeDirPath == "" {
		// unmarshall to a NodeConfigControllerManager
		configControllerManager := NodeConfigControllerManager{}
		if err := json.Unmarshal(nodeConfig, &configControllerManager); err != nil {
			logger.Warningf("unable to parse node config from controller manager: %+v", err)
			return getDefaultFlexvolumeDir()
		}
		flexvolumeDirPath = configControllerManager.ComponentConfig.VolumeConfiguration.FlexVolumePluginDir
		if flexvolumeDirPath == "" {
			flexvolumeDirPath = getDefaultFlexvolumeDir()
		}
	}

	logger.Infof("using flexvolume dir path: %s", flexvolumeDirPath)
	return flexvolumeDirPath
}

func getDefaultFlexvolumeDir() string {
	logger.Info("getting flexvolume dir path from provided env var")
	flexvolumeDirPath := os.Getenv(flexvolumePathDirEnv)
	if flexvolumeDirPath == "" {
		logger.Infof("flexvolume dir path is not provided. Defaulting to: %s", flexvolumeDefaultDirPath)
		flexvolumeDirPath = flexvolumeDefaultDirPath
	}
	return flexvolumeDirPath
}

func getContainerImage(clientset kubernetes.Interface) (string, error) {

	podName := os.Getenv(k8sutil.PodNameEnvVar)
	if podName == "" {
		return "", fmt.Errorf("cannot detect the pod name. Please provide it using the downward API in the manifest file")
	}
	podNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	if podName == "" {
		return "", fmt.Errorf("cannot detect the pod namespace. Please provide it using the downward API in the manifest file")
	}

	pod, err := clientset.Core().Pods(podNamespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	if len(pod.Spec.Containers) != 1 {
		return "", fmt.Errorf("failed to get container image. There should only be exactly one container in this pod")
	}

	return pod.Spec.Containers[0].Image, nil
}
