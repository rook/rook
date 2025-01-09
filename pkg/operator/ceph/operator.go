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

// Package operator to manage Kubernetes storage.
package operator

import (
	"context"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "operator")

	// ImmediateRetryResult Return this for a immediate retry of the reconciliation loop with the same request object.
	ImmediateRetryResult = reconcile.Result{Requeue: true}

	// ShutdownSignals signals to watch for to terminate the operator gracefully
	// Using os.Interrupt is more portable across platforms instead of os.SIGINT
	ShutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

	// Placeholder for the CRD manager life cycle, first we have the context to manage cancellation
	// of the manager.
	// Then we have the cancel function that we can call anything to terminate the context
	// Finally the channel to receive errors from the manager from within the go routine
	opManagerContext context.Context
	opManagerStop    context.CancelFunc
	mgrCRDErrorChan  chan error
)

// Operator type for managing storage
type Operator struct {
	context   *clusterd.Context
	resources []k8sutil.CustomResource
	config    *opcontroller.OperatorConfig
	// The custom resource that is global to the kubernetes cluster.
	// The cluster is global because you create multiple clusters in k8s
	clusterController *cluster.ClusterController
}

// New creates an operator instance
func New(context *clusterd.Context, rookImage, serviceAccount string) *Operator {
	schemes := []k8sutil.CustomResource{opcontroller.ClusterResource}

	o := &Operator{
		context:   context,
		resources: schemes,
		config: &opcontroller.OperatorConfig{
			OperatorNamespace: os.Getenv(k8sutil.PodNamespaceEnvVar),
			Image:             rookImage,
			ServiceAccount:    serviceAccount,
		},
	}
	o.clusterController = cluster.NewClusterController(context, rookImage)
	return o
}

// Run the operator instance
func (o *Operator) Run() error {
	// Initialize signal handler and context for the operator process life cycle
	// This context is used to handle the graceful termination of the operator
	operatorContext, operatorStop := signal.NotifyContext(context.Background(), ShutdownSignals...)
	defer operatorStop()

	// Start the CRD manager
	o.runCRDManager()

	// Used to watch for operator's config changes
	configChan := make(chan os.Signal, 1)
	signal.Notify(configChan, syscall.SIGHUP)

	// Main infinite loop to watch for channel events
	for {
		select {
		case <-operatorContext.Done():
			// Terminate the operator CRD manager, we cannot use "defer opManagerStop()" since
			// earlier in this code the function has not been populated yet. So explicitly calling
			// it here during the main context termination.
			opManagerStop()

			logger.Infof("shutdown signal received, exiting... %v", operatorContext.Err())
			return nil

		case <-configChan:
			logger.Info("reloading operator's CRDs manager, cancelling all orchestrations!")

			// Stop the operator CRD manager
			opManagerStop()

			// Run the operator CRD manager again
			o.runCRDManager()

		case err := <-mgrCRDErrorChan:
			return errors.Wrapf(err, "gave up to run the operator manager")
		}
	}
}

func (o *Operator) runCRDManager() {
	// Create the error channel so that the go routine can return an error
	mgrCRDErrorChan = make(chan error)

	// Create the context and the cancellation function
	opManagerContext, opManagerStop = context.WithCancel(context.Background())

	// The operator config manager is also watching for changes here so if the operator config map
	// content changes for ROOK_CURRENT_NAMESPACE_ONLY we must reload the operator CRD manager
	o.namespaceToWatch()

	// Pass the parent context to the cluster controller so that the monitoring go routines can
	// consume it to terminate gracefully
	o.clusterController.OpManagerCtx = opManagerContext

	// Run the operator CRD manager
	go o.startCRDManager(opManagerContext, mgrCRDErrorChan)

	// Run an informative go routine that prints the number of goroutines
	go func() {
		// Let's wait a bit to make sure most of the reconcilers are done
		time.Sleep(time.Minute)
		logger.Debugf("number of goroutines %d", runtime.NumGoroutine())
	}()
}

func (o *Operator) namespaceToWatch() {
	currentNamespaceOnly, _ := k8sutil.GetOperatorSetting(opManagerContext, o.context.Clientset, opcontroller.OperatorSettingConfigMapName, "ROOK_CURRENT_NAMESPACE_ONLY", "true")
	if currentNamespaceOnly == "true" {
		o.config.NamespaceToWatch = o.config.OperatorNamespace
		logger.Infof("watching the current namespace %q for Ceph CRs", o.config.OperatorNamespace)
	} else {
		o.config.NamespaceToWatch = v1.NamespaceAll
		logger.Infof("watching all namespaces for Ceph CRs")
	}
}
