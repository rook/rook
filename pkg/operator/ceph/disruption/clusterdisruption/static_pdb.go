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

package clusterdisruption

import (
	"context"

	"github.com/pkg/errors"

	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcileClusterDisruption) createStaticPDB(pdb *policyv1beta1.PodDisruptionBudget) error {
	err := r.client.Create(context.TODO(), pdb)
	if err != nil {
		return errors.Wrapf(err, "failed to create pdb %q", pdb.Name)
	}
	return nil
}

func (r *ReconcileClusterDisruption) reconcileStaticPDB(request types.NamespacedName, pdb *policyv1beta1.PodDisruptionBudget) error {
	existingPDB := &policyv1beta1.PodDisruptionBudget{}
	err := r.client.Get(context.TODO(), request, existingPDB)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.createStaticPDB(pdb)
		}
		return errors.Wrapf(err, "failed to get pdb %q", pdb.Name)
	}

	return nil
}
