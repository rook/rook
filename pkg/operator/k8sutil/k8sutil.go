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

// Package k8sutil for Kubernetes helpers.
package k8sutil

import (
	"fmt"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/util/version"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-k8sutil")

const (
	// Namespace for rook
	Namespace = "rook"
	// DefaultNamespace for the cluster
	DefaultNamespace = "default"
	// DataDirVolume data dir volume
	DataDirVolume = "rook-data"
	// DataDir folder
	DataDir = "/var/lib/rook"
	// RookType for the CRD
	RookType = "kubernetes.io/rook"
	// PodNameEnvVar is the env variable for getting the pod name via downward api
	PodNameEnvVar = "POD_NAME"
	// PodNamespaceEnvVar is the env variable for getting the pod namespace via downward api
	PodNamespaceEnvVar = "POD_NAMESPACE"
	// NodeNameEnvVar is the env variable for getting the node via downward api
	NodeNameEnvVar = "NODE_NAME"
)

// GetK8SVersion gets the version of the running K8S cluster
func GetK8SVersion(clientset kubernetes.Interface) (*version.Version, error) {
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("Error getting server version: %v", err)
	}

	// make sure the kubernetes version is parseable
	index := strings.Index(serverVersion.GitVersion, "+")
	if index != -1 {
		newVersion := serverVersion.GitVersion[:index]
		logger.Infof("returning version %s instead of %s", newVersion, serverVersion.GitVersion)
		serverVersion.GitVersion = newVersion
	}
	return version.MustParseSemantic(serverVersion.GitVersion), nil
}
