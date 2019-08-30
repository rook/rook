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

	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// MonPDBAppName for monitor daemon poddisruptionbudgets
	MonPDBAppName = "rook-ceph-mon-pdb"
)

func (r *ReconcileClusterDisruption) createStaticPDB(request types.NamespacedName, pdb *policyv1beta1.PodDisruptionBudget) error {
	err := r.client.Create(context.TODO(), pdb)
	if err != nil {
		return err
	}
	return nil
}

// PDBs can't be updated, so we use a delete/create
// This will change only with kube 1.15: https://github.com/kubernetes/kubernetes/issues/45398#issuecomment-495362316
func (r *ReconcileClusterDisruption) updateStaticPDB(request types.NamespacedName, pdb *policyv1beta1.PodDisruptionBudget) error {
	err := r.client.Delete(context.TODO(), pdb)
	if err != nil {
		return err
	}
	return r.createStaticPDB(request, pdb)
}

func (r *ReconcileClusterDisruption) reconcileStaticPDB(request types.NamespacedName, pdb *policyv1beta1.PodDisruptionBudget) error {
	err := r.client.Get(context.TODO(), request, pdb)
	if errors.IsNotFound(err) {
		return r.createStaticPDB(request, pdb)
	} else if err != nil {
		return err
	}
	return nil
}
