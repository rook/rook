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

package mon

import (
	"encoding/json"
	"reflect"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func PredicateMonEndpointChanges() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			cmNew, ok := e.ObjectNew.(*corev1.ConfigMap)
			if !ok {
				return false
			}
			cmOld, ok := e.ObjectOld.(*corev1.ConfigMap)
			if !ok {
				return false
			}
			if cmNew.GetName() == EndpointConfigMapName {
				if wereMonEndpointsUpdated(cmOld.Data, cmNew.Data) {
					logger.Info("monitor endpoints changed, updating the bootstrap peer token")
					return true
				}
			}
			return false
		},
	}
}

func wereMonEndpointsUpdated(oldCMData, newCMData map[string]string) bool {
	// Check the mapping key first
	if oldMapping, ok := oldCMData["mapping"]; ok {
		if newMapping, ok := newCMData["mapping"]; ok {
			// Unmarshal both into a type
			var oldMappingToGo Mapping
			err := json.Unmarshal([]byte(oldMapping), &oldMappingToGo)
			if err != nil {
				logger.Debugf("failed to unmarshal new. %v", err)
				return false
			}

			var newMappingToGo Mapping
			err = json.Unmarshal([]byte(newMapping), &newMappingToGo)
			if err != nil {
				logger.Debugf("failed to unmarshal new. %v", err)
				return false
			}

			// If length is different, monitors are different
			if len(oldMappingToGo.Schedule) != len(newMappingToGo.Schedule) {
				logger.Debugf("mons were added or removed from the endpoints cm")
				return true
			}
			// Since Schedule is map, it's unordered, so let's order it
			oldKeys := make([]string, 0, len(oldMappingToGo.Schedule))
			for k := range oldMappingToGo.Schedule {
				oldKeys = append(oldKeys, k)
			}
			sort.Strings(oldKeys)

			newKeys := make([]string, 0, len(newMappingToGo.Schedule))
			for k := range oldMappingToGo.Schedule {
				newKeys = append(newKeys, k)
			}
			sort.Strings(newKeys)

			// Iterate over the map and compare the values
			for _, v := range oldKeys {
				if !reflect.DeepEqual(oldMappingToGo.Schedule[v], newMappingToGo.Schedule[v]) {
					logger.Debugf("oldMappingToGo.Schedule[v] AND newMappingToGo.Schedule[v]: %v | %v", oldMappingToGo.Schedule[v], newMappingToGo.Schedule[v])
					return true
				}
			}
		}
	}

	return false
}
