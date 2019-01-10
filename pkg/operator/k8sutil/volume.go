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
	"os"
	"path/filepath"
	"strings"
)

// PathToVolumeName converts a path to a valid volume name
func PathToVolumeName(path string) string {
	// kubernetes volume names must match this regex: [a-z0-9]([-a-z0-9]*[a-z0-9])?

	// first replace all filepath separators with hyphens
	volumeName := strings.Replace(path, string(filepath.Separator), "-", -1)

	// convert underscores to hyphens
	volumeName = strings.Replace(volumeName, "_", "-", -1)

	// trim any leading/trailing hyphens
	volumeName = strings.TrimPrefix(volumeName, "-")
	volumeName = strings.TrimSuffix(volumeName, "-")

	return volumeName
}

// NodeConfigURI returns the node config URI path for this node
func NodeConfigURI() (string, error) {
	nodeName := os.Getenv(NodeNameEnvVar)
	if nodeName == "" {
		return "", fmt.Errorf("cannot detect the node name. Please provide using the downward API in the rook operator manifest file")
	}
	return fmt.Sprintf("api/v1/nodes/%s/proxy/configz", nodeName), nil
}
