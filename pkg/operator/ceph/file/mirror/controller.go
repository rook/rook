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

package mirror

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
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
	controllerName = "ceph-filesystem-mirror-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

// List of object resources to watch by the controller
var objectsToWatch = []client.Object{
	&v1.ConfigMap{TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: v1.SchemeGroupVersion.String()}},
	&v1.Secret{TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: v1.SchemeGroupVersion.String()}},
	&appsv1.Deployment{TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: appsv1.SchemeGroupVersion.String()}},
}

var cephFilesystemMirrorKind = reflect.TypeOf(cephv1.CephFilesystemMirror{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephFilesystemMirrorKind,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

// ReconcileFilesystemMirror reconciles a CephFilesystemMirror object
type ReconcileFilesystemMirror struct {
	context         *clusterd.Context
	clusterInfo     *cephclient.ClusterInfo
	client          client.Client
	scheme          *runtime.Scheme
	cephClusterSpec *cephv1.ClusterSpec
}

// Add creates a new CephFilesystemMirror Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context) error {
	return add(mgr, newReconciler(mgr, context))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context) reconcile.Reconciler {
	// Add the cephv1 scheme to the manager scheme so that the controller knows about it
	mgrScheme := mgr.GetScheme()
	if err := cephv1.AddToScheme(mgr.GetScheme()); err != nil {
		panic(err)
	}
	return &ReconcileFilesystemMirror{
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
	logger.Info("successfully started")

	// Watch for changes on the CephFilesystemMirror CRD object
	err = c.Watch(&source.Kind{Type: &cephv1.CephFilesystemMirror{TypeMeta: controllerTypeMeta}}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate())
	if err != nil {
		return err
	}

	// Watch all other resources
	for _, t := range objectsToWatch {
		err = c.Watch(&source.Kind{Type: t}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &cephv1.CephFilesystemMirror{},
		}, opcontroller.WatchPredicateForNonCRDObject(&cephv1.CephFilesystemMirror{TypeMeta: controllerTypeMeta}, mgr.GetScheme()))
		if err != nil {
			return err
		}
	}

	// Build Handler function to return the list of ceph object
	// This is used by the watchers below
	handlerFunc, err := opcontroller.ObjectToCRMapper(mgr.GetClient(), &cephv1.CephFilesystemMirrorList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	// Watch for CephCluster Spec changes that we want to propagate to us
	err = c.Watch(&source.Kind{Type: &cephv1.CephCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       opcontroller.ClusterResource.Kind,
			APIVersion: opcontroller.ClusterResource.APIVersion,
		},
	},
	}, handler.EnqueueRequestsFromMapFunc(handlerFunc), opcontroller.WatchCephClusterPredicate())
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephFilesystemMirror object and makes changes based on the state read
// and what is in the CephFilesystemMirror.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileFilesystemMirror) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		updateStatus(r.client, request.NamespacedName, k8sutil.FailedStatus)
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileFilesystemMirror) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the CephFilesystemMirror instance
	filesystemMirror := &cephv1.CephFilesystemMirror{}
	err := r.client.Get(context.TODO(), request.NamespacedName, filesystemMirror)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephFilesystemMirror resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "failed to get CephFilesystemMirror")
	}

	// The CR was just created, initializing status fields
	if filesystemMirror.Status == nil {
		updateStatus(r.client, request.NamespacedName, k8sutil.EmptyStatus)
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, _, reconcileResponse := opcontroller.IsReadyToReconcile(r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		logger.Debugf("CephCluster resource not ready in namespace %q, retrying in %q.", request.NamespacedName.Namespace, reconcileResponse.RequeueAfter.String())
		return reconcileResponse, nil
	}

	// Assign the clusterSpec
	r.cephClusterSpec = &cephCluster.Spec

	// Populate clusterInfo
	r.clusterInfo, _, _, err = mon.LoadClusterInfo(r.context, request.NamespacedName.Namespace)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to populate cluster info")
	}

	// Populate CephVersion
	daemon := string(opconfig.MonType)
	currentCephVersion, err := cephclient.LeastUptodateDaemonVersion(r.context, r.clusterInfo, daemon)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, nil
		}
		return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to retrieve current ceph %q version", daemon)
	}
	r.clusterInfo.CephVersion = currentCephVersion

	// Validate Ceph version
	if !currentCephVersion.IsAtLeastPacific() {
		return opcontroller.ImmediateRetryResult, errors.Errorf("ceph pacific version is required to deploy cephfs mirroring, current cluster runs %q", currentCephVersion.String())
	}

	// CREATE/UPDATE
	logger.Debug("reconciling ceph filesystem mirror deployments")
	reconcileResponse, err = r.reconcileFilesystemMirror(filesystemMirror)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to create ceph filesystem mirror deployments")
	}

	// Set Ready status, we are done reconciling
	updateStatus(r.client, request.NamespacedName, k8sutil.ReadyStatus)

	// Return and do not requeue
	logger.Debug("done reconciling ceph filesystem mirror")
	return reconcile.Result{}, nil

}

func (r *ReconcileFilesystemMirror) reconcileFilesystemMirror(filesystemMirror *cephv1.CephFilesystemMirror) (reconcile.Result, error) {
	if r.cephClusterSpec.External.Enable {
		_, err := opcontroller.ValidateCephVersionsBetweenLocalAndExternalClusters(r.context, r.clusterInfo)
		if err != nil {
			// This handles the case where the operator is running, the external cluster has been upgraded and a CR creation is called
			// If that's a major version upgrade we fail, if it's a minor version, we continue, it's not ideal but not critical
			return opcontroller.ImmediateRetryResult, errors.Wrap(err, "refusing to run new crd")
		}
	}

	err := r.start(filesystemMirror)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to start filesystem mirror")
	}

	return reconcile.Result{}, nil
}

// updateStatus updates an object with a given status
func updateStatus(client client.Client, name types.NamespacedName, status string) {
	fsMirror := &cephv1.CephFilesystemMirror{}
	err := client.Get(context.TODO(), name, fsMirror)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephFilesystemMirror resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve filesystem mirror %q to update status to %q. %v", name, status, err)
		return
	}

	if fsMirror.Status == nil {
		fsMirror.Status = &cephv1.Status{}
	}

	fsMirror.Status.Phase = status
	if err := reporting.UpdateStatus(client, fsMirror); err != nil {
		logger.Errorf("failed to set filesystem mirror %q status to %q. %v", fsMirror.Name, status, err)
		return
	}
	logger.Debugf("filesystem mirror %q status updated to %q", name, status)
}
