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
	rook "github.com/rook/rook/pkg/apis/rook.io/v1"
	v1 "k8s.io/api/core/v1"
)

const (
	// ResourcesKeyMon represents the name of resource in the CR for a mon
	ResourcesKeyMon = "mon"
	// ResourcesKeyMgr represents the name of resource in the CR for a mgr
	ResourcesKeyMgr = "mgr"
	// ResourcesKeyMgrSidecar represents the name of resource in the CR for a mgr
	ResourcesKeyMgrSidecar = "mgr-sidecar"
	// ResourcesKeyOSD represents the name of resource in the CR for an osd
	ResourcesKeyOSD = "osd"
	// ResourcesKeyPrepareOSD represents the name of resource in the CR for the osd prepare job
	ResourcesKeyPrepareOSD = "prepareosd"
	// ResourcesKeyMDS represents the name of resource in the CR for the mds
	ResourcesKeyMDS = "mds"
	// ResourcesKeyCrashCollector represents the name of resource in the CR for the crash
	ResourcesKeyCrashCollector = "crashcollector"
	// ResourcesKeyLogCollector represents the name of resource in the CR for the log
	ResourcesKeyLogCollector = "logcollector"
	// ResourcesKeyRBDMirror represents the name of resource in the CR for the rbd mirror
	ResourcesKeyRBDMirror = "rbdmirror"
	// ResourcesKeyFilesystemMirror represents the name of resource in the CR for the filesystem mirror
	ResourcesKeyFilesystemMirror = "fsmirror"
	// ResourcesKeyCleanup represents the name of resource in the CR for the cleanup
	ResourcesKeyCleanup = "cleanup"
)

// GetMgrResources returns the placement for the MGR service
func GetMgrResources(p rook.ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyMgr]
}

// GetMgrSidecarResources returns the placement for the MGR sidecar container
func GetMgrSidecarResources(p rook.ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyMgrSidecar]
}

// GetMonResources returns the placement for the monitors
func GetMonResources(p rook.ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyMon]
}

// GetOSDResources returns the placement for the OSDs
func GetOSDResources(p rook.ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyOSD]
}

// GetPrepareOSDResources returns the placement for the OSDs prepare job
func GetPrepareOSDResources(p rook.ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyPrepareOSD]
}

// GetCrashCollectorResources returns the placement for the crash daemon
func GetCrashCollectorResources(p rook.ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyCrashCollector]
}

// GetLogCollectorResources returns the placement for the crash daemon
func GetLogCollectorResources(p rook.ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyCrashCollector]
}

// GetCleanupResources returns the placement for the cleanup job
func GetCleanupResources(p rook.ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyCleanup]
}
