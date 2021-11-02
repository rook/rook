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

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
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

// newChecker creates a new HealthChecker
func newChecker(context *clusterd.Context, client client.Client, clusterInfo *cephclient.ClusterInfo, namespacedName types.NamespacedName, fsSpec *cephv1.FilesystemSpec, fsName string) *mirrorChecker {
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
		logger.Infof("filesystem %q mirroring status check interval is %q", namespacedName.Name, checkInterval)
		c.interval = checkInterval.Duration
	}

	return c
}

// checkHealth periodically checks the health of the cluster
func (c *mirrorChecker) checkHealth(context context.Context) {
	// check the mirroring health immediately before starting the loop
	err := c.checkFilesystemHealth()
	if err != nil {
		c.updateStatusMirroring(nil, nil, nil, err.Error())
		logger.Debugf("failed to check filesystem mirroring status %q. %v", c.namespacedName.Name, err)
	}

	for {
		select {
		case <-context.Done():
			logger.Infof("stopping monitoring filesystem mirroring status %q", c.namespacedName.Name)
			return

		case <-time.After(c.interval):
			logger.Debugf("checking filesystem mirroring status %q", c.namespacedName.Name)
			err := c.checkFilesystemHealth()
			if err != nil {
				c.updateStatusMirroring(nil, nil, nil, err.Error())
				logger.Debugf("failed to check filesystem %q mirroring status. %v", c.namespacedName.Name, err)
			}
		}
	}
}

func (c *mirrorChecker) checkFilesystemHealth() error {
	var err error
	var snapSchedStatus []cephv1.FilesystemSnapshotSchedulesSpec
	var mirrorStatus []cephv1.FilesystemMirroringInfo
	var perfStats *cephv1.FilesystemStats

	if c.fsSpec.EnablePerfStats {
		isPerfModuleEnabled, err := cephclient.IsModuleEnabled(c.context, c.clusterInfo, cephclient.FilesystemPerfModuleName)
		if err != nil {
			return errors.Wrapf(err, "failed to check whether the mgr %q module is enabled", cephclient.FilesystemPerfModuleName)
		}

		if !isPerfModuleEnabled {
			err := cephclient.MgrEnableModule(c.context, c.clusterInfo, cephclient.FilesystemPerfModuleName, false)
			if err != nil {
				return errors.Wrapf(err, "failed to enable mgr module %q to collect filesystem performance metrics", cephclient.FilesystemPerfModuleName)
			}
		}

		perfStats, err = cephclient.GetPerfStats(c.context, c.clusterInfo)
		if err != nil {
			return errors.Wrap(err, "failed to get filesystem performance statistics")
		}
	}

	if c.fsSpec.Mirroring != nil {
		if c.fsSpec.Mirroring.Enabled {
			mirrorStatus, err = cephclient.GetFSMirrorDaemonStatus(c.context, c.clusterInfo, c.fsName)
			if err != nil {
				c.updateStatusMirroring(nil, nil, nil, err.Error())
				return err
			}

			if c.fsSpec.Mirroring.SnapShotScheduleEnabled() {
				snapSchedStatus, err = cephclient.GetSnapshotScheduleStatus(c.context, c.clusterInfo, c.fsName)
				if err != nil {
					c.updateStatusMirroring(nil, nil, nil, err.Error())
					return err
				}
			}
		}
	}

	// On success
	c.updateStatusMirroring(mirrorStatus, snapSchedStatus, perfStats, "")

	return nil
}
