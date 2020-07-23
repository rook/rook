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

// Package cluster to manage Kubernetes storage.
package cluster

import (
	"os"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/config"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// defaultStatusCheckInterval is the interval to check the status of the ceph cluster
	defaultStatusCheckInterval = 60 * time.Second
)

// cephStatusChecker aggregates the mon/cluster info needed to check the health of the monitors
type cephStatusChecker struct {
	context      *clusterd.Context
	namespace    string
	resourceName string
	interval     time.Duration
	externalCred config.ExternalCred
	isExternal   bool
}

// newCephStatusChecker creates a new HealthChecker object
func newCephStatusChecker(context *clusterd.Context, namespace, resourceName string, externalCred config.ExternalCred, isExternal bool) *cephStatusChecker {
	c := &cephStatusChecker{
		context:      context,
		namespace:    namespace,
		resourceName: resourceName,
		interval:     defaultStatusCheckInterval,
		externalCred: externalCred,
		isExternal:   isExternal,
	}

	// allow overriding the check interval with an env var on the operator
	checkInterval := os.Getenv("ROOK_CEPH_STATUS_CHECK_INTERVAL")
	if checkInterval != "" {
		if duration, err := time.ParseDuration(checkInterval); err == nil {
			logger.Infof("ceph status check interval is %s", checkInterval)
			c.interval = duration
		}
	}
	return c
}

// checkCephStatus periodically checks the health of the cluster
func (c *cephStatusChecker) checkCephStatus(stopCh chan struct{}) {
	// check the status immediately before starting the loop
	c.checkStatus()

	for {
		select {
		case <-stopCh:
			logger.Infof("Stopping monitoring of ceph status")
			return

		case <-time.After(c.interval):
			c.checkStatus()
		}
	}
}

// checkStatus queries the status of ceph health then updates the CR status
func (c *cephStatusChecker) checkStatus() {
	var status client.CephStatus
	var err error

	logger.Debugf("checking health of cluster")

	// Set the user health check to the admin user
	healthCheckUser := client.AdminUsername

	// This is an external cluster OR if the admin keyring is not present
	// As of 1.3 an external cluster is deployed it uses a different user to check ceph's status
	if c.externalCred.Username != "" && c.externalCred.Secret != "" {
		if c.externalCred.Username != client.AdminUsername {
			healthCheckUser = c.externalCred.Username
		}
	}

	// Check ceph's status
	status, err = client.StatusWithUser(c.context, c.namespace, healthCheckUser)
	if err != nil {
		logger.Errorf("failed to get ceph status. %v", err)
		condition, reason, message := c.conditionMessageReason(cephv1.ConditionFailure)
		if err := c.updateCephStatus(cephStatusOnError(err.Error()), condition, reason, message); err != nil {
			logger.Errorf("failed to query cluster status in namespace %q. %v", c.namespace, err)
		}
		return
	}

	logger.Debugf("Cluster status: %+v", status)
	condition, reason, message := c.conditionMessageReason(cephv1.ConditionReady)
	if err := c.updateCephStatus(&status, condition, reason, message); err != nil {
		logger.Errorf("failed to query cluster status in namespace %q. %v", c.namespace, err)
	}
}

// updateCephStatus detects the latest health status from ceph and updates the CR status
func (c *cephStatusChecker) updateCephStatus(status *client.CephStatus, condition cephv1.ConditionType, reason, message string) error {

	// get the most recent cluster CRD object
	cluster, err := c.context.RookClientset.CephV1().CephClusters(c.namespace).Get(c.resourceName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get cluster from namespace %s prior to updating its status", c.namespace)
	}

	// translate the ceph status struct to the crd status
	cluster.Status.CephStatus = toCustomResourceStatus(cluster.Status, status)
	cluster.Status.Phase = condition
	if _, err := c.context.RookClientset.CephV1().CephClusters(c.namespace).Update(cluster); err != nil {
		return errors.Wrapf(err, "failed to update cluster %s status", c.namespace)
	}

	// Update condition
	opconfig.ConditionExport(c.context, c.namespace, c.resourceName, condition, v1.ConditionTrue, reason, message)
	logger.Debugf("ceph cluster %q status and condition updated to %+v, %v, %s, %s", c.namespace, status, v1.ConditionTrue, reason, message)

	return nil
}

// toCustomResourceStatus converts the ceph status to the struct expected for the CephCluster CR status
func toCustomResourceStatus(currentStatus cephv1.ClusterStatus, newStatus *client.CephStatus) *cephv1.CephStatus {
	s := &cephv1.CephStatus{
		Health:      newStatus.Health.Status,
		LastChecked: formatTime(time.Now().UTC()),
		Details:     make(map[string]cephv1.CephHealthMessage),
	}
	for name, message := range newStatus.Health.Checks {
		s.Details[name] = cephv1.CephHealthMessage{
			Severity: message.Severity,
			Message:  message.Summary.Message,
		}
	}
	if currentStatus.CephStatus != nil {
		s.PreviousHealth = currentStatus.CephStatus.PreviousHealth
		s.LastChanged = currentStatus.CephStatus.LastChanged
		if currentStatus.CephStatus.Health != s.Health {
			s.PreviousHealth = currentStatus.CephStatus.Health
			s.LastChanged = s.LastChecked
		}
	}
	return s
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC3339)
}

func cephStatusOnError(errorMessage string) *client.CephStatus {
	details := make(map[string]client.CheckMessage)
	details["error"] = client.CheckMessage{
		Severity: "Urgent",
		Summary: client.Summary{
			Message: errorMessage,
		},
	}

	return &client.CephStatus{
		Health: client.HealthStatus{
			Status: "HEALTH_ERR",
			Checks: details,
		},
	}
}

func (c *cephStatusChecker) conditionMessageReason(condition cephv1.ConditionType) (cephv1.ConditionType, string, string) {
	var reason, message string

	switch condition {
	case cephv1.ConditionFailure:
		reason = "ClusterFailure"
		message = "Failed to configure ceph cluster"
		if c.isExternal {
			message = "Failed to configure external ceph cluster"
		}
	case cephv1.ConditionReady:
		reason = "ClusterCreated"
		message = "Cluster created successfully"
		if c.isExternal {
			condition = cephv1.ConditionConnected
			reason = "ClusterConnected"
			message = "Cluster connected successfully"
		}
	}

	return condition, reason, message
}
