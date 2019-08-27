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

	"github.com/rook/rook/pkg/operator/ceph/controllers/nodedrain"
	"github.com/rook/rook/pkg/operator/k8sutil"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	// metav1 "k8s.io/api/meta/v1"
)

func (r *ReconcileClusterDisruption) getFailureDomain(request reconcile.Request) (string, int, error) {
	namespaceListOpt := client.InNamespace(request.Namespace)
	poolSpecs := make([]cephv1.PoolSpec, 0)
	poolCount := 0
	cephBlockPoolList := &cephv1.CephBlockPoolList{}
	err := r.client.List(context.TODO(), cephBlockPoolList, namespaceListOpt)
	if err != nil {
		return "", poolCount, fmt.Errorf("could not list the CephBlockpools %s: %+v", request.NamespacedName, err)
	}
	poolCount += len(cephBlockPoolList.Items)
	for _, cephBlockPool := range cephBlockPoolList.Items {
		poolSpecs = append(poolSpecs, cephBlockPool.Spec)
	}

	cephFilesystemList := &cephv1.CephFilesystemList{}
	err = r.client.List(context.TODO(), cephFilesystemList, namespaceListOpt)
	if err != nil {
		return "", poolCount, fmt.Errorf("could not list the CephFilesystems  %s: %+v", request.NamespacedName, err)
	}
	poolCount += len(cephFilesystemList.Items)
	for _, cephFilesystem := range cephFilesystemList.Items {
		poolSpecs = append(poolSpecs, cephFilesystem.Spec.MetadataPool)
		poolSpecs = append(poolSpecs, cephFilesystem.Spec.DataPools...)

	}

	cephObjectStoreList := &cephv1.CephObjectStoreList{}
	err = r.client.List(context.TODO(), cephObjectStoreList, namespaceListOpt)
	if err != nil {
		return "", poolCount, fmt.Errorf("could not list the CephObjectStores %s: %+v", request.NamespacedName, err)
	}
	poolCount += len(cephObjectStoreList.Items)
	for _, cephObjectStore := range cephObjectStoreList.Items {
		poolSpecs = append(poolSpecs, cephObjectStore.Spec.MetadataPool)
		poolSpecs = append(poolSpecs, cephObjectStore.Spec.DataPool)

	}

	return getMinimumFailureDomain(poolSpecs), poolCount, nil

}

// TODO: test
func getMinimumFailureDomain(poolList []cephv1.PoolSpec) string {
	failureDomainOrder := []string{"osd", "host", "zone", "region"}

	//start with max as the min
	minfailureDomainIndex := len(failureDomainOrder) - 1
	matched := false

	for _, pool := range poolList {
		for index, failureDomain := range failureDomainOrder {
			if len(failureDomain) == 0 {
				failureDomain = cephv1.DefaultFailureDomain
			}
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
		return cephv1.DefaultFailureDomain
	}
	return failureDomainOrder[minfailureDomainIndex]
}

func (r *ReconcileClusterDisruption) getOngoingDrains(request reconcile.Request) ([]*corev1.Node, error) {
	// Get the canary deployments
	canaryDeploymentList := &appsv1.DeploymentList{}
	operatorNameSpaceListOpts := client.InNamespace(request.Namespace)
	err := r.client.List(context.TODO(), canaryDeploymentList, client.MatchingLabels{k8sutil.AppAttr: nodedrain.CanaryAppName}, operatorNameSpaceListOpts)
	if err != nil {
		return nil, fmt.Errorf("could not list canary deployments: %+v", err)
	}

	ongoingDrains := make([]*corev1.Node, 0)
	for _, deployment := range canaryDeploymentList.Items {
		if deployment.Status.ReadyReplicas < 1 {
			nodeHostname, ok := deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname]
			if !ok {
				logger.Errorf("could not find a the nodeSelector key %s for canary deployment %s", corev1.LabelHostname, deployment.ObjectMeta.Name)
				continue
			}

			node := &corev1.Node{}
			err = r.client.Get(context.TODO(), types.NamespacedName{Name: nodeHostname}, node)
			if err != nil {
				return nil, fmt.Errorf("could not get node: %s ", nodeHostname)
			}
			ongoingDrains = append(ongoingDrains, node)
		}
	}
	return ongoingDrains, nil
}
