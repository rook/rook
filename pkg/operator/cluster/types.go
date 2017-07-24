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

// Package cluster to manage a rook cluster.
package cluster

import (
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/api"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/mgr"
	"github.com/rook/rook/pkg/operator/mon"
	"github.com/rook/rook/pkg/operator/osd"
	"github.com/rook/rook/pkg/operator/rgw"
	rookclient "github.com/rook/rook/pkg/rook/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// schemeGroupVersion is group version used to register these objects
var schemeGroupVersion = schema.GroupVersion{Group: k8sutil.CustomResourceGroup, Version: k8sutil.V1Alpha1}

// Cluster represents an object of a Rook cluster
type Cluster struct {
	context           *clusterd.Context
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ClusterSpec `json:"spec"`
	mons              *mon.Cluster
	mgrs              *mgr.Cluster
	osds              *osd.Cluster
	apis              *api.Cluster
	rgws              *rgw.Cluster
	rookClient        rookclient.RookRestClient
	stopCh            chan struct{}
}

// ClusterList represents an object of a Rook cluster list
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Cluster `json:"items"`
}

// ClusterSpec represents an object of a Rook cluster spec
type ClusterSpec struct {
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
