/*
Copyright 2021 The Rook Authors. All rights reserved.

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
package multus

import (
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/json"
)

// This is part of multus utility
type multusNetConfiguration struct {
	NetworkName   string   `json:"name"`
	InterfaceName string   `json:"interface"`
	Ips           []string `json:"ips"`
}

// This is a multus utility
func getMultusConfs(pod *corev1.Pod) ([]multusNetConfiguration, error) {
	var multusConfs []multusNetConfiguration
	if val, ok := pod.ObjectMeta.Annotations[networksAnnotation]; ok {
		err := json.Unmarshal([]byte(val), &multusConfs)
		if err != nil {
			return multusConfs, errors.Wrap(err, "failed to unmarshal json")
		}
		return multusConfs, nil
	}
	return multusConfs, errors.Errorf("failed to find multus annotation for pod %q in namespace %q", pod.ObjectMeta.Name, pod.ObjectMeta.Namespace)
}

// This is a multus utility
func findMultusInterfaceName(multusConfs []multusNetConfiguration, multusName, multusNamespace string) (string, error) {

	// The network name includes its namespace.
	multusNetwork := fmt.Sprintf("%s/%s", multusNamespace, multusName)

	for _, multusConf := range multusConfs {
		if multusConf.NetworkName == multusNetwork {
			return multusConf.InterfaceName, nil
		}
	}
	return "", errors.New("failed to find multus network configuration")
}
