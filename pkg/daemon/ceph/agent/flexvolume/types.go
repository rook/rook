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
	// ReadOnly mount mode
	ReadOnly = "ro"
	// ReadWrite mount mode
	ReadWrite = "rw"
)

// VolumeManager handles flexvolume plugin storage operations
type VolumeManager interface {
	Init() error
	Attach(image, pool, id, key, clusterName string) (string, error)
	Detach(image, pool, id, key, clusterName string, force bool) error
	Expand(image, pool, clusterName string, size uint64) error
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
	BlockPool        string `json:"blockPool"`
	Pool             string `json:"pool"`
	ClusterNamespace string `json:"clusterNamespace"`
	ClusterName      string `json:"clusterName"`
	StorageClass     string `json:"storageClass"`
	MountDir         string `json:"mountDir"`
	FsName           string `json:"fsName"`
	Path             string `json:"path"` // Path within the CephFS to mount
	MountUser        string `json:"mountUser"`
	MountSecret      string `json:"mountSecret"`
	RW               string `json:"kubernetes.io/readwrite"`
	FsType           string `json:"kubernetes.io/fsType"`
	FsGroup          string `json:"kubernetes.io/fsGroup"`
	VolumeName       string `json:"kubernetes.io/pvOrVolumeName"` // only available on 1.7
	Pod              string `json:"kubernetes.io/pod.name"`
	PodID            string `json:"kubernetes.io/pod.uid"`
	PodNamespace     string `json:"kubernetes.io/pod.namespace"`
}

type ExpandOptions struct {
	Pool             string `json:"pool"`
	RW               string `json:"kubernetes.io/readwrite"`
	ClusterNamespace string `json:"clusterNamespace"`
	DataBlockPool    string `json:"dataBlockPool"`
	Image            string `json:"image"`
	FsType           string `json:"kubernetes.io/fsType"`
	StorageClass     string `json:"storageClass"`
	VolumeName       string `json:"kubernetes.io/pvOrVolumeName"`
}

type ExpandArgs struct {
	ExpandOptions *ExpandOptions
	Size          uint64
}

type LogMessage struct {
	Message string `json:"message"`
	IsError bool   `json:"isError"`
}

type GlobalMountPathInput struct {
	VolumeName string `json:"volumeName"`
	DriverDir  string `json:"driverDir"`
}
