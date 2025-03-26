/*
Copyright 2020 The Rook Authors. All rights reserved.

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

// Package cluster to manage a Ceph cluster.
package cluster

import (
	"context"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
)

var monitorDaemonList = []string{"mon", "osd", "status"}

func (c *ClusterController) configureCephMonitoring(cluster *cluster, clusterInfo *cephclient.ClusterInfo) {
	var isEnabled bool
	for _, daemon := range monitorDaemonList {
		// Is the monitoring enabled for that daemon?
		isEnabled = isMonitoringEnabled(daemon, cluster.Spec)
		if health, ok := cluster.monitoringRoutines[daemon]; ok {
			// If the context Err() is nil this means it hasn't been cancelled yet
			if health.InternalCtx.Err() == nil {
				logger.Debugf("monitoring routine for %q is already running", daemon)
				if !isEnabled {
					cluster.monitoringRoutines[daemon].InternalCancel()
				}
			}
		} else {
			if isEnabled {
				// Instantiate the monitoring goroutine context from the parent context
				// They can individually be cancelled and will be cancelled when the parent context is cancelled
				internalCtx, internalCancel := context.WithCancel(c.OpManagerCtx)

				cluster.monitoringRoutines[daemon] = &opcontroller.ClusterHealth{
					InternalCtx:    internalCtx,
					InternalCancel: internalCancel,
				}

				// Run the go routine
				c.startMonitoringCheck(cluster, clusterInfo, daemon)
			}
		}
	}
}

func isMonitoringEnabled(daemon string, clusterSpec *cephv1.ClusterSpec) bool {
	switch daemon {
	case "mon":
		return !clusterSpec.HealthCheck.DaemonHealth.Monitor.Disabled

	case "osd":
		return !clusterSpec.HealthCheck.DaemonHealth.ObjectStorageDaemon.Disabled

	case "status":
		return !clusterSpec.HealthCheck.DaemonHealth.Status.Disabled
	}

	return false
}

func (c *ClusterController) startMonitoringCheck(cluster *cluster, clusterInfo *cephclient.ClusterInfo, daemon string) {
	switch daemon {
	case "mon":
		healthChecker := mon.NewHealthChecker(cluster.mons)
		logger.Infof("enabling ceph %s monitoring goroutine for cluster %q", daemon, cluster.Namespace)
		go healthChecker.Check(cluster.monitoringRoutines, daemon)

	case "osd":
		if !cluster.Spec.External.Enable {
			c.osdChecker = osd.NewOSDHealthMonitor(c.context, clusterInfo, cluster.Spec.RemoveOSDsIfOutAndSafeToRemove, cluster.Spec.HealthCheck)
			logger.Infof("enabling ceph %s monitoring goroutine for cluster %q", daemon, cluster.Namespace)
			go c.osdChecker.Start(cluster.monitoringRoutines, daemon)
		}

	case "status":
		cephChecker := newCephStatusChecker(c.context, clusterInfo, cluster.Spec)
		logger.Infof("enabling ceph %s monitoring goroutine for cluster %q", daemon, cluster.Namespace)
		go cephChecker.checkCephStatus(cluster.monitoringRoutines, daemon)
	}
}
