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

package enums

import (
	"fmt"
	"strings"
)

type K8sVersion int

const (
	V1dot5 K8sVersion = iota + 1
	V1dot6
)

var versions = [...]string{
	"v1.5",
	"v1.6",
	"None",
}

func (version K8sVersion) String() string {
	return versions[version-1]
}

func GetK8sVersionFromString(name string) (K8sVersion, error) {
	switch {
	case strings.EqualFold(name, V1dot6.String()):
		return V1dot6, nil
	default:
		return V1dot5, fmt.Errorf("Unsupported Kubernetes version: " + name)
	}
}
