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
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	err = c.Watch(&source.Kind{
		Type: &v1.ConfigMap{TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: v1.SchemeGroupVersion.String()}}}, &handler.EnqueueRequestForObject{}, predicateController(ctx, mgr.GetClient()))
	if err != nil {
		return err
	}

	// Watch for CephCluster
	err = c.Watch(&source.Kind{
		Type: &cephv1.CephCluster{TypeMeta: metav1.TypeMeta{Kind: "CephCluster", APIVersion: v1.SchemeGroupVersion.String()}}}, &handler.EnqueueRequestForObject{}, predicateController(ctx, mgr.GetClient()))
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

	// Fetch the operator's configmap. We force the NamespaceName to the operator since the request
	// could be a CephCluster. If so the NamespaceName will be the one from the cluster and thus the
	// CM won't be found
	opNamespaceName := types.NamespacedName{Name: opcontroller.OperatorSettingConfigMapName, Namespace: r.opConfig.OperatorNamespace}
	opConfig := &v1.ConfigMap{}
	err = r.client.Get(r.opManagerContext, opNamespaceName, opConfig)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("operator's configmap resource not found. will use default value or env var.")
			r.opConfig.Parameters = make(map[string]string)
		} else {
			// Error reading the object - requeue the request.
			return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to get operator's configmap")
		}
	} else {
		// Populate the operator's config
		r.opConfig.Parameters = opConfig.Data
	}

	// Populate clusterInfo during each reconcile
	clusterInfo, _, _, err := opcontroller.LoadClusterInfo(r.context, r.opManagerContext, cephCluster.Namespace, cephCluster.Spec.External.Enable)
	if err != nil {
		result := opcontroller.ImmediateRetryResult
		if errors.Is(err, opcontroller.ClusterInfoNoClusterNoSecret) || errors.Is(err, opcontroller.ClusterInfoNoOperatorKeyring) {
			result = opcontroller.WaitForRequeueIfOperatorNotInitialized
		}
		return result, errors.Wrap(err, "failed to populate cluster info")
	}
	r.clusterInfo = clusterInfo

	// Start the object bucket provisioner
	bucketProvisioner := NewProvisioner(r.context, clusterInfo)
	// If cluster is external, pass down the user to the bucket controller

	// note: the error return below is ignored and is expected to be removed from the
	//   bucket library's `NewProvisioner` function
	bucketController, _ := NewBucketController(r.context.KubeConfig, bucketProvisioner, r.opConfig.Parameters)

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
