/*
Copyright 2017 The Rook Authors. All rights reserved.

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

// Package agent to manage Kubernetes storage attach events.
package agent

import (
	"net/rpc"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/cluster"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/manager/ceph"
	"github.com/rook/rook/pkg/operator/ceph/agent"
	v1 "k8s.io/api/core/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "rook-ceph-agent")

// Agent represent all the references needed to manage a Rook agent
type Agent struct {
	context *clusterd.Context
}

// New creates an Agent instance
func New(context *clusterd.Context) *Agent {
	return &Agent{context: context}
}

// Run the agent
func (a *Agent) Run() error {
	volumeAttachmentController, err := attachment.New(a.context)
	if err != nil {
		return errors.Wrapf(err, "failed to create volume attachment controller")
	}

	volumeManager, err := ceph.NewVolumeManager(a.context)
	if err != nil {
		return errors.Wrapf(err, "failed to create volume manager")
	}

	mountSecurityMode := os.Getenv(agent.AgentMountSecurityModeEnv)
	// Don't check if it is not empty because the operator always sets it on the DaemonSet
	// meaning if it is not set, there is something wrong thus return an error.
	if mountSecurityMode == "" {
		return errors.New("no mount security mode env var found on the agent, have you upgraded your Rook operator correctly?")
	}

	flexvolumeController := flexvolume.NewController(a.context, volumeAttachmentController, volumeManager, mountSecurityMode)

	flexvolumeServer := flexvolume.NewFlexvolumeServer(
		a.context,
		flexvolumeController,
	)

	err = rpc.Register(flexvolumeController)
	if err != nil {
		return errors.Wrapf(err, "unable to register rpc")
	}

	driverName, err := flexvolume.RookDriverName(a.context)
	if err != nil {
		return errors.Wrapf(err, "failed to get driver name")
	}

	flexDriverVendors := []string{flexvolume.FlexvolumeVendor, flexvolume.FlexvolumeVendorLegacy}
	for i, vendor := range flexDriverVendors {
		if i > 0 {
			// Wait before the next driver is registered. In 1.11 and newer there is a timing issue if flex drivers are registered too quickly.
			// See https://github.com/rook/rook/issues/1501 and https://github.com/kubernetes/kubernetes/issues/60694
			time.Sleep(time.Second)
		}

		err = flexvolumeServer.Start(vendor, driverName)
		if err != nil {
			return errors.Wrapf(err, "failed to start flex volume server %s/%s", vendor, driverName)
		}

		// Wait before the next driver is registered
		time.Sleep(time.Second)

		// Register drivers both with the name of the namespace and the name "rook"
		// for the volume plugins not based on the namespace.
		err = flexvolumeServer.Start(vendor, flexvolume.FlexDriverName)
		if err != nil {
			return errors.Wrapf(err, "failed to start flex volume server %s/%s", vendor, flexvolume.FlexDriverName)
		}
	}

	// create a cluster controller and tell it to start watching for changes to clusters
	clusterController := cluster.NewClusterController(
		a.context,
		flexvolumeController,
		volumeAttachmentController)
	stopChan := make(chan struct{})
	clusterController.StartWatch(v1.NamespaceAll, stopChan)
	go periodicallyRefreshFlexDrivers(driverName, stopChan)

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM)
	for {
		select {
		case <-sigc:
			logger.Infof("shutdown signal received, exiting...")
			flexvolumeServer.StopAll()
			close(stopChan)
			return nil
		}
	}
}

// In 1.11 and newer there is a timing issue loading flex drivers.
// See https://github.com/rook/rook/issues/1501 and https://github.com/kubernetes/kubernetes/issues/60694
// With this loop we constantly make sure the flex drivers are all loaded.
func periodicallyRefreshFlexDrivers(driverName string, stopCh chan struct{}) {
	waitTime := 2 * time.Minute
	for {
		logger.Debugf("waiting %s before refreshing flex", waitTime.String())
		select {
		case <-time.After(waitTime):
			flexvolume.TouchFlexDrivers(flexvolume.FlexvolumeVendor, driverName)

			// increase the wait time after the first few times we refresh
			// at most the delay will be 32 minutes between each refresh of the flex drivers
			if waitTime < 32*time.Minute {
				waitTime = waitTime * 2
			}
			break
		case <-stopCh:
			logger.Infof("stopping flex driver refresh goroutine")
			return
		}
	}
}
