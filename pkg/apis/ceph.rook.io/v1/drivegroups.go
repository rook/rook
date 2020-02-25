/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopy allows the map[string]interface{} type of DriveGroupSpec to have other DeepCopy-related
// code generated for it. The generator does not know how to copy arbitrary interface{} types
// without this.
func (d DriveGroupSpec) DeepCopy() DriveGroupSpec {
	return (DriveGroupSpec)(runtime.DeepCopyJSON(d))
}
