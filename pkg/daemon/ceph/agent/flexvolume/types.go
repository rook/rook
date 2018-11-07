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

package flexvolume

const (
	ReadOnly  = "ro"
	ReadWrite = "rw"
)

// VolumeManager handles flexvolume plugin storage operations
type VolumeManager interface {
	Init() error
	Attach(image, pool, clusterName string) (string, error)
	Detach(image, pool, clusterName string, force bool) error
}

type VolumeController interface {
	Attach(attachOpts AttachOptions, devicePath *string) error
	Detach(detachOpts AttachOptions, _ *struct{} /* void reply */) error
	DetachForce(detachOpts AttachOptions, _ *struct{} /* void reply */) error
	RemoveAttachmentObject(detachOpts AttachOptions, safeToDetach *bool) error
	Log(message LogMessage, _ *struct{} /* void reply */) error
	GetAttachInfoFromMountDir(mountDir string, attachOptions *AttachOptions) error
}

type AttachOptions struct {
	Image            string `json:"image"`
	Pool             string `json:"pool"`
	ClusterNamespace string `json:"clusterNamespace"`
	ClusterName      string `json:"clusterName"`
	StorageClass     string `json:"storageClass"`
	MountDir         string `json:"mountDir"`
	FsName           string `json:"fsName"`
	Path             string `json:"path"` // Path within the CephFS to mount
	RW               string `json:"kubernetes.io/readwrite"`
	FsType           string `json:"kubernetes.io/fsType"`
	VolumeName       string `json:"kubernetes.io/pvOrVolumeName"` // only available on 1.7
	Pod              string `json:"kubernetes.io/pod.name"`
	PodID            string `json:"kubernetes.io/pod.uid"`
	PodNamespace     string `json:"kubernetes.io/pod.namespace"`
}

type LogMessage struct {
	Message string `json:"message"`
	IsError bool   `json:"isError"`
}

type GlobalMountPathInput struct {
	VolumeName string `json:"volumeName"`
	DriverDir  string `json:"driverDir"`
}
