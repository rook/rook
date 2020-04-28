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
}

// newCephStatusChecker creates a new HealthChecker object
func newCephStatusChecker(context *clusterd.Context, namespace, resourceName string, externalCred config.ExternalCred) *cephStatusChecker {
	c := &cephStatusChecker{
		context:      context,
		namespace:    namespace,
		resourceName: resourceName,
		interval:     defaultStatusCheckInterval,
		externalCred: externalCred,
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
		return
	}

	logger.Debugf("Cluster status: %+v", status)
	if err := c.updateCephStatus(&status); err != nil {
		logger.Errorf("failed to query cluster status in namespace %q. %v", c.namespace, err)
	}
}

// updateCephStatus detects the latest health status from ceph and updates the CR status
func (c *cephStatusChecker) updateCephStatus(status *client.CephStatus) error {

	// get the most recent cluster CRD object
	cluster, err := c.context.RookClientset.CephV1().CephClusters(c.namespace).Get(c.resourceName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get cluster from namespace %s prior to updating its status", c.namespace)
	}

	// translate the ceph status struct to the crd status
	cluster.Status.CephStatus = toCustomResourceStatus(cluster.Status, status)
	if _, err := c.context.RookClientset.CephV1().CephClusters(c.namespace).Update(cluster); err != nil {
		return errors.Wrapf(err, "failed to update cluster %s status", c.namespace)
	}

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
