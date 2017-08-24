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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/

// Package operator to manage Kubernetes storage.
package operator

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/cluster"
	"github.com/rook/rook/pkg/operator/kit"
	"github.com/rook/rook/pkg/operator/pool"
	"k8s.io/api/core/v1"
)

const (
	initRetryDelay = 10 * time.Second
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "operator")

// Operator type for managing storage
type Operator struct {
	context   *clusterd.Context
	resources []kit.CustomResource
	// The custom resource that is global to the kubernetes cluster.
	// The cluster is global because you create multiple clusers in k8s
	clusterController *cluster.ClusterController
}

// New creates an operator instance
func New(context *clusterd.Context) *Operator {
	clusterController, err := cluster.NewClusterController(context)
	if err != nil {
		logger.Errorf("failed to create Operator. %+v.", err)
		return nil
	}

	schemes := []kit.CustomResource{cluster.ClusterResource, pool.PoolResource}
	return &Operator{
		context:           context,
		clusterController: clusterController,
		resources:         schemes,
	}
}

// Run the operator instance
func (o *Operator) Run() error {

	for {
		err := o.initResources()
		if err == nil {
			break
		}
		logger.Errorf("failed to init resources. %+v. retrying...", err)
		<-time.After(initRetryDelay)
	}

	signalChan := make(chan os.Signal, 1)
	stopChan := make(chan struct{})
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// watch for changes to the rook clusters
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

func (o *Operator) initResources() error {
	kitCtx := kit.Context{
		Clientset:             o.context.Clientset,
		APIExtensionClientset: o.context.APIExtensionClientset,
		Interval:              500 * time.Millisecond,
		Timeout:               60 * time.Second,
	}

	// Create and wait for CRD resources
	err := kit.CreateCustomResources(kitCtx, o.resources)
	if err != nil {
		return fmt.Errorf("failed to create custom resource. %+v", err)
	}
	return nil
}
