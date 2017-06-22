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
package mon

import (
	"fmt"

	"github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/ceph/mon"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Cluster) CheckHealth() error {
	logger.Debugf("Checking health for mons. %+v", c.clusterInfo)

	// connect to the mons
	// get the status and check for quorum
	status, err := client.GetMonStatus(c.context, c.clusterInfo.Name)
	if err != nil {
		return fmt.Errorf("failed to get mon status. %+v", err)
	}
	logger.Debugf("Mon status: %+v", status)

	// failover the unhealthy mons
	for _, mon := range status.MonMap.Mons {
		inQuorum := monInQuorum(mon, status.Quorum)
		if inQuorum {
			logger.Debugf("mon %s found in quorum", mon.Name)
		} else {
			logger.Warningf("mon %s NOT found in quorum. %+v", mon.Name, status)

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
	mons := []*MonConfig{&MonConfig{Name: fmt.Sprintf("mon%d", c.maxMonID+1), Port: int32(mon.Port)}}
	logger.Infof("starting new mon %s", mons[0].Name)
	err := c.startPods(mons)
	if err != nil {
		return fmt.Errorf("failed to start new mon %s. %+v", mons[0].Name, err)
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
	err := c.context.Clientset.Extensions().ReplicaSets(c.Namespace).Delete(name, options)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Infof("dead mon %s was already gone", name)
		} else {
			return fmt.Errorf("failed to remove dead mon pod %s. %+v", name, err)
		}
	}

	// Remove the bad monitor from quorum
	err = mon.RemoveMonitorFromQuorum(c.context, c.clusterInfo.Name, name)
	if err != nil {
		return fmt.Errorf("failed to remove mon %s from quorum. %+v", name, err)
	}
	delete(c.clusterInfo.Monitors, name)
	err = c.saveMonConfig()
	if err != nil {
		return fmt.Errorf("failed to save mon config after failing mon %s. %+v", name, err)
	}

	return nil
}
