/*
Copyright 2023 The Ceph-CSI Authors.

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

package kubernetes

type ClusterInfo struct {
	// ClusterID is used for unique identification
	ClusterID string `json:"clusterID"`
	// Monitors is monitor list for corresponding cluster ID
	Monitors []string `json:"monitors"`
	// CephFS contains CephFS specific options
	CephFS CephFS `json:"cephFS"`
	// RBD Contains RBD specific options
	RBD RBD `json:"rbd"`
	// NFS contains NFS specific options
	NFS NFS `json:"nfs"`
	// Read affinity map options
	ReadAffinity ReadAffinity `json:"readAffinity"`
}

type CephFS struct {
	// symlink filepath for the network namespace where we need to execute commands.
	NetNamespaceFilePath string `json:"netNamespaceFilePath"`
	// SubvolumeGroup contains the name of the SubvolumeGroup for CSI volumes
	SubvolumeGroup string `json:"subvolumeGroup"`
	// RadosNamespace is a rados namespace in the filesystem metadata pool
	RadosNamespace string `json:"radosNamespace"`
	// KernelMountOptions contains the kernel mount options for CephFS volumes
	KernelMountOptions string `json:"kernelMountOptions"`
	// FuseMountOptions contains the fuse mount options for CephFS volumes
	FuseMountOptions string `json:"fuseMountOptions"`
}
type RBD struct {
	// symlink filepath for the network namespace where we need to execute commands.
	NetNamespaceFilePath string `json:"netNamespaceFilePath"`
	// RadosNamespace is a rados namespace in the pool
	RadosNamespace string `json:"radosNamespace"`
	// RBD mirror daemons running in the ceph cluster.
	MirrorDaemonCount int `json:"mirrorDaemonCount"`
}

type NFS struct {
	// symlink filepath for the network namespace where we need to execute commands.
	NetNamespaceFilePath string `json:"netNamespaceFilePath"`
}

type ReadAffinity struct {
	Enabled             bool     `json:"enabled"`
	CrushLocationLabels []string `json:"crushLocationLabels"`
}
