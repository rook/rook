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

// Package mon for the Ceph monitors.
package mon

import (
	"fmt"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
)

var (
	// HealthCheckInterval interval to check the mons to be in quorum
	HealthCheckInterval = 45 * time.Second
	// MonOutTimeout the duration to wait before removing/failover to a new mon pod
	MonOutTimeout = 300 * time.Second
)

// HealthChecker check health for the monitors
type HealthChecker struct {
	monCluster *Cluster
}

// NewHealthChecker creates a new HealthChecker object
func NewHealthChecker(monCluster *Cluster) *HealthChecker {
	return &HealthChecker{
		monCluster: monCluster,
	}
}

// Check periodically the health of the monitors
func (hc *HealthChecker) Check(stopCh chan struct{}) {
	for {
		select {
		case <-stopCh:
			logger.Infof("stopping monitoring of cluster in namespace %s", hc.monCluster.Namespace)
			return

		case <-time.After(HealthCheckInterval):
			logger.Debugf("checking health of mons")
			err := hc.monCluster.checkHealth()
			if err != nil {
				logger.Infof("failed to check mon health. %+v", err)
			}
		}
	}
}

func (c *Cluster) checkHealth() error {
	// TODO Copy old/Reimplement health check logic for StatefulSet/Pods
	logger.Debugf("Checking health for mons. %+v", c.clusterInfo)

	// connect to the mons and get the status and check for quorum
	status, err := client.GetMonStatus(c.context, c.clusterInfo.Name, true)
	if err != nil {
		return fmt.Errorf("failed to get mon status. %+v", err)
	}
	logger.Debugf("Mon status: %+v", status)

	// source of thruth of which mons should exist is our *clusterInfo*
	monsTruth := map[string]interface{}{}
	for _, mon := range c.clusterInfo.Monitors {
		monsTruth[mon.Name] = struct{}{}
	}

	// first handle mons that are not in quorum but in the cecph mon map
	// failover the unhealthy mons
	for _, mon := range status.MonMap.Mons {
		if _, ok := monsTruth[mon.Name]; !ok {
			logger.Warningf("mon %s is not in source of truth but in mon map", mon.Name)
		}

		// all mons below this line are in the source of truth, remove them from
		// the list as below we remove the mons that remained (not in quorum)
		if monInQuorum(mon, status.Quorum) {
			logger.Debugf("mon %s found in quorum", mon.Name)
			// delete the "timeout" for a mon if the pod is in quorum again
			if _, ok := c.monTimeoutList[mon.Name]; ok {
				delete(c.monTimeoutList, mon.Name)
				logger.Infof("mon %s is back in quorum again", mon.Name)
			}
		} else {
			logger.Warningf("mon %s NOT found in quorum. %+v", mon.Name, status)
			// only deal with one unhealthy mon per health check
			return nil
		}
	}

	// if there should be more mons than wanted by the user
	if len(c.clusterInfo.Monitors) > c.Size {
		// TODO scale down the StatefulSet
		if err := c.updateStatefulSet(int32(c.Size)); err != nil {
			return fmt.Errorf("failed to scale down replicas for mon statefulset to %d. %+v", c.Size, err)
		}
	} else if len(c.clusterInfo.Monitors) < c.Size {
		return c.startMons()
	}

	return nil
}

func removeMonitorFromQuorum(context *clusterd.Context, clusterName, name string) error {
	logger.Debugf("removing monitor %s from quorum", name)
	args := []string{"mon", "remove", name}
	if _, err := client.ExecuteCephCommand(context, clusterName, args); err != nil {
		return fmt.Errorf("mon %s remove failed: %+v", name, err)
	}

	logger.Infof("removed monitor %s", name)
	return nil
}
