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

	// Check that all racks are ready before taking any action
	for _, rack := range c.Spec.Datacenter.Racks {
		rackStatus := c.Status.Racks[rack.Name]
		if rackStatus.Members != rackStatus.ReadyMembers {
			logger.Infof("Rack %s is not ready, %+v", rack.Name, *rackStatus)
			return nil
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
