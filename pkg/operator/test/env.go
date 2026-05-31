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

package test

import (
	"fmt"

	"k8s.io/api/core/v1"
)

// GetEnv finds returns the env var with the given name from a list of env vars.
func GetEnv(name string, envs []v1.EnvVar) (*v1.EnvVar, error) {
	for _, e := range envs {
		if e.Name == name {
			return &e, nil
		}
	}
	return &v1.EnvVar{Name: "EnvVar was not found by GetEnv()"},
		fmt.Errorf("volume mount %s does not exist in %+v", name, envs)
}
