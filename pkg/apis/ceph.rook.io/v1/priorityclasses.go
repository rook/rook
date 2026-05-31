/*
Copyright 2019 The Rook Authors. All rights reserved.

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

// All returns the priority class name defined for 'all' daemons in the Ceph cluster CRD.
func (p PriorityClassNamesSpec) All() string {
	if val, ok := p[KeyAll]; ok {
		return val
	}
	return ""
}

// GetMgrPriorityClassName returns the priority class name for the MGR service
func GetMgrPriorityClassName(p PriorityClassNamesSpec) string {
	if _, ok := p[KeyMgr]; !ok {
		return p.All()
	}
	return p[KeyMgr]
}

// GetMonPriorityClassName returns the priority class name for the monitors
func GetMonPriorityClassName(p PriorityClassNamesSpec) string {
	if _, ok := p[KeyMon]; !ok {
		return p.All()
	}
	return p[KeyMon]
}

// GetOSDPriorityClassName returns the priority class name for the OSDs
func GetOSDPriorityClassName(p PriorityClassNamesSpec) string {
	if _, ok := p[KeyOSD]; !ok {
		return p.All()
	}
	return p[KeyOSD]
}

// GetCleanupPriorityClassName returns the priority class name for the cleanup job
func GetCleanupPriorityClassName(p PriorityClassNamesSpec) string {
	if _, ok := p[KeyCleanup]; !ok {
		return p.All()
	}
	return p[KeyCleanup]
}

// GetCrashCollectorPriorityClassName returns the priority class name for the crashcollector
func GetCrashCollectorPriorityClassName(p PriorityClassNamesSpec) string {
	if _, ok := p[KeyCrashCollector]; !ok {
		return p.All()
	}
	return p[KeyCrashCollector]
}

// GetCephExporterPriorityClassName returns the priority class name for the ceph-exporter
func GetCephExporterPriorityClassName(p PriorityClassNamesSpec) string {
	if _, ok := p[KeyCephExporter]; !ok {
		return p.All()
	}
	return p[KeyCephExporter]
}
