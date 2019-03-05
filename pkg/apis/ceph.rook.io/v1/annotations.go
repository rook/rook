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

// GetMgrAnnotations returns the Annotations for the MGR service
func GetMgrAnnotations(a rook.AnnotationsSpec) rook.Annotations {
	return a.All().Merge(a[KeyMgr])
}

// GetMonAnnotations returns the Annotations for the MON service
func GetMonAnnotations(a rook.AnnotationsSpec) rook.Annotations {
	return a.All().Merge(a[KeyMon])
}

// GetOSDAnnotations returns the annotations for the OSD service
func GetOSDAnnotations(a rook.AnnotationsSpec) rook.Annotations {
	return a.All().Merge(a[KeyOSD])
}

// GetRGWAnnotations returns the Annotations for the RBD mirrors
func GetRGWAnnotations(a rook.AnnotationsSpec) rook.Annotations {
	return a.All().Merge(a[KeyRGWMirror])
}

// GetRBDMirrorAnnotations returns the Annotations for the RBD mirrors
func GetRBDMirrorAnnotations(a rook.AnnotationsSpec) rook.Annotations {
	return a.All().Merge(a[KeyRBDMirror])
}
