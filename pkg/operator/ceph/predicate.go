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
	"context"

	"github.com/rook/rook/pkg/operator/ceph/controller"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// predicateOpController is the predicate function to trigger reconcile on operator configuration cm change
func predicateController(ctx context.Context, client client.Client) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if cm, ok := e.Object.(*v1.ConfigMap); ok {
				return cm.Name == controller.OperatorSettingConfigMapName
			}
			return false
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			if old, ok := e.ObjectOld.(*v1.ConfigMap); ok {
				if new, ok := e.ObjectNew.(*v1.ConfigMap); ok {
					if old.Name == controller.OperatorSettingConfigMapName && new.Name == controller.OperatorSettingConfigMapName {
						if old.Data["ROOK_CURRENT_NAMESPACE_ONLY"] != new.Data["ROOK_CURRENT_NAMESPACE_ONLY"] {
							logger.Debug("ROOK_CURRENT_NAMESPACE_ONLY config updated, reloading the manager")
							controller.ReloadManager()

							// No need to ask for reconciliation since the context is going to be terminated when
							// the signal is caught and the reconcile will run when the controller starts.
							return false
						}

						// We still want to reconcile the operator manager if the configmap is updated
						return true
					}
				}
			}

			return false
		},

		DeleteFunc: func(e event.DeleteEvent) bool {
			if cm, ok := e.Object.(*v1.ConfigMap); ok {
				if cm.Name == controller.OperatorSettingConfigMapName {
					logger.Debug("operator configmap deleted, not reconciling")
					return false
				}
			}

			return false
		},

		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}
