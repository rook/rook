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
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/object/bucket"
)

var (
	monitorDaemonList = []string{"mon", "osd", "status"}
)

func (c *ClusterController) configureCephMonitoring(cluster *cluster, clusterInfo *cephclient.ClusterInfo) {
	var isDisabled bool

	for _, daemon := range monitorDaemonList {
		// Is the monitoring enabled for that daemon?
		isDisabled = isMonitoringDisabled(daemon, cluster.Spec)
		if health, ok := cluster.monitoringChannels[daemon]; ok {
			if health.monitoringRunning {
				// If the goroutine was running but the CR was updated to stop the monitoring we need to close the channel
				if isDisabled {
					// close the channel so the goroutine can stop
					close(cluster.monitoringChannels[daemon].stopChan)
					// Set monitoring to false since it's not running anymore
					cluster.monitoringChannels[daemon].monitoringRunning = false
				} else {
					logger.Debugf("ceph %s health go routine is already running for cluster %q", daemon, cluster.Namespace)
				}
			} else {
				// if not already running and not disabled, we run it
				if !isDisabled {
					// Run the go routine
					c.startMonitoringCheck(cluster, clusterInfo, daemon)

					// Set the flag to indicate monitoring is running
					cluster.monitoringChannels[daemon].monitoringRunning = true
				}
			}
		} else {
			if !isDisabled {
				cluster.monitoringChannels[daemon] = &clusterHealth{
					stopChan:          make(chan struct{}),
					monitoringRunning: true, // Set the flag to indicate monitoring is running
				}

				// Run the go routine
				c.startMonitoringCheck(cluster, clusterInfo, daemon)
			}
		}
	}

	// Start watchers
	if cluster.watchersActivated {
		logger.Debugf("cluster is already being watched by bucket and client provisioner for cluster %q", cluster.Namespace)
		return
	}

	// Start the object bucket provisioner
	bucketProvisioner := bucket.NewProvisioner(c.context, clusterInfo)
	// If cluster is external, pass down the user to the bucket controller

	// note: the error return below is ignored and is expected to be removed from the
	//   bucket library's `NewProvisioner` function
	bucketController, _ := bucket.NewBucketController(c.context.KubeConfig, bucketProvisioner)
	go func() {
		err := bucketController.Run(cluster.stopCh)
		if err != nil {
			logger.Errorf("failed to run bucket controller. %v", err)
		}
	}()

	// enable the cluster watcher once
	cluster.watchersActivated = true
}

func isMonitoringDisabled(daemon string, clusterSpec *cephv1.ClusterSpec) bool {
	switch daemon {
	case "mon":
		return clusterSpec.HealthCheck.DaemonHealth.Monitor.Disabled

	case "osd":
		return clusterSpec.HealthCheck.DaemonHealth.ObjectStorageDaemon.Disabled

	case "status":
		return clusterSpec.HealthCheck.DaemonHealth.Status.Disabled
	}

	return false
}

func (c *ClusterController) startMonitoringCheck(cluster *cluster, clusterInfo *cephclient.ClusterInfo, daemon string) {
	switch daemon {
	case "mon":
		healthChecker := mon.NewHealthChecker(cluster.mons)
		logger.Infof("enabling ceph %s monitoring goroutine for cluster %q", daemon, cluster.Namespace)
		go healthChecker.Check(cluster.monitoringChannels[daemon].stopChan)

	case "osd":
		if !cluster.Spec.External.Enable {
			c.osdChecker = osd.NewOSDHealthMonitor(c.context, clusterInfo, cluster.Spec.RemoveOSDsIfOutAndSafeToRemove, cluster.Spec.HealthCheck)
			logger.Infof("enabling ceph %s monitoring goroutine for cluster %q", daemon, cluster.Namespace)
			go c.osdChecker.Start(cluster.monitoringChannels[daemon].stopChan)
		}

	case "status":
		cephChecker := newCephStatusChecker(c.context, clusterInfo, cluster.Spec)
		logger.Infof("enabling ceph %s monitoring goroutine for cluster %q", daemon, cluster.Namespace)
		go cephChecker.checkCephStatus(cluster.monitoringChannels[daemon].stopChan)
	}
}
