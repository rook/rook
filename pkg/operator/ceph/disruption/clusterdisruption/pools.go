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
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/disruption/nodedrain"
	"github.com/rook/rook/pkg/operator/k8sutil"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (r *ReconcileClusterDisruption) processPools(request reconcile.Request) (*cephv1.CephObjectStoreList, *cephv1.CephFilesystemList, string, int, error) {
	namespaceListOpt := client.InNamespace(request.Namespace)
	poolSpecs := make([]cephv1.PoolSpec, 0)
	poolCount := 0
	cephBlockPoolList := &cephv1.CephBlockPoolList{}
	err := r.client.List(context.TODO(), cephBlockPoolList, namespaceListOpt)
	if err != nil {
		return nil, nil, "", poolCount, errors.Wrapf(err, "could not list the CephBlockpools %s", request.NamespacedName)
	}
	poolCount += len(cephBlockPoolList.Items)
	for _, cephBlockPool := range cephBlockPoolList.Items {
		poolSpecs = append(poolSpecs, cephBlockPool.Spec)
	}

	cephFilesystemList := &cephv1.CephFilesystemList{}
	err = r.client.List(context.TODO(), cephFilesystemList, namespaceListOpt)
	if err != nil {
		return nil, nil, "", poolCount, errors.Wrapf(err, "could not list the CephFilesystems %s", request.NamespacedName)
	}
	poolCount += len(cephFilesystemList.Items)
	for _, cephFilesystem := range cephFilesystemList.Items {
		poolSpecs = append(poolSpecs, cephFilesystem.Spec.MetadataPool)
		poolSpecs = append(poolSpecs, cephFilesystem.Spec.DataPools...)

	}

	cephObjectStoreList := &cephv1.CephObjectStoreList{}
	err = r.client.List(context.TODO(), cephObjectStoreList, namespaceListOpt)
	if err != nil {
		return nil, nil, "", poolCount, errors.Wrapf(err, "could not list the CephObjectStores %s", request.NamespacedName)
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
	minfailureDomainIndex := len(osd.CRUSHMapLevelsOrdered) - 1
	matched := false

	for _, pool := range poolList {
		for index, failureDomain := range osd.CRUSHMapLevelsOrdered {
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
	return osd.CRUSHMapLevelsOrdered[minfailureDomainIndex]
}

func (r *ReconcileClusterDisruption) getOngoingDrains(request reconcile.Request) ([]*corev1.Node, error) {
	// Get the canary deployments
	canaryDeploymentList := &appsv1.DeploymentList{}
	operatorNameSpaceListOpts := client.InNamespace(request.Namespace)
	err := r.client.List(context.TODO(), canaryDeploymentList, client.MatchingLabels{k8sutil.AppAttr: nodedrain.CanaryAppName}, operatorNameSpaceListOpts)
	if err != nil {
		return nil, errors.Wrapf(err, "could not list canary deployments")
	}

	ongoingDrains := make([]*corev1.Node, 0)
	for _, deployment := range canaryDeploymentList.Items {
		if deployment.Status.ReadyReplicas < 1 {
			nodeHostname, ok := deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname]
			if !ok {
				logger.Errorf("could not find a the nodeSelector key %q for canary deployment %q", corev1.LabelHostname, deployment.ObjectMeta.Name)
				continue
			}

			nodeList := &corev1.NodeList{}
			err = r.client.List(context.TODO(), nodeList, client.MatchingLabels{corev1.LabelHostname: nodeHostname})
			nodeNum := len(nodeList.Items)
			if err != nil || nodeNum < 1 {
				return nil, errors.Errorf("could not get node: %s ", nodeHostname)
			} else if nodeNum > 1 {
				logger.Warningf("found more than one node with %s=%s", corev1.LabelHostname, nodeHostname)
			}
			ongoingDrains = append(ongoingDrains, &nodeList.Items[0])
		}
	}
	return ongoingDrains, nil
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
		if minAvailable.IntVal <= 1 {
			break
		}
		blockOwnerDeletion := false
		pdb := &policyv1beta1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
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
			},
			Spec: policyv1beta1.PodDisruptionBudgetSpec{
				Selector:     labelSelector,
				MinAvailable: minAvailable,
			},
		}

		request := types.NamespacedName{Name: pdbName, Namespace: namespace}
		err := r.reconcileStaticPDB(request, pdb)
		if err != nil {
			return errors.Wrapf(err, "could not reconcile cephobjectstore pdb %s", request)
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
			break
		}
		blockOwnerDeletion := false
		pdb := &policyv1beta1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
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
			},
			Spec: policyv1beta1.PodDisruptionBudgetSpec{
				Selector:     labelSelector,
				MinAvailable: minAvailable,
			},
		}

		request := types.NamespacedName{Name: pdbName, Namespace: namespace}
		err := r.reconcileStaticPDB(request, pdb)
		if err != nil {
			return errors.Wrapf(err, "could not reconcile cephfs pdb %s", request)
		}
	}
	return nil
}
