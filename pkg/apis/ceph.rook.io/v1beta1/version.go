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
package v1beta1

const (
	Luminous             = "luminous"
	Mimic                = "mimic"
	Nautilus             = "nautilus"
	DefaultLuminousImage = "ceph/ceph:v12.2.9-20181026"
)

func VersionAtLeast(version, minimumVersion string) bool {
	orderedVersions := []string{Luminous, Mimic, Nautilus}
	found := false
	for _, v := range orderedVersions {
		if v == minimumVersion {
			found = true
		}
		if v == version {
			// if we found the matching version, the version is at least the minimum version if found==true
			return found
		}
	}
	return false
}
