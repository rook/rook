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
	"encoding/json"
	"fmt"
	"os"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	kserrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	agentDaemonsetName             = "rook-ceph-agent"
	flexvolumePathDirEnv           = "FLEXVOLUME_DIR_PATH"
	flexvolumeDefaultDirPath       = "/usr/libexec/kubernetes/kubelet-plugins/volume/exec/"
	agentDaemonsetTolerationEnv    = "AGENT_TOLERATION"
	agentDaemonsetTolerationKeyEnv = "AGENT_TOLERATION_KEY"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-agent")

// New creates an instance of Agent
func New(clientset kubernetes.Interface) *Agent {
	return &Agent{
		clientset: clientset,
	}
}

// Start the agent
func (a *Agent) Start(namespace, agentImage, serviceAccount string) error {

	err := a.createAgentDaemonSet(namespace, agentImage, serviceAccount)
	if err != nil {
		return fmt.Errorf("Error starting agent daemonset: %v", err)
	}
	return nil
}

func (a *Agent) createAgentDaemonSet(namespace, agentImage, serviceAccount string) error {

	flexvolumeDirPath, source := a.discoverFlexvolumeDir()
	logger.Infof("discovered flexvolume dir path from source %s. value: %s", source, flexvolumeDirPath)

	privileged := true
	ds := &extensions.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: agentDaemonsetName,
		},
		Spec: extensions.DaemonSetSpec{
			UpdateStrategy: extensions.DaemonSetUpdateStrategy{
				Type: extensions.RollingUpdateDaemonSetStrategyType,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": agentDaemonsetName,
					},
				},
				Spec: v1.PodSpec{
					ServiceAccountName: serviceAccount,
					Containers: []v1.Container{
						{
							Name:  agentDaemonsetName,
							Image: agentImage,
							Args:  []string{"ceph", "agent"},
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
									Path: flexvolumeDirPath,
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
					},
					HostNetwork: true,
				},
			},
		},
	}

	// Add toleration if any
	tolerationValue := os.Getenv(agentDaemonsetTolerationEnv)
	if tolerationValue != "" {
		ds.Spec.Template.Spec.Tolerations = []v1.Toleration{
			{
				Effect:   v1.TaintEffect(tolerationValue),
				Operator: v1.TolerationOpExists,
				Key:      os.Getenv(agentDaemonsetTolerationKeyEnv),
			},
		}
	}

	_, err := a.clientset.Extensions().DaemonSets(namespace).Create(ds)
	if err != nil {
		if !kserrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create rook-ceph-agent daemon set. %+v", err)
		}
		logger.Infof("rook-ceph-agent daemonset already exists, updating ...")
		_, err = a.clientset.Extensions().DaemonSets(namespace).Update(ds)
		if err != nil {
			return fmt.Errorf("failed to update rook-ceph-agent daemon set. %+v", err)
		}
	} else {
		logger.Infof("rook-ceph-agent daemonset started")
	}
	return nil

}

func (a *Agent) discoverFlexvolumeDir() (flexvolumeDirPath, source string) {
	//copy flexvolume to flexvolume dir
	nodeName := os.Getenv(k8sutil.NodeNameEnvVar)
	if nodeName == "" {
		logger.Warningf("cannot detect the node name. Please provide using the downward API in the rook operator manifest file")
		return getDefaultFlexvolumeDir()
	}

	// determining where the path of the flexvolume dir on the node
	nodeConfigURI, err := k8sutil.NodeConfigURI()
	if err != nil {
		logger.Warningf(err.Error())
		return getDefaultFlexvolumeDir()
	}
	nodeConfig, err := a.clientset.CoreV1().RESTClient().Get().RequestURI(nodeConfigURI).DoRaw()
	if err != nil {
		logger.Warningf("unable to query node configuration: %v", err)
		return getDefaultFlexvolumeDir()
	}

	// unmarshal to a NodeConfigKubelet
	configKubelet := NodeConfigKubelet{}
	if err := json.Unmarshal(nodeConfig, &configKubelet); err != nil {
		logger.Warningf("unable to parse node config from Kubelet: %+v", err)
	} else {
		flexvolumeDirPath = configKubelet.ComponentConfig.VolumePluginDir
	}

	if flexvolumeDirPath != "" {
		return flexvolumeDirPath, "NodeConfigKubelet"
	}

	// unmarshal to a NodeConfigControllerManager
	configControllerManager := NodeConfigControllerManager{}
	if err := json.Unmarshal(nodeConfig, &configControllerManager); err != nil {
		logger.Warningf("unable to parse node config from controller manager: %+v", err)
	} else {
		flexvolumeDirPath = configControllerManager.ComponentConfig.VolumeConfiguration.FlexVolumePluginDir
	}

	if flexvolumeDirPath != "" {
		return flexvolumeDirPath, "NodeConfigControllerManager"
	}

	// unmarshal to a KubeletConfiguration
	kubeletConfiguration := KubeletConfiguration{}
	if err := json.Unmarshal(nodeConfig, &kubeletConfiguration); err != nil {
		logger.Warningf("unable to parse node config as kubelet configuration: %+v", err)
	} else {
		flexvolumeDirPath = kubeletConfiguration.KubeletConfig.VolumePluginDir
	}

	if flexvolumeDirPath != "" {
		return flexvolumeDirPath, "KubeletConfiguration"
	}

	return getDefaultFlexvolumeDir()
}

func getDefaultFlexvolumeDir() (flexvolumeDirPath, source string) {
	logger.Infof("getting flexvolume dir path from %s env var", flexvolumePathDirEnv)
	flexvolumeDirPath = os.Getenv(flexvolumePathDirEnv)
	if flexvolumeDirPath != "" {
		return flexvolumeDirPath, "env var"
	}

	logger.Infof("flexvolume dir path env var %s is not provided. Defaulting to: %s",
		flexvolumePathDirEnv, flexvolumeDefaultDirPath)
	flexvolumeDirPath = flexvolumeDefaultDirPath

	return flexvolumeDirPath, "default"
}
