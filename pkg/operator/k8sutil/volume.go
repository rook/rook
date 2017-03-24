/*
Copyright 2016 The Rook Authors. All rights reserved.

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
package k8sutil

import (
	"path/filepath"
	"strings"
)

func PathToVolumeName(path string) string {
	// kubernetes volume names must match this regex: [a-z0-9]([-a-z0-9]*[a-z0-9])?

	// first replace all filepath separators with hyphens
	volumeName := strings.Replace(path, string(filepath.Separator), "-", -1)

	// trim any leading/trailing hyphens
	volumeName = strings.TrimPrefix(volumeName, "-")
	volumeName = strings.TrimSuffix(volumeName, "-")

	return volumeName
}
