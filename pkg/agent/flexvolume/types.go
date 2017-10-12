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

package flexvolume

const (
	ReadOnly  = "ro"
	ReadWrite = "rw"
)

// VolumeManager handles flexvolume plugin storage operations
type VolumeManager interface {
	Init() error
	Attach(image, pool, clusterName string) (string, error)
	Detach(image, pool, clusterName string) error
}

type AttachOptions struct {
	Image        string `json:"image"`
	Pool         string `json:"pool"`
	ClusterName  string `json:"ClusterName"`
	StorageClass string `json:"storageClass"`
	MountDir     string `json:"mountDir"`
	RW           string `json:"kubernetes.io/readwrite"`
	FsType       string `json:"kubernetes.io/fsType"`
	VolumeName   string `json:"kubernetes.io/pvOrVolumeName"` // only available on 1.7
	Pod          string `json:"kubernetes.io/pod.name"`
	PodID        string `json:"kubernetes.io/pod.uid"`
	PodNamespace string `json:"kubernetes.io/pod.namespace"`
}

type LogMessage struct {
	Message string `json:"message"`
	IsError bool   `json:"isError"`
}
