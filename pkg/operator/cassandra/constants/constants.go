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

package constants

// These labels are only used on the ClusterIP services
// acting as each member's identity (static ip).
// Each of these labels is a record of intent to do
// something. The controller sets these labels and each
// member watches for them and takes the appropriate
// actions.
//
// See the sidecar design doc for more details.
const (
	// SeedLabel determines if a member is a seed or not.
	SeedLabel = "cassandra.rook.io/seed"

	// DecommissionLabel expresses the intent to decommission
	// the specific member. The presence of the label expresses
	// the intent to decommission. If the value is true, it means
	// the member has finished decommissioning.
	// Values: {true, false}
	DecommissionLabel = "cassandra.rook.io/decommissioned"

	// DeveloperModeAnnotation is present when the user wishes
	// to bypass production-readiness checks and start the database
	// either way. Currently useful for scylla, may get removed
	// once configMapName field is implemented in Cluster CRD.
	DeveloperModeAnnotation = "cassandra.rook.io/developer-mode"

	LabelValueTrue  = "true"
	LabelValueFalse = "false"
)

// Generic Labels used on objects created by the operator.
const (
	ClusterNameLabel    = "cassandra.rook.io/cluster"
	DatacenterNameLabel = "cassandra.rook.io/datacenter"
	RackNameLabel       = "cassandra.rook.io/rack"

	AppName         = "rook-cassandra"
	OperatorAppName = "rook-cassandra-operator"
)

// Environment Variable Names
const (
	PodIPEnvVar = "POD_IP"

	ResourceLimitCPUEnvVar    = "CPU_LIMIT"
	ResourceLimitMemoryEnvVar = "MEMORY_LIMIT"
)

// Configuration Values
const (
	SharedDirName = "/mnt/shared"
	PluginDirName = SharedDirName + "/" + "plugins"

	DataDirCassandra = "/var/lib/cassandra"
	DataDirScylla    = "/var/lib/scylla"

	JolokiaJarName = "jolokia.jar"
	JolokiaPort    = 8778
	JolokiaContext = "jolokia"

	ReadinessProbePath = "/readyz"
	LivenessProbePath  = "/healthz"
	ProbePort          = 8080
)
