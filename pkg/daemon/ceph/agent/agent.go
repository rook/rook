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
	"fmt"
	"net/rpc"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/cluster"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/manager/ceph"
	"k8s.io/api/core/v1"
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
		return fmt.Errorf("failed to create volume attachment controller: %+v", err)
	}

	volumeManager := ceph.NewVolumeManager(a.context)

	flexvolumeController := flexvolume.NewController(a.context, volumeAttachmentController, volumeManager)

	flexvolumeServer := flexvolume.NewFlexvolumeServer(
		a.context,
		flexvolumeController,
		volumeManager,
	)

	err = rpc.Register(flexvolumeController)
	if err != nil {
		return fmt.Errorf("unable to register rpc: %v", err)
	}

	driverName, err := flexvolume.RookDriverName(a.context)
	if err != nil {
		return fmt.Errorf("failed to get driver name. %+v", err)
	}

	flexDriverVendors := []string{flexvolume.FlexvolumeVendor, flexvolume.FlexvolumeVendorLegacy}
	for i, vendor := range flexDriverVendors {
		if i > 0 {
			// Wait before the next driver is registered. In 1.11 and newer there is a timing issue if flex drivers are registered too quickly.
			// See https://github.com/rook/rook/issues/1501 and https://github.com/kubernetes/kubernetes/issues/60694
			time.Sleep(500 * time.Millisecond)
		}

		err = flexvolumeServer.Start(vendor, driverName)
		if err != nil {
			return fmt.Errorf("failed to start flex volume server %s/%s, %+v", vendor, driverName, err)
		}

		// Wait before the next driver is registered
		time.Sleep(500 * time.Millisecond)

		// Register drivers both with the name of the namespace and the name "rook"
		// for the volume plugins not based on the namespace.
		err = flexvolumeServer.Start(vendor, flexvolume.FlexDriverName)
		if err != nil {
			return fmt.Errorf("failed to start flex volume server %s/%s. %+v", vendor, flexvolume.FlexDriverName, err)
		}
	}

	// create a cluster controller and tell it to start watching for changes to clusters
	clusterController := cluster.NewClusterController(
		a.context,
		flexvolumeController,
		volumeAttachmentController,
		volumeManager)
	stopChan := make(chan struct{})
	clusterController.StartWatch(v1.NamespaceAll, stopChan)

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
