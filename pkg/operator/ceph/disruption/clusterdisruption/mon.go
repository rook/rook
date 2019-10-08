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
	"math"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/k8sutil"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	pdbName = "rook-ceph-mon-pdb"
)

// setting minAvailable for mons at floor((n+1)/2) an logging and error if n is even
// monCount - minAvailable
// 1,2      - 0 (no HA)
// even     - n -1
// odd      - floor((n+1)/2)
func (r *ReconcileClusterDisruption) reconcileMonPDB(cephCluster *cephv1.CephCluster) error {
	monCount := cephCluster.Spec.Mon.Count
	minAvailable := int32(math.Floor(float64((monCount + 1) / 2)))
	if monCount%2 == 0 {
		logger.Error("mon count should be an odd number, setting effective maxUnvailable to 1")
		minAvailable = int32(monCount - 1)
	}
	if monCount <= 2 {
		logger.Error("managePodBudgets is set, but mon-count <= 2. Not creating a disruptionbudget for Mons")
		return nil
	}
	namespace := cephCluster.ObjectMeta.Namespace
	pdbRequest := types.NamespacedName{Name: pdbName, Namespace: namespace}
	pdb := &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pdbName,
			Namespace: namespace,
		},
		Spec: policyv1beta1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{k8sutil.AppAttr: mon.AppName},
			},
			MinAvailable: &intstr.IntOrString{IntVal: minAvailable},
		},
	}
	err := r.reconcileStaticPDB(pdbRequest, pdb)
	if err != nil {
		return errors.Wrapf(err, "could not reconcile mon pdb")
	}
	return nil
}
