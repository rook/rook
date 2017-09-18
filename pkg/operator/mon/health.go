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
	"fmt"
	"time"

	"github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/ceph/mon"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// HealthCheckInterval interval to check the mons to be in quorum
	HealthCheckInterval = 20 * time.Second
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
			logger.Infof("Stopping monitoring of cluster in namespace %s", hc.monCluster.Namespace)
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
	logger.Debugf("Checking health for mons. %+v", c.clusterInfo)

	// connect to the mons
	// get the status and check for quorum
	status, err := client.GetMonStatus(c.context, c.clusterInfo.Name, true)
	if err != nil {
		return fmt.Errorf("failed to get mon status. %+v", err)
	}
	logger.Debugf("Mon status: %+v", status)

	// failover the unhealthy mons
	for _, mon := range status.MonMap.Mons {
		inQuorum := monInQuorum(mon, status.Quorum)
		if inQuorum {
			logger.Debugf("mon %s found in quorum", mon.Name)
			// delete the "timeout" for a mon if the pod is in quorum again
			if _, ok := c.monTimeoutList[mon.Name]; ok {
				delete(c.monTimeoutList, mon.Name)
			}
		} else {
			logger.Warningf("mon %s NOT found in quorum. %+v", mon.Name, status)

			// If not yet set, add the current time, for the timeout
			// calculation, to the list
			if _, ok := c.monTimeoutList[mon.Name]; !ok {
				c.monTimeoutList[mon.Name] = time.Now()
			}

			// when the timeout for the mon has been reached, continue to the
			// normal failover/delete mon pod part of the code
			if time.Since(c.monTimeoutList[mon.Name]) <= MonOutTimeout {
				logger.Warningf("mon %s NOT found in quorum, STILL in mon out timeout", mon.Name)
				continue
			}

			if len(status.MonMap.Mons) > c.Size {
				// no need to create a new mon since we have an extra
				err = c.removeMon(mon.Name)
				if err != nil {
					logger.Errorf("failed to remove mon %s. %+v", mon.Name, err)
				}
			} else {
				// bring up a new mon to replace the unhealthy mon
				err = c.failoverMon(mon.Name)
				if err != nil {
					logger.Errorf("failed to failover mon %s. %+v", mon.Name, err)
				}
			}
			// only deal with one unhealthy mon per health check
			return nil
		}
	}

	return nil
}

func (c *Cluster) failoverMon(name string) error {
	logger.Infof("Failing over monitor %s", name)

	// Start a new monitor
	m := &monConfig{Name: fmt.Sprintf("%s%d", appName, c.maxMonID+1), Port: int32(mon.DefaultPort)}
	logger.Infof("starting new mon %s", m.Name)

	// Create the service endpoint
	serviceIP, err := c.createService(m)
	if err != nil {
		return fmt.Errorf("failed to create mon service. %+v", err)
	}

	if !c.HostNetwork {
		m.PublicIP = serviceIP
		c.clusterInfo.Monitors[m.Name] = mon.ToCephMon(m.Name, m.PublicIP, m.Port)
	}

	mConf := []*monConfig{m}

	// Assign the pod to a node
	if err = c.assignMons(mConf); err != nil {
		return fmt.Errorf("failed to assign pods to mons. %+v", err)
	}

	// Start the pod
	if err = c.startPods(mConf); err != nil {
		return fmt.Errorf("failed to start new mon %s. %+v", m.Name, err)
	}

	// Only increment the max mon id if the new pod started successfully
	c.maxMonID++

	return c.removeMon(name)
}

func (c *Cluster) removeMon(name string) error {
	logger.Infof("ensuring removal of unhealthy monitor %s", name)

	// Remove the mon pod if it is still there
	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}
	if err := c.context.Clientset.Extensions().ReplicaSets(c.Namespace).Delete(name, options); err != nil {
		if errors.IsNotFound(err) {
			logger.Infof("dead mon %s was already gone", name)
		} else {
			return fmt.Errorf("failed to remove dead mon pod %s. %+v", name, err)
		}
	}

	// Remove the bad monitor from quorum
	if err := mon.RemoveMonitorFromQuorum(c.context, c.clusterInfo.Name, name); err != nil {
		return fmt.Errorf("failed to remove mon %s from quorum. %+v", name, err)
	}
	delete(c.clusterInfo.Monitors, name)
	nodeName := c.mapping.Node[name].Name
	// if node->port "mapping" has been created, decrease or delete it
	if port, ok := c.mapping.Port[nodeName]; ok {
		if port == mon.DefaultPort {
			delete(c.mapping.Port, nodeName)
		}
		// don't clean up if a node port is higher than the default port, other
		//mons could be on the same node with > DefaultPort ports, decreasing could
		//cause port collsions
		// This can be solved by using a map[nodeName][]int32 for the ports to
		//even better check which ports are open for the HostNetwork mode
	}
	delete(c.mapping.Node, name)

	// Remove the service endpoint
	if err := c.context.Clientset.CoreV1().Services(c.Namespace).Delete(name, options); err != nil {
		if errors.IsNotFound(err) {
			logger.Infof("dead mon service %s was already gone", name)
		} else {
			return fmt.Errorf("failed to remove dead mon pod %s. %+v", name, err)
		}
	}

	if err := c.saveMonConfig(); err != nil {
		return fmt.Errorf("failed to save mon config after failing over mon %s. %+v", name, err)
	}

	// make sure to rewrite the config so NO new connections are made to the removed mon
	if err := c.writeConnectionConfig(); err != nil {
		return fmt.Errorf("failed to write connection config after failing over mon %s. %+v", name, err)
	}

	return nil
}
