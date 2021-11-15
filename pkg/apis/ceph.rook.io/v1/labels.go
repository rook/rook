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

// GetCrashCollectorLabels returns the Labels for the crash collector resources
func GetCrashCollectorLabels(a LabelsSpec) rook.Labels {
	return mergeAllLabelsWithKey(a, KeyCrashCollector)
}

func mergeAllLabelsWithKey(a LabelsSpec, name rook.KeyType) rook.Labels {
	all := a.All()
	if all != nil {
		return all.Merge(a[name])
	}
	return a[name]
}
<<<<<<< HEAD
=======

// ApplyToObjectMeta adds labels to object meta unless the keys are already defined.
func (a Labels) ApplyToObjectMeta(t *metav1.ObjectMeta) {
	if t.Labels == nil {
		t.Labels = map[string]string{}
	}
	for k, v := range a {
		if _, ok := t.Labels[k]; !ok {
			t.Labels[k] = v
		}
	}
}

// OverwriteApplyToObjectMeta adds labels to object meta, overwriting keys that are already defined.
func (a Labels) OverwriteApplyToObjectMeta(t *metav1.ObjectMeta) {
	if t.Labels == nil {
		t.Labels = map[string]string{}
	}
	for k, v := range a {
		t.Labels[k] = v
	}
}

// Merge returns a Labels which results from merging the attributes of the
// original Labels with the attributes of the supplied one. The supplied
// Labels attributes will override the original ones if defined.
func (a Labels) Merge(with Labels) Labels {
	ret := Labels{}
	for k, v := range a {
		if _, ok := ret[k]; !ok {
			ret[k] = v
		}
	}
	for k, v := range with {
		if _, ok := ret[k]; !ok {
			ret[k] = v
		}
	}
	return ret
}
>>>>>>> 00a5debba (monitoring: allow overriding monitoring labels)
