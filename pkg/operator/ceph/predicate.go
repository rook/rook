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

// Package operator to manage Kubernetes storage.
package operator

import (
	"github.com/rook/rook/pkg/operator/ceph/controller"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// predicateController is the predicate function to trigger reconcile on operator configuration cm change
func operatorSettingConfigMapPredicate[T *corev1.ConfigMap]() predicate.TypedFuncs[T] {
	return predicate.TypedFuncs[T]{
		CreateFunc: func(e event.TypedCreateEvent[T]) bool {
			obj := (*corev1.ConfigMap)(e.Object)

			return obj.GetName() == controller.OperatorSettingConfigMapName
		},

		UpdateFunc: func(e event.TypedUpdateEvent[T]) bool {
			objOld := (*corev1.ConfigMap)(e.ObjectOld)
			objNew := (*corev1.ConfigMap)(e.ObjectNew)

			if objOld.GetName() == controller.OperatorSettingConfigMapName && objNew.GetName() == controller.OperatorSettingConfigMapName {
				if objOld.Data["ROOK_CURRENT_NAMESPACE_ONLY"] != objNew.Data["ROOK_CURRENT_NAMESPACE_ONLY"] {
					logger.Debug("ROOK_CURRENT_NAMESPACE_ONLY config updated, reloading the manager")
					controller.ReloadManager()

					// No need to ask for reconciliation since the context is going to be terminated when
					// the signal is caught and the reconcile will run when the controller starts.
					return false
				}

				// We still want to reconcile the operator manager if the configmap is updated
				return true
			}

			return false
		},

		DeleteFunc: func(e event.TypedDeleteEvent[T]) bool {
			obj := (*corev1.ConfigMap)(e.Object)

			if obj.GetName() == controller.OperatorSettingConfigMapName {
				logger.Debug("operator configmap deleted, not reconciling")
				return false
			}

			return false
		},

		GenericFunc: func(e event.TypedGenericEvent[T]) bool {
			return false
		},
	}
}
