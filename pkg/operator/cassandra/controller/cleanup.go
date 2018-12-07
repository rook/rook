/*
Copyright 2018 The Rook Authors. All rights reserved.

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

package controller

import (
	"fmt"
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/operator/cassandra/controller/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// cleanup deletes all resources remaining because of cluster scale downs
func (cc *ClusterController) cleanup(c *cassandrav1alpha1.Cluster) error {

	for _, r := range c.Spec.Datacenter.Racks {
		services, err := cc.serviceLister.Services(c.Namespace).List(util.RackSelector(r, c))
		if err != nil {
			return fmt.Errorf("error listing member services: %s", err.Error())
		}
		// Get rack status. If it doesn't exist, the rack isn't yet created.
		stsName := util.StatefulSetNameForRack(r, c)
		sts, err := cc.statefulSetLister.StatefulSets(c.Namespace).Get(stsName)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("error getting statefulset %s: %s", stsName, err.Error())
		}
		memberCount := *sts.Spec.Replicas
		memberServiceCount := int32(len(services))
		// If there are more services than members, some services need to be cleaned up
		if memberServiceCount > memberCount {
			maxIndex := memberCount - 1
			for _, svc := range services {
				svcIndex, err := util.IndexFromName(svc.Name)
				if err != nil {
					logger.Errorf("Unexpected error while parsing index from name %s : %s", svc.Name, err.Error())
					continue
				}
				if svcIndex > maxIndex {
					err := cc.cleanupMemberResources(svc.Name, r, c)
					if err != nil {
						return fmt.Errorf("error cleaning up member resources: %s", err.Error())
					}
				}
			}
		}
	}
	logger.Infof("%s/%s - Successfully cleaned up cluster.", c.Namespace, c.Name)
	return nil
}

// cleanupMemberResources deletes all resources associated with a given member.
// Currently those are :
//  - A PVC
//  - A ClusterIP Service
func (cc *ClusterController) cleanupMemberResources(memberName string, r cassandrav1alpha1.RackSpec, c *cassandrav1alpha1.Cluster) error {

	logger.Infof("%s/%s - Cleaning up resources for member %s", c.Namespace, c.Name, memberName)
	// Delete PVC
	if len(r.Storage.VolumeClaimTemplates) > 0 {
		// PVC naming convention for StatefulSets is <volumeClaimTemplate.Name>-<pod.Name>
		pvcName := fmt.Sprintf("%s-%s", r.Storage.VolumeClaimTemplates[0].Name, memberName)
		err := cc.kubeClient.CoreV1().PersistentVolumeClaims(c.Namespace).Delete(pvcName, &metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("error deleting pvc %s: %s", pvcName, err.Error())
		}
	}

	// Delete Member Service
	err := cc.kubeClient.CoreV1().Services(c.Namespace).Delete(memberName, &metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("error deleting member service %s: %s", memberName, err.Error())
	}
	return nil
}
