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
	"fmt"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/topology"
	log "github.com/rook/rook/pkg/util/log"

	"github.com/pkg/errors"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileClusterDisruption) processPools(request reconcile.Request) (*cephv1.CephObjectStoreList, *cephv1.CephFilesystemList, string, int, error) {
	poolCRs, err := topology.ListPoolCRs(r.context.OpManagerContext, r.client, request.Namespace)
	if err != nil {
		return nil, nil, "", 0, errors.Wrapf(err, "failed to list the pools for %v", request.NamespacedName)
	}
	minFailureDomain := topology.GetMinimumFailureDomain(poolCRs.PoolSpecs())

	return poolCRs.ObjectStores, poolCRs.Filesystems, minFailureDomain, poolCRs.Count(), nil
}

// Setting naive minAvailable for RGW at: n - 1
func (r *ReconcileClusterDisruption) reconcileCephObjectStore(cephObjectStoreList *cephv1.CephObjectStoreList) error {
	for _, objectStore := range cephObjectStoreList.Items {
		storeName := objectStore.ObjectMeta.Name
		namespace := objectStore.ObjectMeta.Namespace
		pdbName := fmt.Sprintf("rook-ceph-rgw-%s", storeName)
		labelSelector := &metav1.LabelSelector{
			MatchLabels: map[string]string{"rgw": storeName},
		}

		rgwCount := objectStore.Spec.Gateway.Instances
		minAvailable := &intstr.IntOrString{IntVal: rgwCount - 1}
		if minAvailable.IntVal < 1 {
			stalePDB := &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{Name: pdbName, Namespace: namespace},
			}
			log.NamespacedInfo(namespace, logger, "deleting stale PDB %q with %d instance(s)", pdbName, rgwCount)
			if err := r.deletePDB(stalePDB); err != nil {
				return errors.Wrapf(err, "failed to delete stale pdb %q", pdbName)
			}
			continue
		}
		blockOwnerDeletion := false
		objectMeta := metav1.ObjectMeta{
			Name:      pdbName,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         objectStore.APIVersion,
					Kind:               objectStore.Kind,
					Name:               objectStore.ObjectMeta.Name,
					UID:                objectStore.UID,
					BlockOwnerDeletion: &blockOwnerDeletion,
				},
			},
		}
		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: objectMeta,
			Spec: policyv1.PodDisruptionBudgetSpec{
				Selector:     labelSelector,
				MinAvailable: minAvailable,
			},
		}
		request := types.NamespacedName{Name: pdbName, Namespace: namespace}
		err := r.reconcileStaticPDB(request, pdb)
		if err != nil {
			return errors.Wrapf(err, "failed to reconcile cephobjectstore pdb %v", request)
		}
	}
	return nil
}

// Setting naive minAvailable for MDS at: n -1
// getting n from the cephfilesystem.spec.metadataserver.activecount
func (r *ReconcileClusterDisruption) reconcileCephFilesystem(cephFilesystemList *cephv1.CephFilesystemList) error {
	for _, filesystem := range cephFilesystemList.Items {
		fsName := filesystem.ObjectMeta.Name
		namespace := filesystem.ObjectMeta.Namespace
		pdbName := fmt.Sprintf("rook-ceph-mds-%s", fsName)
		labelSelector := &metav1.LabelSelector{
			MatchLabels: map[string]string{"rook_file_system": fsName},
		}

		activeCount := filesystem.Spec.MetadataServer.ActiveCount
		minAvailable := &intstr.IntOrString{IntVal: activeCount - 1}
		if filesystem.Spec.MetadataServer.ActiveStandby {
			minAvailable.IntVal++
		}
		if minAvailable.IntVal < 1 {
			stalePDB := &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{Name: pdbName, Namespace: namespace},
			}
			log.NamespacedInfo(namespace, logger, "deleting stale PDB %q", pdbName)
			if err := r.deletePDB(stalePDB); err != nil {
				return errors.Wrapf(err, "failed to delete stale pdb %q", pdbName)
			}
			continue
		}
		blockOwnerDeletion := false
		objectMeta := metav1.ObjectMeta{
			Name:      pdbName,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         filesystem.APIVersion,
					Kind:               filesystem.Kind,
					Name:               filesystem.ObjectMeta.Name,
					UID:                filesystem.UID,
					BlockOwnerDeletion: &blockOwnerDeletion,
				},
			},
		}
		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: objectMeta,
			Spec: policyv1.PodDisruptionBudgetSpec{
				Selector:     labelSelector,
				MinAvailable: minAvailable,
			},
		}
		request := types.NamespacedName{Name: pdbName, Namespace: namespace}
		err := r.reconcileStaticPDB(request, pdb)
		if err != nil {
			return errors.Wrapf(err, "failed to reconcile cephfs pdb %v", request)
		}
	}
	return nil
}
