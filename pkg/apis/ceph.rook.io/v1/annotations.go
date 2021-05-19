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

// AnnotationsSpec is the main spec annotation for all daemons
// +kubebuilder:pruning:PreserveUnknownFields
// +nullable
type AnnotationsSpec map[rook.KeyType]rook.Annotations

func (a AnnotationsSpec) All() rook.Annotations {
	return a[KeyAll]
}

// GetMgrAnnotations returns the Annotations for the MGR service
func GetMgrAnnotations(a AnnotationsSpec) rook.Annotations {
	return mergeAllAnnotationsWithKey(a, KeyMgr)
}

// GetMonAnnotations returns the Annotations for the MON service
func GetMonAnnotations(a AnnotationsSpec) rook.Annotations {
	return mergeAllAnnotationsWithKey(a, KeyMon)
}

// GetOSDPrepareAnnotations returns the annotations for the OSD service
func GetOSDPrepareAnnotations(a AnnotationsSpec) rook.Annotations {
	return mergeAllAnnotationsWithKey(a, KeyOSDPrepare)
}

// GetOSDAnnotations returns the annotations for the OSD service
func GetOSDAnnotations(a AnnotationsSpec) rook.Annotations {
	return mergeAllAnnotationsWithKey(a, KeyOSD)
}

// GetCleanupAnnotations returns the Annotations for the cleanup job
func GetCleanupAnnotations(a AnnotationsSpec) rook.Annotations {
	return mergeAllAnnotationsWithKey(a, KeyCleanup)
}

func mergeAllAnnotationsWithKey(a AnnotationsSpec, name rook.KeyType) rook.Annotations {
	all := a.All()
	if all != nil {
		return all.Merge(a[name])
	}
	return a[name]
}
