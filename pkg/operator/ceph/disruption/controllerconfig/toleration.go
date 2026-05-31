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

package controllerconfig

import (
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// TolerationSet is a set of unique tolerations.
type TolerationSet struct {
	tolerations map[string]corev1.Toleration
}

// Add adds a toleration to the TolerationSet
func (t *TolerationSet) Add(toleration corev1.Toleration) {
	key := getKey(toleration)
	if len(t.tolerations) == 0 {
		t.tolerations = make(map[string]corev1.Toleration)
	}
	t.tolerations[key] = toleration
}

func getKey(toleration corev1.Toleration) string {
	return fmt.Sprintf("%s-%s-%s-%s", toleration.Key, toleration.Operator, toleration.Effect, toleration.Value)
}

// ToList returns a list of all tolerations in the set. The order will always be the same for the same set.
func (t *TolerationSet) ToList() []corev1.Toleration {
	tolerationList := make([]corev1.Toleration, 0)
	for _, toleration := range t.tolerations {
		tolerationList = append(tolerationList, toleration)
	}
	sort.SliceStable(tolerationList, func(i, j int) bool {
		a := getKey(tolerationList[i])
		b := getKey(tolerationList[j])
		return strings.Compare(a, b) == -1
	})
	return tolerationList
}
