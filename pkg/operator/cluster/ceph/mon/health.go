/*
Copyright 2016 The Rook Authors. All rights reserved.

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
	"time"
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
	return nil
}
