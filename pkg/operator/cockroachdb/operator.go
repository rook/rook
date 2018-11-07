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

// Package operator to manage Kubernetes storage.
package cockroachdb

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	"github.com/rook/rook/pkg/clusterd"
	"k8s.io/api/core/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cockroachdb-operator")

type Operator struct {
	context           *clusterd.Context
	resources         []opkit.CustomResource
	rookImage         string
	clusterController *ClusterController
}

// New creates an operator instance
func New(context *clusterd.Context, rookImage string) *Operator {
	clusterController := NewClusterController(context, rookImage)

	schemes := []opkit.CustomResource{ClusterResource}
	return &Operator{
		context:           context,
		clusterController: clusterController,
		resources:         schemes,
		rookImage:         rookImage,
	}
}

// Run the operator instance
func (o *Operator) Run() error {

	signalChan := make(chan os.Signal, 1)
	stopChan := make(chan struct{})
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// watch for changes to the cockroachdb clusters
	o.clusterController.StartWatch(v1.NamespaceAll, stopChan)

	for {
		select {
		case <-signalChan:
			logger.Infof("shutdown signal received, exiting...")
			close(stopChan)
			return nil
		}
	}
}
