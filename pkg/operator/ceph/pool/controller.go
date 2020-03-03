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
	"strconv"
	"strings"

	"github.com/coreos/pkg/capnslog"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/model"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	cephBlockPoolFinalizer = "finalizer.ceph.io"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

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
	err = c.Watch(&source.Kind{Type: &cephv1.CephBlockPool{}}, &handler.EnqueueRequestForObject{})
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
	// Make sure a CephCluster is present otherwise do nothing
	_, isReadyToReconcile, reconcileResponse := opcontroller.IsReadyToReconcile(r.client, r.context, request.NamespacedName)
	if !isReadyToReconcile {
		logger.Debugf("CephCluster resource not ready in namespace %q, retrying in %q.", request.NamespacedName.Namespace, opcontroller.WaitForRequeueIfCephClusterNotReadyAfter.String())
		return reconcileResponse, nil
	}

	// Fetch the CephBlockPool instance
	cephBlockPool := &cephv1.CephBlockPool{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cephBlockPool)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephBlockPool resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to get CephBlockPool")
	}

	// validate the pool settings
	if err := ValidatePool(r.context, cephBlockPool); err != nil {
		updateCephBlockPoolStatus(cephBlockPool.GetName(), cephBlockPool.GetNamespace(), k8sutil.ReconcileFailedStatus, r.context)
		return reconcile.Result{}, errors.Wrapf(err, "invalid pool CR %q spec", cephBlockPool.Name)
	}

	// Set a finalizer so we can do cleanup before the object goes away
	if !opcontroller.Contains(cephBlockPool.GetFinalizers(), cephBlockPoolFinalizer) {
		err := r.addFinalizer(cephBlockPool)
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to add finalizer")
		}
	}

	// DELETE: the CR was deleted
	if !cephBlockPool.GetDeletionTimestamp().IsZero() {
		logger.Debugf("deleting pool %q", cephBlockPool.Name)
		err := deletePool(r.context, cephBlockPool)
		if err != nil {
			logger.Errorf("could not delete pool %q. %v", cephBlockPool.Name, err)
			return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to delete pool %q. ", cephBlockPool.Name)
		}

		// Remove finalizer
		err = r.removeFinalizer(cephBlockPool)
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to remove finalizer")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	// CREATE
	if cephBlockPool.Status == nil || cephBlockPool.Status.Phase != k8sutil.ReadyStatus {
		reconcileResponse, err := r.reconcileCreatePool(cephBlockPool)
		if err != nil {
			return reconcileResponse, errors.Wrapf(err, "failed to create pool %q.", cephBlockPool.GetName())
		}
		return reconcile.Result{}, nil
	}

	// UPDATE
	needsUpdate, err := r.needsUpdate(cephBlockPool)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to check if the pool %q needs to be updated", cephBlockPool.GetName())
	}
	if needsUpdate {
		reconcileResponse, err := r.reconcileCreatePool(cephBlockPool)
		if err != nil {
			return reconcileResponse, errors.Wrapf(err, "failed to create pool %q.", cephBlockPool.GetName())
		}
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileCephBlockPool) reconcileCreatePool(cephBlockPool *cephv1.CephBlockPool) (reconcile.Result, error) {
	updateCephBlockPoolStatus(cephBlockPool.GetName(), cephBlockPool.GetNamespace(), k8sutil.ProcessingStatus, r.context)
	err := createPool(r.context, cephBlockPool)
	if err != nil {
		updateCephBlockPoolStatus(cephBlockPool.GetName(), cephBlockPool.GetNamespace(), k8sutil.FailedStatus, r.context)
		return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to create pool %q.", cephBlockPool.GetName())
	}

	// Set Ready status
	updateCephBlockPoolStatus(cephBlockPool.GetName(), cephBlockPool.GetNamespace(), k8sutil.ReadyStatus, r.context)

	// Let's return here so that on the initial creation we don't check for update right away
	return reconcile.Result{}, nil
}

// Create the pool
func createPool(context *clusterd.Context, p *cephv1.CephBlockPool) error {
	// create the pool
	logger.Infof("creating pool %q in namespace %q", p.Name, p.Namespace)
	if err := cephclient.CreatePoolWithProfile(context, p.Namespace, *p.Spec.ToModel(p.Name), poolApplicationNameRBD); err != nil {
		return errors.Wrapf(err, "failed to create pool %q", p.Name)
	}

	return nil
}

// Delete the pool
func deletePool(context *clusterd.Context, p *cephv1.CephBlockPool) error {
	if err := cephclient.DeletePool(context, p.Namespace, p.Name); err != nil {
		return errors.Wrapf(err, "failed to delete pool %q", p.Name)
	}

	return nil
}

// ModelToSpec reflect the internal pool struct from a pool spec
func ModelToSpec(pool model.Pool) cephv1.PoolSpec {
	ec := pool.ErasureCodedConfig
	return cephv1.PoolSpec{
		FailureDomain: pool.FailureDomain,
		CrushRoot:     pool.CrushRoot,
		DeviceClass:   pool.DeviceClass,
		Replicated:    cephv1.ReplicatedSpec{Size: pool.ReplicatedConfig.Size},
		ErasureCoded:  cephv1.ErasureCodedSpec{CodingChunks: ec.CodingChunkCount, DataChunks: ec.DataChunkCount, Algorithm: ec.Algorithm},
	}
}

// ValidatePool Validate the pool arguments
func ValidatePool(context *clusterd.Context, p *cephv1.CephBlockPool) error {
	if p.Name == "" {
		return errors.New("missing name")
	}
	if p.Namespace == "" {
		return errors.New("missing namespace")
	}
	if err := ValidatePoolSpec(context, p.Namespace, &p.Spec); err != nil {
		return err
	}
	return nil
}

// ValidatePoolSpec validates the Ceph block pool spec CR
func ValidatePoolSpec(context *clusterd.Context, namespace string, p *cephv1.PoolSpec) error {
	if p.Replication() != nil && p.ErasureCode() != nil {
		return errors.New("both replication and erasure code settings cannot be specified")
	}

	var crush cephclient.CrushMap
	var err error
	if p.FailureDomain != "" || p.CrushRoot != "" {
		crush, err = cephclient.GetCrushMap(context, namespace)
		if err != nil {
			return errors.Wrapf(err, "failed to get crush map")
		}
	}

	// validate the failure domain if specified
	if p.FailureDomain != "" {
		found := false
		for _, t := range crush.Types {
			if t.Name == p.FailureDomain {
				found = true
				break
			}
		}
		if !found {
			return errors.Errorf("unrecognized failure domain %s", p.FailureDomain)
		}
	}

	// validate the crush root if specified
	if p.CrushRoot != "" {
		found := false
		for _, t := range crush.Buckets {
			if t.Name == p.CrushRoot {
				found = true
				break
			}
		}
		if !found {
			return errors.Errorf("unrecognized crush root %s", p.CrushRoot)
		}
	}

	// validate pool replica size
	if p.Replicated.Size == 1 && p.Replicated.RequireSafeReplicaSize {
		return errors.Errorf("error pool size is %d and requireSafeReplicaSize is %t, must be false", p.Replicated.Size, p.Replicated.RequireSafeReplicaSize)
	}

	return nil
}

func (r *ReconcileCephBlockPool) needsUpdate(pool *cephv1.CephBlockPool) (bool, error) {
	var needUpdates bool
	if pool.Spec.Replicated.Size > 0 {
		replicatedPoolDetails, err := cephclient.GetPoolDetails(r.context, pool.GetNamespace(), pool.GetName())
		if err != nil {
			if strings.Contains(err.Error(), "error calling conf_read_file") {
				return false, errors.Errorf("ceph %q cluster is not ready, cannot check pool details yet.", pool.GetNamespace())
			}
			return false, errors.Wrapf(err, "failed to get pool %q details", pool.GetName())
		}

		// Was the size updated?
		if replicatedPoolDetails.Size != pool.Spec.Replicated.Size {
			logger.Infof("pool size property changed from %d to %d, updating.", replicatedPoolDetails.Size, pool.Spec.Replicated.Size)
			needUpdates = true
		}

		// Was the target_size_ratio updated?
		if replicatedPoolDetails.TargetSizeRatio != pool.Spec.Replicated.TargetSizeRatio {
			logger.Infof("pool target_size_ratio property changed from %q to %q, updating.", strconv.FormatFloat(replicatedPoolDetails.TargetSizeRatio, 'f', -1, 32), strconv.FormatFloat(pool.Spec.Replicated.TargetSizeRatio, 'f', -1, 32))
			needUpdates = true
		}

	} else {
		erasurePoolDetails, err := cephclient.GetErasureCodeProfileDetails(r.context, pool.GetNamespace(), pool.GetName())
		if err != nil {
			if strings.Contains(err.Error(), "error calling conf_read_file") {
				return false, errors.Errorf("ceph %q cluster is not ready, cannot check pool details yet.", pool.GetNamespace())
			}
			return false, errors.Wrapf(err, "failed to get pool %q details", pool.GetName())
		}
		if erasurePoolDetails.CodingChunkCount != pool.Spec.ErasureCoded.CodingChunks {
			logger.Infof("pool coding chunk count property changed from %d to %d, updating.", erasurePoolDetails.CodingChunkCount, pool.Spec.ErasureCoded.CodingChunks)
			needUpdates = true
		}
		if erasurePoolDetails.DataChunkCount != pool.Spec.ErasureCoded.DataChunks {
			logger.Infof("pool data chunk count property changed from %d to %d, updating.", erasurePoolDetails.DataChunkCount, pool.Spec.ErasureCoded.DataChunks)
			needUpdates = true
		}
	}

	return needUpdates, nil
}

func updateCephBlockPoolStatus(name, namespace, status string, context *clusterd.Context) {
	updatedCephBlockPool, err := context.RookClientset.CephV1().CephBlockPools(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("Unable to update the cephBlockPool %s status %v", updatedCephBlockPool.GetName(), err)
		return
	}
	if updatedCephBlockPool.Status == nil {
		updatedCephBlockPool.Status = &cephv1.Status{}
	} else if updatedCephBlockPool.Status.Phase == status {
		return
	}
	updatedCephBlockPool.Status.Phase = status
	_, err = context.RookClientset.CephV1().CephBlockPools(updatedCephBlockPool.Namespace).Update(updatedCephBlockPool)
	if err != nil {
		logger.Errorf("Unable to update the cephBlockPool %s status %v", updatedCephBlockPool.GetName(), err)
		return
	}
}

// addFinalizer adds a finalizer on the cluster object to avoid instant deletion
// of the object without finalizing it.
func (r *ReconcileCephBlockPool) addFinalizer(cephBlockPool *cephv1.CephBlockPool) error {
	logger.Infof("adding finalizer on %q", cephBlockPool.Name)
	cephBlockPool.SetFinalizers(append(cephBlockPool.GetFinalizers(), cephBlockPoolFinalizer))

	// Update CR with finalizer
	if err := r.client.Update(context.TODO(), cephBlockPool); err != nil {
		return errors.Wrapf(err, "failed to add finalizer on %q", cephBlockPool.Name)
	}

	return nil
}

func (r *ReconcileCephBlockPool) removeFinalizer(cephBlockPool *cephv1.CephBlockPool) error {
	logger.Infof("removing finalizer on %q", cephBlockPool.Name)

	cephBlockPool.SetFinalizers(opcontroller.Remove(cephBlockPool.GetFinalizers(), cephBlockPoolFinalizer))
	if err := r.client.Update(context.TODO(), cephBlockPool); err != nil {
		return errors.Wrapf(err, "failed to remove finalizer on %q", cephBlockPool.Name)
	}

	return nil
}
