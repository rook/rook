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

package k8sutil

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ExpandPVCIfRequired will expand the PVC if requested size is greater than the actual size of existing PVC
func ExpandPVCIfRequired(client client.Client, desiredPVC *v1.PersistentVolumeClaim, currentPVC *v1.PersistentVolumeClaim) {
	desiredSize, desiredOK := desiredPVC.Spec.Resources.Requests[v1.ResourceStorage]
	currentSize, currentOK := currentPVC.Spec.Resources.Requests[v1.ResourceStorage]
	if !desiredOK || !currentOK {
		logger.Debugf("desired or current size are not specified for PVC %q", currentPVC.Name)
		return
	}
	if desiredSize.Value() > currentSize.Value() {
		currentPVC.Spec.Resources.Requests[v1.ResourceStorage] = desiredSize
		logger.Infof("updating PVC %q size from %s to %s", currentPVC.Name, currentSize.String(), desiredSize.String())
		if err := client.Update(context.TODO(), currentPVC); err != nil {
			// log the error, but don't fail the reconcile
			logger.Errorf("failed to update PVC size. %v", err)
			return
		}
		logger.Infof("successfully updated PVC %q size", currentPVC.Name)
	} else if desiredSize.Value() < currentSize.Value() {
		logger.Warningf("ignoring request to shrink PVC %q size from %s to %s, only expansion is allowed", currentPVC.Name, currentSize.String(), desiredSize.String())
	}
}
