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

package bucket

import (
	"context"
	"os"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/object"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	controllerName = "rook-ceph-operator-bucket-controller"
)

// ReconcileBucket reconciles a ceph-csi driver
type ReconcileBucket struct {
	client           client.Client
	context          *clusterd.Context
	clusterInfo      *cephclient.ClusterInfo
	opConfig         opcontroller.OperatorConfig
	opManagerContext context.Context
}

// Add creates a new Ceph CSI Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	if os.Getenv(object.DisableOBCEnvVar) == "true" {
		logger.Info("skip running Object Bucket controller")
		return nil
	}
	return add(opManagerContext, mgr, newReconciler(mgr, context, opManagerContext, opConfig))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) reconcile.Reconciler {
	return &ReconcileBucket{
		client:           mgr.GetClient(),
		context:          context,
		opConfig:         opConfig,
		opManagerContext: opManagerContext,
	}
}

func add(ctx context.Context, mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Infof("%s successfully started", controllerName)

	// Watch for ConfigMap (operator config)
	cmKind := source.Kind[client.Object](
		mgr.GetCache(),
		&v1.ConfigMap{TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: v1.SchemeGroupVersion.String()}},
		&handler.EnqueueRequestForObject{}, predicateController(ctx, mgr.GetClient()),
	)
	err = c.Watch(cmKind)
	if err != nil {
		return err
	}

	// Watch for CephCluster
	clusterKind := source.Kind[client.Object](mgr.GetCache(),
		&cephv1.CephCluster{TypeMeta: metav1.TypeMeta{Kind: "CephCluster", APIVersion: v1.SchemeGroupVersion.String()}},
		&handler.EnqueueRequestForObject{}, predicateController(ctx, mgr.GetClient()),
	)
	err = c.Watch(clusterKind)
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the operator config map and makes changes based on the state read
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileBucket) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileBucket) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// See if there is a CephCluster
	cephCluster := &cephv1.CephCluster{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephCluster)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Infof("no ceph cluster found in %+v. not deploying the bucket provisioner", request.NamespacedName)
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to get the ceph cluster")
	}

	if !cephCluster.DeletionTimestamp.IsZero() {
		logger.Debug("ceph cluster is being deleted, no need to reconcile the bucket provisioner")
		return reconcile.Result{}, nil
	}

	if !cephCluster.Spec.External.Enable && cephCluster.Spec.CleanupPolicy.HasDataDirCleanPolicy() {
		logger.Debug("ceph cluster has cleanup policy, the cluster will soon go away, no need to reconcile the bucket provisioner")
		return reconcile.Result{}, nil
	}

	// Populate clusterInfo during each reconcile
	clusterInfo, _, _, err := opcontroller.LoadClusterInfo(r.context, r.opManagerContext, cephCluster.Namespace, &cephCluster.Spec)
	if err != nil {
		// This avoids a requeue with exponential backoff and allows the controller to reconcile
		// more quickly when the cluster is ready.
		if errors.Is(err, opcontroller.ClusterInfoNoClusterNoSecret) {
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, nil
		}
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to populate cluster info")
	}
	r.clusterInfo = clusterInfo

	// Start the object bucket provisioner
	bucketProvisioner := NewProvisioner(r.context, clusterInfo)
	// If cluster is external, pass down the user to the bucket controller

	// note: the error return below is ignored and is expected to be removed from the
	//   bucket library's `NewProvisioner` function
	bucketController, _ := NewBucketController(r.context.KubeConfig, bucketProvisioner)

	// We must run this in a go routine since RunWithContext() blocks and waits for the context to
	// be Done. However, since it has a context, the go routine will exit on reload with SIGHUP
	errChan := make(chan error)
	go func() {
		err = bucketController.RunWithContext(r.opManagerContext)
		if err != nil {
			logger.Errorf("failed to run bucket controller. %v", err)
			errChan <- err
		}
	}()

	// Check for errors when running the bucket controller
	select {
	case <-errChan:
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to run bucket controller")
	default:
		logger.Info("successfully reconciled bucket provisioner")
		return reconcile.Result{}, nil
	}
}
