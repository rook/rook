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

package bucket

import (
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const rookOBCWatchOperatorNamespace = "ROOK_OBC_WATCH_OPERATOR_NAMESPACE"

// predicateController is the predicate function to trigger reconcile on operator configuration cm change
func predicateController() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// if the operator configuration file is created we want to reconcile
			if cm, ok := e.Object.(*v1.ConfigMap); ok {
				// It's probably fine to use the Generation value here. The case where the operator was stopped and the
				// ConfigMap was created is low since the cm is always present these days
				return cm.Name == controller.OperatorSettingConfigMapName && cm.Generation == 1
			}

			// If a Ceph Cluster is created we want to reconcile the bucket provisioner
			if _, ok := e.Object.(*cephv1.CephCluster); ok {
				// Always return true, so when the controller starts we reconcile too. We don't get
				return true
			}

			return false
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			if old, ok := e.ObjectOld.(*v1.ConfigMap); ok {
				if new, ok := e.ObjectNew.(*v1.ConfigMap); ok {
					if old.Name == controller.OperatorSettingConfigMapName && new.Name == controller.OperatorSettingConfigMapName {
						if old.Data[rookOBCWatchOperatorNamespace] != new.Data[rookOBCWatchOperatorNamespace] {
							logger.Infof("%s changed. reconciling bucket controller", rookOBCWatchOperatorNamespace)

							// We reload the manager so that the controller restarts and goes
							// through the CreateFunc again. Then the CephCluster watcher will be triggered
							controller.ReloadManager()
						}
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
