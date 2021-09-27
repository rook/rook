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
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	BinariesMountPath = "/rook"
)

// PathToVolumeName converts a path to a valid volume name
func PathToVolumeName(path string) string {
	// kubernetes volume names must match this regex: [a-z0-9]([-a-z0-9]*[a-z0-9])?

	sanitized := make([]rune, 0, len(path))
	for _, c := range path {
		switch {
		case '0' <= c && c <= '9':
			fallthrough
		case 'a' <= c && c <= 'z':
			sanitized = append(sanitized, c)
			continue
		case 'A' <= c && c <= 'Z':
			// convert upper to lower case
			sanitized = append(sanitized, c+('a'-'A'))
		default:
			// convert any non alphanum char to a hyphen
			sanitized = append(sanitized, '-')
		}
	}
	volumeName := string(sanitized)

	// trim any leading/trailing hyphens
	volumeName = strings.TrimPrefix(volumeName, "-")
	volumeName = strings.TrimSuffix(volumeName, "-")

	if len(volumeName) > validation.DNS1123LabelMaxLength {
		// keep an equal sample of the original name from both the beginning and from the end,
		// and add some characters from a hash of the full name to help prevent name collisions.
		// Make room for 3 hyphens in the middle (~ellipsis) and 1 hyphen to separate the hash.
		hashLength := 8
		sampleLength := int((validation.DNS1123LabelMaxLength - hashLength - 3 - 1) / 2)
		first := volumeName[0:sampleLength]
		last := volumeName[len(volumeName)-sampleLength:]
		hash := Hash(volumeName)
		volumeName = fmt.Sprintf("%s---%s-%s", first, last, hash[0:hashLength])
	}

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

func BinariesMountInfo() (v1.EnvVar, v1.Volume, v1.VolumeMount) {
	// To get rook inside the container, the config init container needs to copy "rook" binary into a volume.
	// Set the config flag so rook will copy the binaries.
	// Create the volume and mount that will be shared between the init container and the daemon container
	volumeName := "rookbinaries"
	e := v1.EnvVar{Name: "ROOK_COPY_BINARIES_PATH", Value: BinariesMountPath}
	v := v1.Volume{Name: volumeName, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}}
	m := v1.VolumeMount{Name: volumeName, MountPath: BinariesMountPath}
	return e, v, m
}
