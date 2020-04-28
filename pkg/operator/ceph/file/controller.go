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

// Package file manages a CephFS filesystem and the required daemons.
package file

import (
	"context"
	"fmt"
	"reflect"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName = "ceph-file-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

// List of object resources to watch by the controller
var objectsToWatch = []runtime.Object{
	&corev1.Secret{TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: corev1.SchemeGroupVersion.String()}},
	&appsv1.Deployment{TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: appsv1.SchemeGroupVersion.String()}},
}

var cephFilesystemKind = reflect.TypeOf(cephv1.CephFilesystem{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephFilesystemKind,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

// ReconcileCephFilesystem reconciles a CephFilesystem object
type ReconcileCephFilesystem struct {
	client          client.Client
	scheme          *runtime.Scheme
	context         *clusterd.Context
	cephClusterSpec *cephv1.ClusterSpec
	clusterInfo     *cephconfig.ClusterInfo
}

// Add creates a new CephFilesystem Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context) error {
	return add(mgr, newReconciler(mgr, context))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context) reconcile.Reconciler {
	// Add the cephv1 scheme to the manager scheme so that the controller knows about it
	mgrScheme := mgr.GetScheme()
	cephv1.AddToScheme(mgr.GetScheme())

	return &ReconcileCephFilesystem{
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

	// Watch for changes on the CephFilesystem CRD object
	err = c.Watch(&source.Kind{Type: &cephv1.CephFilesystem{TypeMeta: controllerTypeMeta}}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate())
	if err != nil {
		return err
	}

	// Watch all other resources
	for _, t := range objectsToWatch {
		err = c.Watch(&source.Kind{Type: t}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &cephv1.CephFilesystem{},
		}, opcontroller.WatchPredicateForNonCRDObject(&cephv1.CephFilesystem{TypeMeta: controllerTypeMeta}, mgr.GetScheme()))
		if err != nil {
			return err
		}
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephFilesystem object and makes changes based on the state read
// and what is in the cephFilesystem.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephFilesystem) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime loggin interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileCephFilesystem) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the cephFilesystem instance
	cephFilesystem := &cephv1.CephFilesystem{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cephFilesystem)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("cephFilesystem resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "failed to get cephFilesystem")
	}

	// The CR was just created, initializing status fields
	if cephFilesystem.Status == nil {
		updateStatus(r.client, request.NamespacedName, k8sutil.Created)
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephClusterSpec, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.client, r.context, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		// We skip the deleteFilesystem() function since everything is gone already
		//
		// Also, only remove the finalizer if the CephCluster is gone
		// If not, we should wait for it to be ready
		// This handles the case where the operator is not ready to accept Ceph command but the cluster exists
		if !cephFilesystem.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Remove finalizer
			err := opcontroller.RemoveFinalizer(r.client, cephFilesystem)
			if err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, nil
		}
		return reconcileResponse, nil
	}
	r.cephClusterSpec = &cephClusterSpec

	// Populate clusterInfo
	// Always populate it during each reconcile
	clusterInfo, _, _, err := mon.LoadClusterInfo(r.context, request.NamespacedName.Namespace)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to populate cluster info")
	}
	r.clusterInfo = clusterInfo

	// Populate CephVersion
	currentCephVersion, err := cephclient.LeastUptodateDaemonVersion(r.context, r.clusterInfo.Name, opconfig.MonType)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to retrieve current ceph %q version", opconfig.MonType)
	}
	r.clusterInfo.CephVersion = currentCephVersion

	// Set a finalizer so we can do cleanup before the object goes away
	err = opcontroller.AddFinalizerIfNotPresent(r.client, cephFilesystem)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer")
	}

	// DELETE: the CR was deleted
	if !cephFilesystem.GetDeletionTimestamp().IsZero() {
		logger.Debugf("deleting filesystem %q", cephFilesystem.Name)
		err = r.reconcileDeleteFilesystem(cephFilesystem)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to delete filesystem %q. ", cephFilesystem.Name)
		}

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.client, cephFilesystem)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	// validate the filesystem settings
	if err := validateFilesystem(r.context, cephFilesystem); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "invalid object filesystem %q arguments", cephFilesystem.Name)
	}

	// RECONCILE
	logger.Debug("reconciling ceph filesystem store deployments")
	reconcileResponse, err = r.reconcileCreateFilesystem(cephFilesystem)
	if err != nil {
		updateStatus(r.client, request.NamespacedName, k8sutil.ReconcileFailedStatus)
		return reconcileResponse, err
	}

	// Set Ready status, we are done reconciling
	updateStatus(r.client, request.NamespacedName, k8sutil.ReadyStatus)

	// Return and do not requeue
	logger.Debug("done reconciling")
	return reconcile.Result{}, nil
}

func (r *ReconcileCephFilesystem) reconcileCreateFilesystem(cephFilesystem *cephv1.CephFilesystem) (reconcile.Result, error) {
	if r.cephClusterSpec.External.Enable {
		_, err := opcontroller.ValidateCephVersionsBetweenLocalAndExternalClusters(r.context, cephFilesystem.Namespace, r.clusterInfo.CephVersion)
		if err != nil {
			// This handles the case where the operator is running, the external cluster has been upgraded and a CR creation is called
			// If that's a major version upgrade we fail, if it's a minor version, we continue, it's not ideal but not critical
			return reconcile.Result{}, errors.Wrapf(err, "refusing to run new crd")
		}
	}

	// Create the controller owner ref
	// It will be associated to all resources of the CephObjectStore
	ref, err := opcontroller.GetControllerObjectOwnerReference(cephFilesystem, r.scheme)
	if err != nil || ref == nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to get controller %q owner reference", cephFilesystem.Name)
	}

	err = createFilesystem(r.clusterInfo, r.context, *cephFilesystem, r.cephClusterSpec, *ref, r.cephClusterSpec.DataDirHostPath, r.scheme)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to create filesystem %q", cephFilesystem.Name)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileCephFilesystem) reconcileDeleteFilesystem(cephFilesystem *cephv1.CephFilesystem) error {
	// Create the controller owner ref
	ref, err := opcontroller.GetControllerObjectOwnerReference(cephFilesystem, r.scheme)
	if err != nil || ref == nil {
		return errors.Wrapf(err, "failed to get controller %q owner reference", cephFilesystem.Name)
	}

	err = deleteFilesystem(r.clusterInfo, r.context, *cephFilesystem, r.cephClusterSpec, *ref, r.cephClusterSpec.DataDirHostPath, r.scheme)
	if err != nil {
		return err
	}

	return nil
}

// updateStatus updates an object with a given status
func updateStatus(client client.Client, name types.NamespacedName, status string) {
	fs := &cephv1.CephFilesystem{}
	err := client.Get(context.TODO(), name, fs)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephFilesystem resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve filesystem %q to update status to %q. %v", name, status, err)
		return
	}

	if fs.Status == nil {
		fs.Status = &cephv1.Status{}
	}

	fs.Status.Phase = status
	if err := opcontroller.UpdateStatus(client, fs); err != nil {
		logger.Errorf("failed to set filesystem %q status to %q. %v", fs.Name, status, err)
		return
	}
	logger.Debugf("filesystem %q status updated to %q", name, status)
}
