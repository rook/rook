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

package pool

import (
	"context"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	defaultHealthCheckInterval = 1 * time.Minute
)

type mirrorChecker struct {
	context        *clusterd.Context
	interval       *time.Duration
	client         client.Client
	clusterInfo    *cephclient.ClusterInfo
	namespacedName types.NamespacedName
	poolSpec       *cephv1.NamedPoolSpec
}

// newMirrorChecker creates a new HealthChecker object
func newMirrorChecker(context *clusterd.Context, client client.Client, clusterInfo *cephclient.ClusterInfo, namespacedName types.NamespacedName, poolSpec *cephv1.NamedPoolSpec) *mirrorChecker {
	c := &mirrorChecker{
		context:        context,
		interval:       &defaultHealthCheckInterval,
		clusterInfo:    clusterInfo,
		namespacedName: namespacedName,
		client:         client,
		poolSpec:       poolSpec,
	}

	// allow overriding the check interval
	checkInterval := poolSpec.StatusCheck.Mirror.Interval
	if checkInterval != nil {
		logger.Infof("pool mirroring status check interval for block pool %q is %q", namespacedName.Name, checkInterval.Duration.String())
		c.interval = &checkInterval.Duration
	}

	return c
}

// checkMirroring periodically checks the health of the cluster
func (c *mirrorChecker) checkMirroring(context context.Context) {
	// check the mirroring health immediately before starting the loop
	err := c.checkMirroringHealth()
	if err != nil {
		c.updateStatusMirroring(nil, nil, nil, err.Error())
		logger.Debugf("failed to check pool mirroring status for ceph block pool %q. %v", c.namespacedName.Name, err)
	}

	for {
		select {
		case <-context.Done():
			logger.Infof("stopping monitoring pool mirroring status %q", c.namespacedName.Name)
			return

		case <-time.After(*c.interval):
			logger.Debugf("checking pool mirroring status %q", c.namespacedName.Name)
			err := c.checkMirroringHealth()
			if err != nil {
				c.updateStatusMirroring(nil, nil, nil, err.Error())
				logger.Debugf("failed to check pool mirroring status for ceph block pool %q. %v", c.namespacedName.Name, err)
			}
		}
	}
}

func (c *mirrorChecker) checkMirroringHealth() error {
	// Check mirroring status
	mirrorStatus, err := cephclient.GetPoolMirroringStatus(c.context, c.clusterInfo, c.poolSpec.Name)
	if err != nil {
		c.updateStatusMirroring(nil, nil, nil, err.Error())
	}

	// Check mirroring info
	mirrorInfo, err := cephclient.GetPoolMirroringInfo(c.context, c.clusterInfo, c.poolSpec.Name)
	if err != nil {
		c.updateStatusMirroring(nil, nil, nil, err.Error())
	}

	// If snapshot scheduling is enabled let's add it to the status
	// snapSchedStatus := cephclient.SnapshotScheduleStatus{}
	snapSchedStatus := []cephv1.SnapshotSchedulesSpec{}
	if c.poolSpec.Mirroring.SnapshotSchedulesEnabled() {
		snapSchedStatus, err = cephclient.ListSnapshotSchedulesRecursively(c.context, c.clusterInfo, c.poolSpec.Name)
		if err != nil {
			c.updateStatusMirroring(nil, nil, nil, err.Error())
		}
	}

	// On success
	c.updateStatusMirroring(mirrorStatus.Summary, mirrorInfo, snapSchedStatus, "")

	return nil
}
