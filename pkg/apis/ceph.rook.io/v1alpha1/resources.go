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
package v1alpha1

import (
	rook "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"k8s.io/api/core/v1"
)

const (
	ResourcesKeyMgr = "mgr"
	ResourcesKeyMon = "mon"
	ResourcesKeyOSD = "osd"
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
