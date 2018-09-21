/*
Copyright 2018 The Rook Authors. All rights reserved.

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

package config

import "k8s.io/api/core/v1"

const (
	// DefaultConfigMountName is the name of the volume mount used to mount the default Ceph config
	DefaultConfigMountName = "ceph-default-config-dir"
)

// DefaultConfigVolume returns an empty volume used to store Ceph's config at the default path
// in containers.
func DefaultConfigVolume() v1.Volume {
	return v1.Volume{Name: DefaultConfigMountName,
		VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}}
}

// DefaultConfigMount returns a volume mount to Ceph's default config path.
func DefaultConfigMount() v1.VolumeMount {
	return v1.VolumeMount{Name: DefaultConfigMountName, MountPath: DefaultConfigDir}
}
