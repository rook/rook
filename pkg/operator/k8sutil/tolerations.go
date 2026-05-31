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

package k8sutil

import (
	"encoding/json"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// This function takes raw YAML string and converts it to Kubernetes Tolerations array
func YamlToTolerations(raw string) ([]v1.Toleration, error) {
	if raw == "" {
		return []v1.Toleration{}, nil
	}

	rawJSON, err := yaml.ToJSON([]byte(raw))
	if err != nil {
		return []v1.Toleration{}, err
	}

	var tolerations []v1.Toleration
	err = json.Unmarshal(rawJSON, &tolerations)
	if err != nil {
		return []v1.Toleration{}, err
	}

	return tolerations, nil
}
