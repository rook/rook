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
	"github.com/rook/rook/pkg/operator/k8sutil"

	"github.com/pkg/errors"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileClusterDisruption) processPools(request reconcile.Request) (*cephv1.CephObjectStoreList, *cephv1.CephFilesystemList, string, int, error) {
	namespaceListOpt := client.InNamespace(request.Namespace)
	poolSpecs := make([]cephv1.PoolSpec, 0)
	poolCount := 0
	cephBlockPoolList := &cephv1.CephBlockPoolList{}
	err := r.client.List(r.context.OpManagerContext, cephBlockPoolList, namespaceListOpt)
	if err != nil {
		return nil, nil, "", poolCount, errors.Wrapf(err, "could not list the CephBlockpools %v", request.NamespacedName)
	}
	poolCount += len(cephBlockPoolList.Items)
	for _, cephBlockPool := range cephBlockPoolList.Items {
		poolSpecs = append(poolSpecs, cephBlockPool.Spec.PoolSpec)
	}

	cephFilesystemList := &cephv1.CephFilesystemList{}
	err = r.client.List(r.context.OpManagerContext, cephFilesystemList, namespaceListOpt)
	if err != nil {
		return nil, nil, "", poolCount, errors.Wrapf(err, "could not list the CephFilesystems %v", request.NamespacedName)
	}
	poolCount += len(cephFilesystemList.Items)
	for _, cephFilesystem := range cephFilesystemList.Items {
		poolSpecs = append(poolSpecs, cephFilesystem.Spec.MetadataPool)
		for _, pool := range cephFilesystem.Spec.DataPools {
			poolSpecs = append(poolSpecs, pool.PoolSpec)
		}

	}

	cephObjectStoreList := &cephv1.CephObjectStoreList{}
	err = r.client.List(r.context.OpManagerContext, cephObjectStoreList, namespaceListOpt)
	if err != nil {
		return nil, nil, "", poolCount, errors.Wrapf(err, "could not list the CephObjectStores %v", request.NamespacedName)
	}
	poolCount += len(cephObjectStoreList.Items)
	for _, cephObjectStore := range cephObjectStoreList.Items {
		poolSpecs = append(poolSpecs, cephObjectStore.Spec.MetadataPool)
		poolSpecs = append(poolSpecs, cephObjectStore.Spec.DataPool)

	}
	minFailureDomain := getMinimumFailureDomain(poolSpecs)

	return cephObjectStoreList, cephFilesystemList, minFailureDomain, poolCount, nil

}

func getMinimumFailureDomain(poolList []cephv1.PoolSpec) string {
	if len(poolList) == 0 {
		return cephv1.DefaultFailureDomain
	}

	//start with max as the min
	minfailureDomainIndex := len(topology.CRUSHMapLevelsOrdered) - 1
	matched := false

	for _, pool := range poolList {
		for index, failureDomain := range topology.CRUSHMapLevelsOrdered {
			if index == minfailureDomainIndex {
				// index is higher-than/equal-to the min
				break
			}
			if pool.FailureDomain == failureDomain {
				// new min found
				matched = true
				minfailureDomainIndex = index
			}
		}
	}
	if !matched {
		logger.Debugf("could not match failure domain. defaulting to %q", cephv1.DefaultFailureDomain)
		return cephv1.DefaultFailureDomain
	}
	return topology.CRUSHMapLevelsOrdered[minfailureDomainIndex]
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
		usePDBV1Beta1, err := k8sutil.UsePDBV1Beta1Version(r.context.ClusterdContext.Clientset)
		if err != nil {
			return errors.Wrap(err, "failed to fetch pdb version")
		}
		if usePDBV1Beta1 {
			pdb := &policyv1beta1.PodDisruptionBudget{
				ObjectMeta: objectMeta,
				Spec: policyv1beta1.PodDisruptionBudgetSpec{
					Selector:     labelSelector,
					MinAvailable: minAvailable,
				},
			}
			request := types.NamespacedName{Name: pdbName, Namespace: namespace}
			err = r.reconcileStaticPDB(request, pdb)
			if err != nil {
				return errors.Wrapf(err, "failed to reconcile cephobjectstore pdb %v", request)
			}
			continue
		}
		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: objectMeta,
			Spec: policyv1.PodDisruptionBudgetSpec{
				Selector:     labelSelector,
				MinAvailable: minAvailable,
			},
		}
		request := types.NamespacedName{Name: pdbName, Namespace: namespace}
		err = r.reconcileStaticPDB(request, pdb)
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
		usePDBV1Beta1, err := k8sutil.UsePDBV1Beta1Version(r.context.ClusterdContext.Clientset)
		if err != nil {
			return errors.Wrap(err, "failed to fetch pdb version")
		}
		if usePDBV1Beta1 {
			pdb := &policyv1beta1.PodDisruptionBudget{
				ObjectMeta: objectMeta,
				Spec: policyv1beta1.PodDisruptionBudgetSpec{
					Selector:     labelSelector,
					MinAvailable: minAvailable,
				},
			}
			request := types.NamespacedName{Name: pdbName, Namespace: namespace}
			err := r.reconcileStaticPDB(request, pdb)
			if err != nil {
				return errors.Wrapf(err, "failed to reconcile cephfs pdb %v", request)
			}
			continue
		}
		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: objectMeta,
			Spec: policyv1.PodDisruptionBudgetSpec{
				Selector:     labelSelector,
				MinAvailable: minAvailable,
			},
		}
		request := types.NamespacedName{Name: pdbName, Namespace: namespace}
		err = r.reconcileStaticPDB(request, pdb)
		if err != nil {
			return errors.Wrapf(err, "failed to reconcile cephfs pdb %v", request)
		}
	}
	return nil
}
