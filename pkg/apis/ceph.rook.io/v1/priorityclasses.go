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

import (
	rook "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
)

// GetMgrPriorityClassName returns the priority class name for the MGR service
func GetMgrPriorityClassName(p rook.PriorityClassNamesSpec) string {
	if _, ok := p[KeyMgr]; !ok {
		return p.All()
	}
	return p[KeyMgr]
}

// GetMonPriorityClassName returns the priority class name for the monitors
func GetMonPriorityClassName(p rook.PriorityClassNamesSpec) string {
	if _, ok := p[KeyMon]; !ok {
		return p.All()
	}
	return p[KeyMon]
}

// GetOSDPriorityClassName returns the priority class name for the OSDs
func GetOSDPriorityClassName(p rook.PriorityClassNamesSpec) string {
	if _, ok := p[KeyOSD]; !ok {
		return p.All()
	}
	return p[KeyOSD]
}

// GetRBDMirrorPriorityClassName returns the priority class name for the RBD Mirrors
func GetRBDMirrorPriorityClassName(p rook.PriorityClassNamesSpec) string {
	if _, ok := p[KeyRBDMirror]; !ok {
		return p.All()
	}
	return p[KeyRBDMirror]
}
