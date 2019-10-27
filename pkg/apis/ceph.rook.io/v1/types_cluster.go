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

package v1

import (
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rook "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
)

// ***************************************************************************
// IMPORTANT FOR CODE GENERATION
// If the types in this file are updated, you will need to run
// `make codegen` to generate the new types under the client/clientset folder.
// ***************************************************************************

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CephCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ClusterSpec   `json:"spec"`
	Status            ClusterStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CephClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephCluster `json:"items"`
}

type ClusterSpec struct {
	// The version information that instructs Rook to orchestrate a particular version of Ceph.
	CephVersion CephVersionSpec `json:"cephVersion,omitempty"`

	// A spec for available storage in the cluster and how it should be used
	Storage rook.StorageScopeSpec `json:"storage,omitempty"`

	// The annotations-related configuration to add/set on each Pod related object.
	Annotations rook.AnnotationsSpec `json:"annotations,omitempty"`

	// The placement-related configuration to pass to kubernetes (affinity, node selector, tolerations).
	Placement rook.PlacementSpec `json:"placement,omitempty"`

	// Network related configuration
	Network NetworkSpec `json:"network,omitempty"`

	// Resources set resource requests and limits
	Resources rook.ResourceSpec `json:"resources,omitempty"`

	// The path on the host where config and data can be persisted.
	DataDirHostPath string `json:"dataDirHostPath,omitempty"`

	// SkipUpgradeChecks defines if an upgrade should be forced even if one of the check fails
	SkipUpgradeChecks bool `json:"skipUpgradeChecks,omitempty"`

	// A spec for configuring disruption management.
	DisruptionManagement DisruptionManagementSpec `json:"disruptionManagement,omitempty"`

	// A spec for mon related options
	Mon MonSpec `json:"mon,omitempty"`

	// A spec for rbd mirroring
	RBDMirroring RBDMirroringSpec `json:"rbdMirroring"`

	// Dashboard settings
	Dashboard DashboardSpec `json:"dashboard,omitempty"`

	// Prometheus based Monitoring settings
	Monitoring MonitoringSpec `json:"monitoring,omitempty"`

	// Whether the Ceph Cluster is running external to this Kubernetes cluster
	// mon, mgr, osd, mds, and discover daemons will not be created for external clusters.
	External ExternalSpec `json:"external"`

	// A spec for mgr related options
	Mgr MgrSpec `json:"mgr,omitempty"`

	// Remove the OSD that is out and safe to remove only if this option is true
	RemoveOSDsIfOutAndSafeToRemove bool `json:"removeOSDsIfOutAndSafeToRemove"`
}

// VersionSpec represents the settings for the Ceph version that Rook is orchestrating.
type CephVersionSpec struct {
	// Image is the container image used to launch the ceph daemons, such as ceph/ceph:v13.2.6 or ceph/ceph:v14.2.2
	Image string `json:"image,omitempty"`

	// Whether to allow unsupported versions (do not set to true in production)
	AllowUnsupported bool `json:"allowUnsupported,omitempty"`
}

// DashboardSpec represents the settings for the Ceph dashboard
type DashboardSpec struct {
	// Whether to enable the dashboard
	Enabled bool `json:"enabled,omitempty"`
	// A prefix for all URLs to use the dashboard with a reverse proxy
	UrlPrefix string `json:"urlPrefix,omitempty"`
	// The dashboard webserver port
	Port int `json:"port,omitempty"`
	// Whether SSL should be used
	SSL bool `json:"ssl,omitempty"`
}

// MonitoringSpec represents the settings for Prometheus based Ceph monitoring
type MonitoringSpec struct {
	// Whether to create the prometheus rules for the ceph cluster. If true, the prometheus
	// types must exist or the creation will fail.
	Enabled bool `json:"enabled,omitempty"`

	// The namespace where the prometheus rules and alerts should be created.
	// If empty, the same namespace as the cluster will be used.
	RulesNamespace string `json:"rulesNamespace,omitempty"`
}

type ClusterStatus struct {
	State      ClusterState `json:"state,omitempty"`
	Message    string       `json:"message,omitempty"`
	CephStatus *CephStatus  `json:"ceph,omitempty"`
}

type CephStatus struct {
	Health         string                       `json:"health,omitempty"`
	Details        map[string]CephHealthMessage `json:"details,omitempty"`
	LastChecked    string                       `json:"lastChecked,omitempty"`
	LastChanged    string                       `json:"lastChanged,omitempty"`
	PreviousHealth string                       `json:"previousHealth,omitempty"`
}

type CephHealthMessage struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type ClusterState string

const (
	ClusterStateCreating   ClusterState = "Creating"
	ClusterStateCreated    ClusterState = "Created"
	ClusterStateUpdating   ClusterState = "Updating"
	ClusterStateConnecting ClusterState = "Connecting"
	ClusterStateConnected  ClusterState = "Connected"
	ClusterStateError      ClusterState = "Error"
	// DefaultFailureDomain for PoolSpec
	DefaultFailureDomain = "host"
)

type MonSpec struct {
	Count                int                       `json:"count,omitempty"`
	AllowMultiplePerNode bool                      `json:"allowMultiplePerNode,omitempty"`
	VolumeClaimTemplate  *v1.PersistentVolumeClaim `json:"volumeClaimTemplate,omitempty"`
}

// MgrSpec represents options to configure a ceph mgr
type MgrSpec struct {
	Modules []Module `json:"modules,omitempty"`
}

// Module represents mgr modules that the user wants to enable or disable
type Module struct {
	Name    string `json:"name,omitempty"`
	Enabled bool   `json:"enabled"`
}

// ExternalSpec represents the options supported by an external cluster
type ExternalSpec struct {
	Enable bool `json:"enable"`
}

type RBDMirroringSpec struct {
	Workers int `json:"workers"`
}

// NetworkSpec for Ceph includes backward compatibility code
type NetworkSpec struct {
	rook.NetworkSpec `json:",inline"`

	// HostNetwork to enable host network
	HostNetwork bool `json:"hostNetwork"`
}

// DisruptionManagementSpec configures management of daemon disruptions
type DisruptionManagementSpec struct {
	// This enables management of poddisruptionbudgets
	ManagePodBudgets bool `json:"managePodBudgets,omitempty"`
	// OSDMaintenanceTimeout sets how many additional minutes the DOWN/OUT interval is for drained failure domains
	// it only works if managePodBudgetss is true.
	// the default is 30 minutes
	OSDMaintenanceTimeout time.Duration `json:"osdMaintenanceTimeout,omitempty"`
	// This enables management of machinedisruptionbudgets
	ManageMachineDisruptionBudgets bool `json:"manageMachineDisruptionBudgets,omitempty"`
	// Namespace to look for MDBs by the machineDisruptionBudgetController
	MachineDisruptionBudgetNamespace string `json:"machineDisruptionBudgetNamespace,omitempty"`
}
