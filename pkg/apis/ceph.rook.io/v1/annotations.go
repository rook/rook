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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AnnotationsSpec is the main spec annotation for all daemons
// +kubebuilder:pruning:PreserveUnknownFields
// +nullable
type AnnotationsSpec map[KeyType]Annotations

// Annotations are annotations
type Annotations map[string]string

func (a AnnotationsSpec) All() Annotations {
	return a[KeyAll]
}

// GetMgrAnnotations returns the Annotations for the MGR service
func GetMgrAnnotations(a AnnotationsSpec) Annotations {
	return mergeAllAnnotationsWithKey(a, KeyMgr)
}

// GetDashboardAnnotations returns the Annotations for the Dashboard service
func GetDashboardAnnotations(a AnnotationsSpec) Annotations {
	return mergeAllAnnotationsWithKey(a, KeyDashboard)
}

// GetMonAnnotations returns the Annotations for the MON service
func GetMonAnnotations(a AnnotationsSpec) Annotations {
	return mergeAllAnnotationsWithKey(a, KeyMon)
}

// GetKeyRotationAnnotations returns the annotations for the key rotation job
func GetKeyRotationAnnotations(a AnnotationsSpec) Annotations {
	return mergeAllAnnotationsWithKey(a, KeyRotation)
}

// GetOSDPrepareAnnotations returns the annotations for the OSD service
func GetOSDPrepareAnnotations(a AnnotationsSpec) Annotations {
	return mergeAllAnnotationsWithKey(a, KeyOSDPrepare)
}

// GetOSDAnnotations returns the annotations for the OSD service
func GetOSDAnnotations(a AnnotationsSpec) Annotations {
	return mergeAllAnnotationsWithKey(a, KeyOSD)
}

// GetCleanupAnnotations returns the Annotations for the cleanup job
func GetCleanupAnnotations(a AnnotationsSpec) Annotations {
	return mergeAllAnnotationsWithKey(a, KeyCleanup)
}

// GetCephExporterAnnotations returns the Annotations for the MGR service
func GetCephExporterAnnotations(a AnnotationsSpec) Annotations {
	return mergeAllAnnotationsWithKey(a, KeyCephExporter)
}

func GetClusterMetadataAnnotations(a AnnotationsSpec) Annotations {
	return a[KeyClusterMetadata]
}

func mergeAllAnnotationsWithKey(a AnnotationsSpec, name KeyType) Annotations {
	all := a.All()
	if all != nil {
		return all.Merge(a[name])
	}
	return a[name]
}

// ApplyToObjectMeta adds annotations to object meta unless the keys are already defined.
func (a Annotations) ApplyToObjectMeta(t *metav1.ObjectMeta) {
	if t.Annotations == nil {
		t.Annotations = map[string]string{}
	}
	for k, v := range a {
		if _, ok := t.Annotations[k]; !ok {
			t.Annotations[k] = v
		}
	}
}

// Merge returns an Annotations which results from merging the attributes of the
// original Annotations with the attributes of the supplied one. The supplied
// Annotation attributes will override the original ones if defined.
func (a Annotations) Merge(with map[string]string) Annotations {
	ret := a
	if ret == nil {
		ret = map[string]string{}
	}
	for k, v := range with {
		if _, ok := ret[k]; !ok {
			ret[k] = v
		}
	}
	return ret
}
