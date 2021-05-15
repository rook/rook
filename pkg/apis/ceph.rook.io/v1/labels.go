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
	rook "github.com/rook/rook/pkg/apis/rook.io"
)

// LabelsSpec is the main spec label for all daemons
type LabelsSpec map[rook.KeyType]rook.Labels

func (a LabelsSpec) All() rook.Labels {
	return a[KeyAll]
}

// GetMgrLabels returns the Labels for the MGR service
func GetMgrLabels(a LabelsSpec) rook.Labels {
	return mergeAllLabelsWithKey(a, KeyMgr)
}

// GetMonLabels returns the Labels for the MON service
func GetMonLabels(a LabelsSpec) rook.Labels {
	return mergeAllLabelsWithKey(a, KeyMon)
}

// GetOSDPrepareLabels returns the Labels for the OSD prepare job
func GetOSDPrepareLabels(a LabelsSpec) rook.Labels {
	return mergeAllLabelsWithKey(a, KeyOSDPrepare)
}

// GetOSDLabels returns the Labels for the OSD service
func GetOSDLabels(a LabelsSpec) rook.Labels {
	return mergeAllLabelsWithKey(a, KeyOSD)
}

// GetCleanupLabels returns the Labels for the cleanup job
func GetCleanupLabels(a LabelsSpec) rook.Labels {
	return mergeAllLabelsWithKey(a, KeyCleanup)
}

// GetMonitoringLabels returns the Labels for monitoring resources
func GetMonitoringLabels(a LabelsSpec) rook.Labels {
	return mergeAllLabelsWithKey(a, KeyMonitoring)
}

func mergeAllLabelsWithKey(a LabelsSpec, name rook.KeyType) rook.Labels {
	all := a.All()
	if all != nil {
		return all.Merge(a[name])
	}
	return a[name]
}
