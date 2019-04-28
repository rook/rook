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
	"github.com/rook/rook/pkg/operator/cassandra/constants"
	"github.com/rook/rook/pkg/operator/cassandra/controller/util"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// UpdateStatus updates the status of the given Cassandra Cluster.
// It doesn't post the result to the API Server yet.
// That will be done at the end of the sync loop.
func (cc *ClusterController) updateStatus(c *cassandrav1alpha1.Cluster) error {
	clusterStatus := cassandrav1alpha1.ClusterStatus{
		Racks: map[string]*cassandrav1alpha1.RackStatus{},
	}
	logger.Infof("Updating Status for cluster %s in namespace %s", c.Name, c.Namespace)

	for _, rack := range c.Spec.Datacenter.Racks {

		status := &cassandrav1alpha1.RackStatus{}

		// Get corresponding StatefulSet from lister
		sts, err := cc.statefulSetLister.StatefulSets(c.Namespace).
			Get(util.StatefulSetNameForRack(rack, c))
		// If it wasn't found, continue
		if apierrors.IsNotFound(err) {
			continue
		}
		// If we got a different error, requeue and log it
		if err != nil {
			return fmt.Errorf("error trying to get StatefulSet %s in namespace %s: %s", sts.Name, sts.Namespace, err.Error())
		}

		// Update Members
		status.Members = *sts.Spec.Replicas
		// Update ReadyMembers
		status.ReadyMembers = sts.Status.ReadyReplicas

		// Update Scaling Down condition
		services, err := util.GerMemberServicesForRack(rack, c, cc.serviceLister)
		if err != nil {
			return fmt.Errorf("error trying to get Pods for rack %s", rack.Name)
		}
		for _, svc := range services {
			// Check if there is a decommission in progress
			if _, ok := svc.Labels[constants.DecommissionLabel]; ok {
				// Add MemberLeaving Condition to rack status
				status.Conditions = append(status.Conditions, cassandrav1alpha1.RackCondition{
					Type:   cassandrav1alpha1.RackConditionTypeMemberLeaving,
					Status: cassandrav1alpha1.ConditionTrue,
				})
				// Sanity check. Only the last member should be decommissioning.
				index, err := util.IndexFromName(svc.Name)
				if err != nil {
					return err
				}
				if index != status.Members-1 {
					return fmt.Errorf("only last member of each rack should be decommissioning, but %d-th member of %s found decommissioning while rack had %d members", index, rack.Name, status.Members)
				}
			}
		}

		// Update Status for Rack
		clusterStatus.Racks[rack.Name] = status
	}

	c.Status = clusterStatus
	return nil
}

// SyncCluster checks the Status and performs reconciliation for
// the given Cassandra Cluster.
func (cc *ClusterController) syncCluster(c *cassandrav1alpha1.Cluster) error {
	// Check if any rack isn't created
	for _, rack := range c.Spec.Datacenter.Racks {
		// For each rack, check if a status entry exists
		if _, ok := c.Status.Racks[rack.Name]; !ok {
			logger.Infof("Attempting to create Rack %s", rack.Name)
			err := cc.createRack(rack, c)
			return err
		}
	}

	// Check if there is a scale-down in progress
	for _, rack := range c.Spec.Datacenter.Racks {
		if util.IsRackConditionTrue(c.Status.Racks[rack.Name], cassandrav1alpha1.RackConditionTypeMemberLeaving) {
			// Resume scale down
			err := cc.scaleDownRack(rack, c)
			return err
		}
	}

	// Check that all racks are ready before taking any action
	for _, rack := range c.Spec.Datacenter.Racks {
		rackStatus := c.Status.Racks[rack.Name]
		if rackStatus.Members != rackStatus.ReadyMembers {
			logger.Infof("Rack %s is not ready, %+v", rack.Name, *rackStatus)
			return nil
		}
	}

	// Check if any rack needs to scale down
	for _, rack := range c.Spec.Datacenter.Racks {
		if rack.Members < c.Status.Racks[rack.Name].Members {
			// scale down
			err := cc.scaleDownRack(rack, c)
			return err
		}
	}

	// Check if any rack needs to scale up
	for _, rack := range c.Spec.Datacenter.Racks {

		if rack.Members > c.Status.Racks[rack.Name].Members {
			logger.Infof("Attempting to scale rack %s", rack.Name)
			err := cc.scaleUpRack(rack, c)
			return err
		}
	}

	return nil
}

// createRack creates a new Cassandra Rack with 0 Members.
func (cc *ClusterController) createRack(r cassandrav1alpha1.RackSpec, c *cassandrav1alpha1.Cluster) error {
	sts := util.StatefulSetForRack(r, c, cc.rookImage)
	c.Spec.Annotations.Merge(r.Annotations).ApplyToObjectMeta(&sts.Spec.Template.ObjectMeta)
	c.Spec.Annotations.Merge(r.Annotations).ApplyToObjectMeta(&sts.ObjectMeta)
	existingStatefulset, err := cc.statefulSetLister.StatefulSets(sts.Namespace).Get(sts.Name)
	if err == nil {
		return util.VerifyOwner(existingStatefulset, c)
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("Error trying to create StatefulSet %s in namespace %s : %s", sts.Name, sts.Namespace, err.Error())
	}

	_, err = cc.kubeClient.AppsV1().StatefulSets(sts.Namespace).Create(sts)

	if err == nil {
		cc.recorder.Event(
			c,
			corev1.EventTypeNormal,
			SuccessSynced,
			fmt.Sprintf(MessageRackCreated, r.Name),
		)
	}

	if err != nil {
		logger.Errorf("Unexpected error while creating rack for cluster %+v: %s", c, err.Error())
	}

	return err
}

// scaleUpRack handles scaling up for an existing Cassandra Rack.
// Calling this action implies all members of the Rack are Ready.
func (cc *ClusterController) scaleUpRack(r cassandrav1alpha1.RackSpec, c *cassandrav1alpha1.Cluster) error {
	sts, err := cc.statefulSetLister.StatefulSets(c.Namespace).Get(util.StatefulSetNameForRack(r, c))
	if err != nil {
		return fmt.Errorf("error trying to scale rack %s in namespace %s, underlying StatefulSet not found", r.Name, c.Namespace)
	}

	logger.Infof("Attempting to scale up Rack %s", r.Name)

	err = util.ScaleStatefulSet(sts, 1, cc.kubeClient)

	if err == nil {
		cc.recorder.Event(
			c,
			corev1.EventTypeNormal,
			SuccessSynced,
			fmt.Sprintf(MessageRackScaledUp, r.Name, *sts.Spec.Replicas+1),
		)
	}

	return err

}

// scaleDownRack handles scaling down for an existing Cassandra Rack.
// Calling this action implies all members of the Rack are Ready.
func (cc *ClusterController) scaleDownRack(r cassandrav1alpha1.RackSpec, c *cassandrav1alpha1.Cluster) error {
	logger.Infof("Scaling down rack %s", r.Name)

	// Get the current actual number of Members
	members := c.Status.Racks[r.Name].Members

	// Find the member to decommission
	memberName := fmt.Sprintf("%s-%d", util.StatefulSetNameForRack(r, c), members-1)
	logger.Infof("Member of interest: %s", memberName)
	memberService, err := cc.serviceLister.Services(c.Namespace).Get(memberName)
	if err != nil {
		return fmt.Errorf("error trying to get Member Service %s: %s", memberName, err.Error())
	}

	// Check if there was a scale down in progress that has completed.
	if memberService.Labels[constants.DecommissionLabel] == constants.LabelValueTrue {

		logger.Infof("Found decommissioned member: %s", memberName)

		// Get rack's statefulset
		stsName := util.StatefulSetNameForRack(r, c)
		sts, err := cc.statefulSetLister.StatefulSets(c.Namespace).Get(stsName)
		if err != nil {
			return fmt.Errorf("error trying to get StatefulSet %s", stsName)
		}
		// Scale the statefulset
		err = util.ScaleStatefulSet(sts, -1, cc.kubeClient)
		if err != nil {
			return fmt.Errorf("error trying to scale down StatefulSet %s", stsName)
		}
		// Cleanup is done on each sync loop, no need to do anything else here

		cc.recorder.Event(
			c,
			corev1.EventTypeNormal,
			SuccessSynced,
			fmt.Sprintf(MessageRackScaledDown, r.Name, members-1),
		)
		return nil
	}

	logger.Infof("Checking for scale down. Desired: %d. Actual: %d", r.Members, c.Status.Racks[r.Name].Members)
	// Then, check if there is a requested scale down.
	if r.Members < c.Status.Racks[r.Name].Members {

		logger.Infof("Scale down requested, member %s will decommission", memberName)
		// Record the intent to decommission the member
		old := memberService.DeepCopy()
		memberService.Labels[constants.DecommissionLabel] = constants.LabelValueFalse
		if err := util.PatchService(old, memberService, cc.kubeClient); err != nil {
			return fmt.Errorf("error patching member service %s: %s", memberName, err.Error())
		}

		cc.recorder.Event(
			c,
			corev1.EventTypeNormal,
			SuccessSynced,
			fmt.Sprintf(MessageRackScaleDownInProgress, r.Name, members-1),
		)
	}

	return nil
}
