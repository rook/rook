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

package agent

import (
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// predicateController is the predicate function to trigger reconcile on operator configuration cm change
func predicateController() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if cm, ok := e.Object.(*v1.ConfigMap); ok {
				return cm.Name == opcontroller.OperatorSettingConfigMapName
			}

			return false
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			if old, ok := e.ObjectOld.(*v1.ConfigMap); ok {
				if new, ok := e.ObjectNew.(*v1.ConfigMap); ok {
					if old.Name == opcontroller.OperatorSettingConfigMapName && new.Name == opcontroller.OperatorSettingConfigMapName {
						return old.Data["ROOK_ENABLE_FLEX_DRIVER"] != new.Data["ROOK_ENABLE_FLEX_DRIVER"]
					}
				}
			}

			return false
		},

		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},

		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}
