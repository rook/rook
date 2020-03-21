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

Portions of this file came from https://github.com/cockroachdb/cockroach, which uses the same license.
*/

// Package cockroachdb to manage a cockroachdb cluster.
package cockroachdb

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
)

const operatorName = "cockroachdb-operator"

var logger = capnslog.NewPackageLogger("github.com/rook/rook", operatorName)

type Operator struct {
	context           *clusterd.Context
	rookImage         string
	operatorNamespace string
}

// New creates an operator instance
func New(context *clusterd.Context, rookImage string) *Operator {
	operatorNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	return &Operator{
		context:           context,
		rookImage:         rookImage,
		operatorNamespace: operatorNamespace,
	}
}

// Run the operator instance
func (o *Operator) Run() error {

	signalChan := make(chan os.Signal, 1)
	stopChan := make(chan struct{})
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	var namespaceToWatch string
	if os.Getenv("ROOK_CURRENT_NAMESPACE_ONLY") == "true" {
		logger.Infof("Watching the current namespace for a cluster CRD")
		namespaceToWatch = o.operatorNamespace
	} else {
		logger.Infof("Watching all namespaces for cluster CRDs")
		namespaceToWatch = v1.NamespaceAll
	}

	// Start the controller-runtime Manager.
	go o.startManager(namespaceToWatch, stopChan)

	for {
		select {
		case <-signalChan:
			logger.Infof("shutdown signal received, exiting...")
			close(stopChan)
			return nil
		}
	}
}
