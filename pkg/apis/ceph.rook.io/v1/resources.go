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
	rook "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	v1 "k8s.io/api/core/v1"
)

const (
	// ResourcesKeyMon represents the name of resource in the CR for a mon
	ResourcesKeyMon = "mon"
	// ResourcesKeyMgr represents the name of resource in the CR for a mgr
	ResourcesKeyMgr = "mgr"
	// ResourcesKeyOSD represents the name of resource in the CR for an osd
	ResourcesKeyOSD = "osd"
	// ResourcesKeyPrepareOSD represents the name of resource in the CR for the osd prepare job
	ResourcesKeyPrepareOSD = "prepareosd"
	// ResourcesKeyRBDMirror represents the name of resource in the CR for the rbdmirror
	ResourcesKeyRBDMirror = "rbdmirror"
	// ResourcesKeyCrashCollector represents the name of resource in the CR for the crash
	ResourcesKeyCrashCollector = "crashcollector"
)

// GetMgrResources returns the placement for the MGR service
func GetMgrResources(p rook.ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyMgr]
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

// GetRBDMirrorResources returns the placement for the RBD Mirrors
func GetRBDMirrorResources(p rook.ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyRBDMirror]
}

// GetCrashCollectorResources returns the placement for the crash daemon
func GetCrashCollectorResources(p rook.ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyCrashCollector]
}
