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
	"k8s.io/client-go/kubernetes"
	componentconfig "k8s.io/kube-controller-manager/config/v1alpha1"
)

// Agent reference to be deployed
type Agent struct {
	clientset kubernetes.Interface
}

// NodeConfigControllerManager is a reference of all the configuration for the K8S node from the controllermanager
type NodeConfigControllerManager struct {
	ComponentConfig componentconfig.KubeControllerManagerConfiguration `json:"componentconfig"`
}

// KubeletConfiguration represents the response from the node config URI (configz) in Kubernetes 1.8+
type KubeletConfiguration struct {
	KubeletConfig struct {
		VolumePluginDir string `json:"volumePluginDir"`
	} `json:"kubeletconfig"`
}
