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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/

// Package agent to manage Kubernetes storage attach events.
package agent

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	"github.com/rook/rook/pkg/agent/flexvolume"
	"github.com/rook/rook/pkg/agent/flexvolume/crd"
	"github.com/rook/rook/pkg/agent/flexvolume/manager/ceph"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "rook-agent")

// Agent represent all the references needed to manage a Rook agent
type Agent struct {
	context *clusterd.Context
}

// New creates an Agent instance
func New(context *clusterd.Context) *Agent {
	return &Agent{
		context: context,
	}
}

// Run the agent
func (a *Agent) Run() error {

	volumeAttachmentClient, _, err := opkit.NewHTTPClient(k8sutil.CustomResourceGroup, k8sutil.V1Alpha1, crd.SchemeBuilder)
	if err != nil {
		return fmt.Errorf("failed to create Volumeattach CRD client: %+v", err)
	}

	flexvolumeServer, err := flexvolume.NewFlexvolumeServer(
		a.context,
		volumeAttachmentClient,
		ceph.NewVolumeManager(a.context),
	)
	if err != nil {
		return err
	}

	flexvolumeServer.Start()

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM)
	for {
		select {
		case <-sigc:
			logger.Infof("shutdown signal received, exiting...")
			flexvolumeServer.Stop()
			return nil
		}
	}
}
