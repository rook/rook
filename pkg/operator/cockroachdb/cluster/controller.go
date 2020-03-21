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

// Package cluster to manage a cockroachdb cluster.
package cluster

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/cmd/rook/rook"
	cockroachdbv1alpha1 "github.com/rook/rook/pkg/apis/cockroachdb.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
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
	controllerName = "cockroachdb-cluster-controller"
	containerName  = "rook-cockroachdb-operator"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

var cephCockroachDBKind = reflect.TypeOf(cockroachdbv1alpha1.Cluster{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephCockroachDBKind,
	APIVersion: fmt.Sprintf("%s/%s", cockroachdbv1alpha1.CustomResourceGroup, cockroachdbv1alpha1.Version),
}

var _ reconcile.Reconciler = &ReconcileCockroachDBCluster{}

// Add creates a new Cluster Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context) error {
	return add(mgr, newReconciler(mgr, context))
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes on the Cluster CRD object
	err = c.Watch(&source.Kind{Type: &cockroachdbv1alpha1.Cluster{TypeMeta: controllerTypeMeta}}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate())
	if err != nil {
		return err
	}

	return nil
}

// ReconcileCockroachDBCluster reconciles a Cluster object
type ReconcileCockroachDBCluster struct {
	client                  client.Client
	scheme                  *runtime.Scheme
	context                 *clusterd.Context
	containerImage          string
	createInitRetryInterval time.Duration
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context) reconcile.Reconciler {
	// Add the cockroachdbv1alpha1 scheme to the manager scheme so that the controller knows about it
	mgrScheme := mgr.GetScheme()
	cockroachdbv1alpha1.AddToScheme(mgr.GetScheme())
	containerImage := rook.GetOperatorImage(context.Clientset, containerName)

	return &ReconcileCockroachDBCluster{
		client:                  mgr.GetClient(),
		scheme:                  mgrScheme,
		context:                 context,
		containerImage:          containerImage,
		createInitRetryInterval: createInitRetryIntervalDefault,
	}
}

// Reconcile reads that state of the cluster for a Cluster object and makes changes based on the state read
// and what is in the Cluster.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCockroachDBCluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime loggin interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileCockroachDBCluster) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Cluster instance
	cluster := &cockroachdbv1alpha1.Cluster{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cluster)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("Cluster resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrapf(err, "failed to get Cluster")
	}

	// The CR was just created, initializing status fields
	if cluster.Status == nil {
		cluster.Status = &cockroachdbv1alpha1.Status{}
		cluster.Status.Phase = k8sutil.Created
		err := opcontroller.UpdateStatus(r.client, cluster)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to set status")
		}
	}

	// // Set a finalizer so we can do cleanup before the object goes away
	// err = opcontroller.AddFinalizerIfNotPresent(r.client, cluster)
	// if err != nil {
	// 	return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer")
	// }

	// DELETE: the CR was deleted
	if !cluster.GetDeletionTimestamp().IsZero() {
		logger.Debugf("deleting cluster %q", cluster.Name)
		err := DeleteCluster(r.context, cluster)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to delete cluster %q. ", cluster.Name)
		}

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.client, cluster)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	// validate the cluster settings
	if err := ValidateCluster(r.context, cluster); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "invalid cluster CR %q spec", cluster.Name)
	}

	// Start object reconciliation, updating status for this
	cluster.Status.Phase = k8sutil.ReconcilingStatus
	err = opcontroller.UpdateStatus(r.client, cluster)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to set status")
	}

	// CREATE/UPDATE
	reconcileResponse, err := r.reconcileCreateCluster(cluster)
	if err != nil {
		cluster.Status.Phase = k8sutil.ReconcileFailedStatus
		errStatus := opcontroller.UpdateStatus(r.client, cluster)
		if errStatus != nil {
			logger.Errorf("failed to set status. %v", errStatus)
		}
		return reconcileResponse, errors.Wrapf(err, "failed to create cluster %q.", cluster.GetName())
	}

	// Set Ready status, we are done reconciling
	cluster.Status.Phase = k8sutil.ReadyStatus
	err = opcontroller.UpdateStatus(r.client, cluster)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to set status")
	}

	// Return and do not requeue
	logger.Debug("done reconciling")
	return reconcile.Result{}, nil
}

func (r *ReconcileCockroachDBCluster) reconcileCreateCluster(cluster *cockroachdbv1alpha1.Cluster) (reconcile.Result, error) {
	if err := ValidateClusterSpec(&cluster.Spec); err != nil {
		return reconcile.Result{}, fmt.Errorf("invalid cluster spec: %w", err)
	}

	ref, err := opcontroller.GetControllerObjectOwnerReference(cluster, r.scheme)
	if err != nil || ref == nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to get controller %q owner reference", cluster.Name)
	}

	err = r.CreateCluster(r.context, cluster, ref)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to create cluster %q.", cluster.GetName())
	}

	// Let's return here so that on the initial creation we don't check for update right away
	return reconcile.Result{}, nil
}

// ValidateCluster Validate the cluster arguments
func ValidateCluster(context *clusterd.Context, c *cockroachdbv1alpha1.Cluster) error {
	if c.Name == "" {
		return errors.New("missing name")
	}
	if c.Namespace == "" {
		return errors.New("missing namespace")
	}
	if err := ValidateClusterSpec(&c.Spec); err != nil {
		return err
	}
	return nil
}

// ValidateClusterSpec validates the CockroachDB Cluster spec CR
func ValidateClusterSpec(spec *cockroachdbv1alpha1.ClusterSpec) error {
	if spec.Storage.NodeCount < 1 {
		return fmt.Errorf("invalid node count: %d. Must be at least 1", spec.Storage.NodeCount)
	}

	if err := validatePercentValue(spec.CachePercent, "cache"); err != nil {
		return err
	}
	if err := validatePercentValue(spec.MaxSQLMemoryPercent, "maxSQLMemory"); err != nil {
		return err
	}

	if _, _, err := getPortsFromSpec(spec.Network); err != nil {
		return err
	}

	return nil
}

func validatePercentValue(value int, name string) error {
	if value < 0 || value > 100 {
		return fmt.Errorf("invalid value (%d) for %s percent, must be between 0 and 100 inclusive", value, name)
	}

	return nil
}
