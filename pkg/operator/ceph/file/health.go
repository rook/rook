/*
Copyright 2021 The Rook Authors. All rights reserved.

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

// Package file manages a CephFS filesystem and the required daemons.
package file

import (
	"context"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/util/log"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultHealthCheckInterval = 1 * time.Minute
)

type mirrorChecker struct {
	context        *clusterd.Context
	interval       time.Duration
	client         client.Client
	clusterInfo    *cephclient.ClusterInfo
	namespacedName types.NamespacedName
	fsSpec         *cephv1.FilesystemSpec
	fsName         string
}

// newMirrorChecker creates a new HealthChecker
func newMirrorChecker(context *clusterd.Context, client client.Client, clusterInfo *cephclient.ClusterInfo, namespacedName types.NamespacedName, fsSpec *cephv1.FilesystemSpec, fsName string) *mirrorChecker {
	c := &mirrorChecker{
		context:        context,
		interval:       defaultHealthCheckInterval,
		clusterInfo:    clusterInfo,
		namespacedName: namespacedName,
		client:         client,
		fsSpec:         fsSpec,
		fsName:         fsName,
	}

	// allow overriding the check interval
	checkInterval := fsSpec.StatusCheck.Mirror.Interval
	if checkInterval != nil {
		log.NamedInfo(namespacedName, logger, "filesystem mirroring status check interval is %q", checkInterval)
		c.interval = checkInterval.Duration
	}

	return c
}

// checkMirroring periodically checks the health of the cluster
func (c *mirrorChecker) checkMirroring(context context.Context) {
	// check the mirroring health immediately before starting the loop
	err := c.checkMirroringHealth()
	if err != nil {
		c.updateStatusMirroring(nil, nil, err.Error())
		log.NamedDebug(c.namespacedName, logger, "failed to check filesystem mirroring status. %v", err)
	}

	for {
		select {
		case <-context.Done():
			log.NamedInfo(c.namespacedName, logger, "stopping monitoring filesystem mirroring status")
			return

		case <-time.After(c.interval):
			log.NamedDebug(c.namespacedName, logger, "checking filesystem mirroring status")
			err := c.checkMirroringHealth()
			if err != nil {
				c.updateStatusMirroring(nil, nil, err.Error())
				log.NamedDebug(c.namespacedName, logger, "failed to check filesystem mirroring status. %v", err)
			}
		}
	}
}

func (c *mirrorChecker) checkMirroringHealth() error {
	mirrorStatus, err := cephclient.GetFSMirrorDaemonStatus(c.context, c.clusterInfo, c.fsName)
	if err != nil {
		c.updateStatusMirroring(nil, nil, err.Error())
		return err
	}

	var snapSchedStatus []cephv1.FilesystemSnapshotSchedulesSpec
	if c.fsSpec.Mirroring.SnapShotScheduleEnabled() {
		snapSchedStatus, err = cephclient.GetSnapshotScheduleStatus(c.context, c.clusterInfo, c.fsName)
		if err != nil {
			c.updateStatusMirroring(nil, nil, err.Error())
			return err
		}
	}

	// On success
	c.updateStatusMirroring(mirrorStatus, snapSchedStatus, "")

	return nil
}
