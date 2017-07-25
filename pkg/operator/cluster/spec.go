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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/

// Package cluster to manage a rook cluster.
package cluster

import (
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/osd"
)

// Spec for the cluster
type Spec struct {
	// VersionTag is the expected version of the rook container to run in the cluster.
	// The operator will eventually make the rook cluster version
	// equal to the expected version.
	VersionTag string `json:"versionTag"`

	// Paused is to pause the control of the operator for the rook cluster.
	Paused bool `json:"paused,omitempty"`

	// The path on the host where config and data can be persisted.
	DataDirHostPath string `json:"dataDirHostPath"`

	// The placement-related configuration to pass to kubernetes (affinity, node selector, tolerations).
	Placement PlacementSpec `json:"placement,omitempty"`

	// A spec for available storage in the cluster and how it should be used
	Storage osd.StorageSpec `json:"storage"`
}

// PoolSpec is the specific spec for the redundancy
type PoolSpec struct {
	// The replication settings
	Replication ReplicationSpec `json:"replication"`

	// The erasure code setteings
	ErasureCoding ErasureCodeSpec `json:"erasureCode"`
}

// ReplicationSpec specifies the number of replicas
type ReplicationSpec struct {
	// Number of copies per object in a replicated storage pool, including the object itself (required for replicated pool type)
	Size uint `json:"size"`
}

// ErasureCodeSpec specifies the erasure coding params
type ErasureCodeSpec struct {
	// Number of coding chunks per object in an erasure coded storage pool (required for erasure-coded pool type)
	CodingChunks uint `json:"codingChunks"`

	// Number of data chunks per object in an erasure coded storage pool (required for erasure-coded pool type)
	DataChunks uint `json:"dataChunks"`
}

// PlacementSpec is a set of Placement configurations for the rook cluster.
type PlacementSpec struct {
	All k8sutil.Placement `json:"all,omitempty"`
	API k8sutil.Placement `json:"api,omitempty"`
	MDS k8sutil.Placement `json:"mds,omitempty"`
	MON k8sutil.Placement `json:"mon,omitempty"`
	OSD k8sutil.Placement `json:"osd,omitempty"`
	RGW k8sutil.Placement `json:"rgw,omitempty"`
}

// GetAPI returns the placement for the API service
func (p PlacementSpec) GetAPI() k8sutil.Placement { return p.All.Merge(p.API) }

// GetMDS returns the placement for the MDS service
func (p PlacementSpec) GetMDS() k8sutil.Placement { return p.All.Merge(p.MDS) }

// GetMON returns the placement for the MON service
func (p PlacementSpec) GetMON() k8sutil.Placement { return p.All.Merge(p.MON) }

// GetOSD returns the placement for the OSD service
func (p PlacementSpec) GetOSD() k8sutil.Placement { return p.All.Merge(p.OSD) }

// GetRGW returns the placement for the RGW service
func (p PlacementSpec) GetRGW() k8sutil.Placement { return p.All.Merge(p.RGW) }
