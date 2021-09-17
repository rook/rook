/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package nfs

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
	"github.com/rook/rook/pkg/operator/ceph/config"
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
	controllerName = "ceph-nfs-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

// List of object resources to watch by the controller
var objectsToWatch = []client.Object{
	&v1.Service{TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: v1.SchemeGroupVersion.String()}},
	&v1.ConfigMap{TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: v1.SchemeGroupVersion.String()}},
	&appsv1.Deployment{TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: appsv1.SchemeGroupVersion.String()}},
}

var cephNFSKind = reflect.TypeOf(cephv1.CephNFS{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephNFSKind,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

var currentAndDesiredCephVersion = opcontroller.CurrentAndDesiredCephVersion

// ReconcileCephNFS reconciles a cephNFS object
type ReconcileCephNFS struct {
	client           client.Client
	scheme           *runtime.Scheme
	context          *clusterd.Context
	cephClusterSpec  *cephv1.ClusterSpec
	clusterInfo      *cephclient.ClusterInfo
	opManagerContext context.Context
	opConfig         opcontroller.OperatorConfig
}

// Add creates a new cephNFS Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext, opConfig))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) reconcile.Reconciler {
	return &ReconcileCephNFS{
		client:           mgr.GetClient(),
		scheme:           mgr.GetScheme(),
		context:          context,
		opManagerContext: opManagerContext,
		opConfig:         opConfig,
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the cephNFS CRD object
	err = c.Watch(&source.Kind{Type: &cephv1.CephNFS{TypeMeta: controllerTypeMeta}}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate())
	if err != nil {
		return err
	}

	// Watch all other resources
	for _, t := range objectsToWatch {
		err = c.Watch(&source.Kind{Type: t}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &cephv1.CephNFS{},
		}, opcontroller.WatchPredicateForNonCRDObject(&cephv1.CephNFS{TypeMeta: controllerTypeMeta}, mgr.GetScheme()))
		if err != nil {
			return err
		}
	}

	return nil
}

// Reconcile reads that state of the cluster for a cephNFS object and makes changes based on the state read
// and what is in the cephNFS.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephNFS) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileCephNFS) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the cephNFS instance
	cephNFS := &cephv1.CephNFS{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephNFS)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("cephNFS resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "failed to get cephNFS")
	}

	// Set a finalizer so we can do cleanup before the object goes away
	err = opcontroller.AddFinalizerIfNotPresent(r.client, cephNFS)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer")
	}

	// The CR was just created, initializing status fields
	if cephNFS.Status == nil {
		updateStatus(r.client, request.NamespacedName, k8sutil.EmptyStatus)
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.client, r.context, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		// We skip the deleteStore() function since everything is gone already
		//
		// Also, only remove the finalizer if the CephCluster is gone
		// If not, we should wait for it to be ready
		// This handles the case where the operator is not ready to accept Ceph command but the cluster exists
		if !cephNFS.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Remove finalizer
			err := opcontroller.RemoveFinalizer(r.client, cephNFS)
			if err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, nil
		}
		return reconcileResponse, nil
	}
	r.cephClusterSpec = &cephCluster.Spec

	// Populate clusterInfo
	// Always populate it during each reconcile
	r.clusterInfo, _, _, err = mon.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to populate cluster info")
	}

	// DELETE: the CR was deleted
	if !cephNFS.GetDeletionTimestamp().IsZero() {
		logger.Infof("deleting ceph nfs %q", cephNFS.Name)

		// Detect running Ceph version
		runningCephVersion, err := cephclient.LeastUptodateDaemonVersion(r.context, r.clusterInfo, config.MonType)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to retrieve current ceph %q version", config.MonType)
		}
		r.clusterInfo.CephVersion = runningCephVersion

		err = r.removeServersFromDatabase(cephNFS, 0)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to delete filesystem %q. ", cephNFS.Name)
		}

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.client, cephNFS)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	// Detect desired CephCluster version
	runningCephVersion, desiredCephVersion, err := currentAndDesiredCephVersion(
		r.opConfig.Image,
		cephNFS.Namespace,
		controllerName,
		k8sutil.NewOwnerInfo(cephNFS, r.scheme),
		r.context,
		r.cephClusterSpec,
		r.clusterInfo,
	)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, nil
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to detect running and desired ceph version")
	}

	// If the version of the Ceph monitor differs from the CephCluster CR image version we assume
	// the cluster is being upgraded. So the controller will just wait for the upgrade to finish and
	// then versions should match. Obviously using the cmd reporter job adds up to the deployment time
	if !reflect.DeepEqual(runningCephVersion, desiredCephVersion) {
		// Upgrade is in progress, let's wait for the mons to be done
		return opcontroller.WaitForRequeueIfCephClusterIsUpgrading,
			opcontroller.ErrorCephUpgradingRequeue(desiredCephVersion, runningCephVersion)
	}
	r.clusterInfo.CephVersion = *runningCephVersion

	// validate the store settings
	if err := validateGanesha(r.context, r.clusterInfo, cephNFS); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "invalid ceph nfs %q arguments", cephNFS.Name)
	}

	// CREATE/UPDATE
	logger.Debug("reconciling ceph nfs deployments")
	_, err = r.reconcileCreateCephNFS(cephNFS)
	if err != nil {
		updateStatus(r.client, request.NamespacedName, k8sutil.FailedStatus)
		return reconcile.Result{}, errors.Wrap(err, "failed to create ceph nfs deployments")
	}

	// Set Ready status, we are done reconciling
	updateStatus(r.client, request.NamespacedName, k8sutil.ReadyStatus)

	// Return and do not requeue
	logger.Debug("done reconciling ceph nfs")
	return reconcile.Result{}, nil

}

func (r *ReconcileCephNFS) reconcileCreateCephNFS(cephNFS *cephv1.CephNFS) (reconcile.Result, error) {
	if r.cephClusterSpec.External.Enable {
		_, err := opcontroller.ValidateCephVersionsBetweenLocalAndExternalClusters(r.context, r.clusterInfo)
		if err != nil {
			// This handles the case where the operator is running, the external cluster has been upgraded and a CR creation is called
			// If that's a major version upgrade we fail, if it's a minor version, we continue, it's not ideal but not critical
			return reconcile.Result{}, errors.Wrap(err, "refusing to run new crd")
		}
	}

	deployments, err := r.context.Clientset.AppsV1().Deployments(cephNFS.Namespace).List(r.opManagerContext, metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", AppName)})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Infof("creating ceph nfs %q", cephNFS.Name)
			err := r.upCephNFS(cephNFS)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "failed to create ceph nfs %q", cephNFS.Name)
			}
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to list ceph nfs deployments")
	}

	nfsServerListNum := len(deployments.Items)
	// Scale down case (CR value cephNFS.Spec.Server.Active changed)
	if nfsServerListNum > cephNFS.Spec.Server.Active {
		logger.Infof("scaling down ceph nfs %q from %d to %d", cephNFS.Name, nfsServerListNum, cephNFS.Spec.Server.Active)
		err := r.downCephNFS(cephNFS, nfsServerListNum)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to scale down ceph nfs %q", cephNFS.Name)
		}
	}
	// Update existing deployments and create new ones in the scale up case
	logger.Infof("updating ceph nfs %q", cephNFS.Name)
	err = r.upCephNFS(cephNFS)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to update ceph nfs %q", cephNFS.Name)
	}

	return reconcile.Result{}, nil
}

// updateStatus updates an object with a given status
func updateStatus(client client.Client, name types.NamespacedName, status string) {
	nfs := &cephv1.CephNFS{}
	err := client.Get(context.TODO(), name, nfs)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephNFS resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve nfs %q to update status to %q. %v", name, status, err)
		return
	}
	if nfs.Status == nil {
		nfs.Status = &cephv1.Status{}
	}

	nfs.Status.Phase = status
	if err := reporting.UpdateStatus(client, nfs); err != nil {
		logger.Errorf("failed to set nfs %q status to %q. %v", nfs.Name, status, err)
	}
	logger.Debugf("nfs %q status updated to %q", name, status)
}
