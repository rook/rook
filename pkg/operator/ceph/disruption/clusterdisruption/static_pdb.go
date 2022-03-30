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
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rook/rook/pkg/operator/k8sutil"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcileClusterDisruption) createStaticPDB(pdb client.Object) error {
	err := r.client.Create(r.context.OpManagerContext, pdb)
	if err != nil {
		return errors.Wrapf(err, "failed to create pdb %q", pdb.GetName())
	}
	return nil
}

func (r *ReconcileClusterDisruption) reconcileStaticPDB(request types.NamespacedName, pdb client.Object) error {
	var existingPDB client.Object
	usePDBV1Beta1, err := k8sutil.UsePDBV1Beta1Version(r.context.ClusterdContext.Clientset)
	if err != nil {
		return errors.Wrap(err, "failed to fetch pdb version")
	}
	if usePDBV1Beta1 {
		existingPDB = &policyv1beta1.PodDisruptionBudget{}
	} else {
		existingPDB = &policyv1.PodDisruptionBudget{}
	}
	err = r.client.Get(r.context.OpManagerContext, request, existingPDB)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.createStaticPDB(pdb)
		}
		return errors.Wrapf(err, "failed to get pdb %q", pdb.GetName())
	}

	return nil
}
