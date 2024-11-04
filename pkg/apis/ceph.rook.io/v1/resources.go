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
	v1 "k8s.io/api/core/v1"
)

const (
	// ResourcesKeyMon represents the name of resource in the CR for a mon
	ResourcesKeyMon = "mon"
	// ResourcesKeyMgr represents the name of resource in the CR for a mgr
	ResourcesKeyMgr = "mgr"
	// ResourcesKeyMgrSidecar represents the name of resource in the CR for a mgr
	ResourcesKeyMgrSidecar = "mgr-sidecar"
	// ResourcesKeyOSD represents the name of a resource in the CR for all OSDs
	ResourcesKeyOSD = "osd"
	// ResourcesKeyPrepareOSD represents the name of resource in the CR for the osd prepare job
	ResourcesKeyPrepareOSD = "prepareosd"
	// ResourcesKeyCmdReporter represents the name of resource in the CR for the detect version and network jobs
	ResourcesKeyCmdReporter = "cmd-reporter"
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
	// ResourcesKeyCleanup represents the name of resource in the CR for ceph-exporter
	ResourcesKeyCephExporter = "exporter"
)

// GetMgrResources returns the resources for the MGR service
func GetMgrResources(p ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyMgr]
}

// GetMgrSidecarResources returns the resources for the MGR sidecar container
func GetMgrSidecarResources(p ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyMgrSidecar]
}

// GetMonResources returns the resources for the monitors
func GetMonResources(p ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyMon]
}

// GetOSDResources returns the resources for all OSDs or for OSDs of specified device class (hdd, nvme, ssd)
func GetOSDResources(p ResourceSpec, deviceClass string) v1.ResourceRequirements {
	if deviceClass == "" {
		return p[ResourcesKeyOSD]
	}
	// if device class specified, but not set in requirements return common osd requirements if present
	r, ok := p[getOSDResourceKeyForDeviceClass(deviceClass)]
	if ok {
		return r
	}
	return p[ResourcesKeyOSD]
}

// getOSDResourceKeyForDeviceClass returns key name for device class in resources spec
func getOSDResourceKeyForDeviceClass(deviceClass string) string {
	return ResourcesKeyOSD + "-" + deviceClass
}

// GetPrepareOSDResources returns the resources for the OSDs prepare job
func GetPrepareOSDResources(p ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyPrepareOSD]
}

// GetCmdReporterResources returns the resources for the detect version job
func GetCmdReporterResources(p ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyCmdReporter]
}

// GetCrashCollectorResources returns the resources for the crash daemon
func GetCrashCollectorResources(p ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyCrashCollector]
}

// GetLogCollectorResources returns the resources for the logo collector
func GetLogCollectorResources(p ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyLogCollector]
}

// GetCleanupResources returns the resources for the cleanup job
func GetCleanupResources(p ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyCleanup]
}

// GetCephExporterResources returns the resources for the cleanup job
func GetCephExporterResources(p ResourceSpec) v1.ResourceRequirements {
	return p[ResourcesKeyCephExporter]
}
