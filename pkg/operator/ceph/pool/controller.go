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

// Package pool to manage a rook pool.
package pool

import (
	"context"
	"fmt"
	"reflect"

	"github.com/coreos/pkg/capnslog"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	replicatedType         = "replicated"
	erasureCodeType        = "erasure-coded"
	poolApplicationNameRBD = "rbd"
	controllerName         = "ceph-block-pool-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

var cephBlockPoolKind = reflect.TypeOf(cephv1.CephBlockPool{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephBlockPoolKind,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

var _ reconcile.Reconciler = &ReconcileCephBlockPool{}

// ReconcileCephBlockPool reconciles a CephBlockPool object
type ReconcileCephBlockPool struct {
	client  client.Client
	scheme  *runtime.Scheme
	context *clusterd.Context
}

// Add creates a new CephBlockPool Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context) error {
	return add(mgr, newReconciler(mgr, context))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context) reconcile.Reconciler {
	// Add the cephv1 scheme to the manager scheme so that the controller knows about it
	mgrScheme := mgr.GetScheme()
	cephv1.AddToScheme(mgr.GetScheme())

	return &ReconcileCephBlockPool{
		client:  mgr.GetClient(),
		scheme:  mgrScheme,
		context: context,
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes on the CephBlockPool CRD object
	err = c.Watch(&source.Kind{Type: &cephv1.CephBlockPool{TypeMeta: controllerTypeMeta}}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate())
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephBlockPool object and makes changes based on the state read
// and what is in the CephBlockPool.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephBlockPool) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime loggin interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileCephBlockPool) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the CephBlockPool instance
	cephBlockPool := &cephv1.CephBlockPool{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cephBlockPool)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephBlockPool resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrapf(err, "failed to get CephBlockPool")
	}

	// The CR was just created, initializing status fields
	if cephBlockPool.Status == nil {
		updateStatus(r.client, request.NamespacedName, k8sutil.Created)
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.client, r.context, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		// We skip the deletePool() function since everything is gone already
		//
		// Also, only remove the finalizer if the CephCluster is gone
		// If not, we should wait for it to be ready
		// This handles the case where the operator is not ready to accept Ceph command but the cluster exists
		if !cephBlockPool.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Remove finalizer
			err = opcontroller.RemoveFinalizer(r.client, cephBlockPool)
			if err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, nil
		}
		return reconcileResponse, nil
	}

	// Set a finalizer so we can do cleanup before the object goes away
	err = opcontroller.AddFinalizerIfNotPresent(r.client, cephBlockPool)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer")
	}

	// DELETE: the CR was deleted
	if !cephBlockPool.GetDeletionTimestamp().IsZero() {
		logger.Debugf("deleting pool %q", cephBlockPool.Name)
		err := deletePool(r.context, cephBlockPool)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to delete pool %q. ", cephBlockPool.Name)
		}

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.client, cephBlockPool)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	// validate the pool settings
	if err := ValidatePool(r.context, cephBlockPool); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "invalid pool CR %q spec", cephBlockPool.Name)
	}

	updateStatus(r.client, request.NamespacedName, k8sutil.ReconcilingStatus)

	// Get CephCluster version
	cephVersion, err := opcontroller.GetImageVersion(cephCluster)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to fetch ceph version from cephcluster %q", cephCluster.Name)
	}

	// If the CephCluster has enabled the "pg_autoscaler" module and is running Nautilus
	// we force the pg_autoscale_mode to "on"
	_, propertyExists := cephBlockPool.Spec.Parameters[cephclient.PgAutoscaleModeProperty]
	if mgr.IsModuleInSpec(cephCluster.Spec.Mgr.Modules, mgr.PgautoscalerModuleName) &&
		!cephVersion.IsAtLeastOctopus() &&
		!propertyExists {
		if len(cephBlockPool.Spec.Parameters) == 0 {
			cephBlockPool.Spec.Parameters = make(map[string]string)
		}
		cephBlockPool.Spec.Parameters[cephclient.PgAutoscaleModeProperty] = cephclient.PgAutoscaleModeOn
	}

	// CREATE/UPDATE
	reconcileResponse, err = r.reconcileCreatePool(cephBlockPool)
	if err != nil {
		updateStatus(r.client, request.NamespacedName, k8sutil.ReconcileFailedStatus)
		return reconcileResponse, errors.Wrapf(err, "failed to create pool %q.", cephBlockPool.GetName())
	}

	// Set Ready status, we are done reconciling
	updateStatus(r.client, request.NamespacedName, k8sutil.ReadyStatus)

	// Return and do not requeue
	logger.Debug("done reconciling")
	return reconcile.Result{}, nil
}

func (r *ReconcileCephBlockPool) reconcileCreatePool(cephBlockPool *cephv1.CephBlockPool) (reconcile.Result, error) {
	err := createPool(r.context, cephBlockPool)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to create pool %q.", cephBlockPool.GetName())
	}

	// Let's return here so that on the initial creation we don't check for update right away
	return reconcile.Result{}, nil
}

// Create the pool
func createPool(context *clusterd.Context, p *cephv1.CephBlockPool) error {
	// create the pool
	logger.Infof("creating pool %q in namespace %q", p.Name, p.Namespace)
	if err := cephclient.CreatePoolWithProfile(context, p.Namespace, p.Name, p.Spec, poolApplicationNameRBD); err != nil {
		return errors.Wrapf(err, "failed to create pool %q", p.Name)
	}

	return nil
}

// Delete the pool
func deletePool(context *clusterd.Context, p *cephv1.CephBlockPool) error {
	pools, err := cephclient.ListPoolSummaries(context, p.Namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to list pools")
	}

	// Only delete the pool if it exists...
	for _, pool := range pools {
		if pool.Name == p.Name {
			err := cephclient.DeletePool(context, p.Namespace, p.Name)
			if err != nil {
				return errors.Wrapf(err, "failed to delete pool %q", p.Name)
			}
		}
	}

	return nil
}

// updateStatus updates a pool CR with the given status
func updateStatus(client client.Client, poolName types.NamespacedName, status string) {
	pool := &cephv1.CephBlockPool{}
	err := client.Get(context.TODO(), poolName, pool)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephBlockPool resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve pool %q to update status to %q. %v", poolName, status, err)
		return
	}

	if pool.Status == nil {
		pool.Status = &cephv1.Status{}
	}

	pool.Status.Phase = status
	if err := opcontroller.UpdateStatus(client, pool); err != nil {
		logger.Warningf("failed to set pool %q status to %q. %v", pool.Name, status, err)
		return
	}
	logger.Debugf("pool %q status updated to %q", poolName, status)
}
