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
	"context"
	"os"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// defaultStatusCheckInterval is the interval to check the status of the ceph cluster
	defaultStatusCheckInterval = 60 * time.Second
)

// cephStatusChecker aggregates the mon/cluster info needed to check the health of the monitors
type cephStatusChecker struct {
	context        *clusterd.Context
	resourceName   string
	interval       time.Duration
	cephUser       string
	client         client.Client
	namespacedName types.NamespacedName
}

// newCephStatusChecker creates a new HealthChecker object
func newCephStatusChecker(context *clusterd.Context, resourceName string, cephUser string, namespacedName types.NamespacedName) *cephStatusChecker {
	c := &cephStatusChecker{
		context:        context,
		resourceName:   resourceName,
		interval:       defaultStatusCheckInterval,
		cephUser:       cephUser,
		client:         context.Client,
		namespacedName: namespacedName,
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
	var status cephclient.CephStatus
	var err error

	logger.Debugf("checking health of cluster")

	// Check ceph's status
	status, err = cephclient.StatusWithUser(c.context, c.namespacedName.Namespace, c.cephUser)
	if err != nil {
		logger.Errorf("failed to get ceph status. %v", err)
		return
	}

	logger.Debugf("Cluster status: %+v", status)
	if err := c.updateCephStatus(&status); err != nil {
		logger.Errorf("failed to query cluster status in namespace %q. %v", c.namespacedName.Namespace, err)
	}
}

// updateStatus updates an object with a given status
func (c *cephStatusChecker) updateCephStatus(status *cephclient.CephStatus) error {
	cephCluster := &cephv1.CephCluster{}
	err := c.client.Get(context.TODO(), c.namespacedName, cephCluster)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephCluster resource not found. Ignoring since object must be deleted.")
			return nil
		}
		return errors.Wrapf(err, "failed to retrieve ceph cluster %q to update status to %+v", c.namespacedName.Name, status)
	}

	cephCluster.Status.CephStatus = toCustomResourceStatus(cephCluster.Status, status)
	if err := opcontroller.UpdateStatus(c.client, cephCluster); err != nil {
		return errors.Wrapf(err, "failed to update cluster %q status", c.namespacedName.Namespace)
	}

	logger.Debugf("ceph cluster %q status updated to %+v", c.namespacedName.Name, status)
	return nil
}

// toCustomResourceStatus converts the ceph status to the struct expected for the CephCluster CR status
func toCustomResourceStatus(currentStatus cephv1.ClusterStatus, newStatus *cephclient.CephStatus) *cephv1.CephStatus {
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

func (c *ClusterController) updateClusterCephVersion(image string, cephVersion cephver.CephVersion) {
	logger.Infof("cluster %q: version %q detected for image %q", c.namespacedName.Namespace, cephVersion.String(), image)

	cephCluster := &cephv1.CephCluster{}
	err := c.client.Get(context.TODO(), c.namespacedName, cephCluster)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephCluster resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Errorf("failed to retrieve ceph cluster %q to update ceph version to %+v. %v", c.namespacedName.Name, cephVersion, err)
		return
	}

	cephClusterVersion := &cephv1.ClusterVersion{
		Image:   image,
		Version: opcontroller.GetCephVersionLabel(cephVersion),
	}
	// update the Ceph version on the retrieved cluster object
	// do not overwrite the ceph status that is updated in a separate goroutine
	cephCluster.Status.CephVersion = cephClusterVersion
	if err := opcontroller.UpdateStatus(c.client, cephCluster); err != nil {
		logger.Errorf("failed to update cluster %q version. %v", c.namespacedName.Name, err)
		return
	}
}
