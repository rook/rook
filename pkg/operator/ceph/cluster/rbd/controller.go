/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package rbd

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
	controllerName = "ceph-rbd-mirror-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

// List of object resources to watch by the controller
var objectsToWatch = []client.Object{
	&v1.Service{TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: v1.SchemeGroupVersion.String()}},
	&v1.ConfigMap{TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: v1.SchemeGroupVersion.String()}},
	&appsv1.Deployment{TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: appsv1.SchemeGroupVersion.String()}},
}

var cephRBDMirrorKind = reflect.TypeOf(cephv1.CephRBDMirror{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephRBDMirrorKind,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

var currentAndDesiredCephVersion = opcontroller.CurrentAndDesiredCephVersion

// ReconcileCephRBDMirror reconciles a cephRBDMirror object
type ReconcileCephRBDMirror struct {
	context          *clusterd.Context
	clusterInfo      *cephclient.ClusterInfo
	client           client.Client
	scheme           *runtime.Scheme
	cephClusterSpec  *cephv1.ClusterSpec
	peers            map[string]*peerSpec
	opManagerContext context.Context
	opConfig         opcontroller.OperatorConfig
}

// peerSpec represents peer details
type peerSpec struct {
	info      *cephv1.PoolMirroringInfo
	poolName  string
	direction string
}

// Add creates a new cephRBDMirror Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext, opConfig))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) reconcile.Reconciler {
	return &ReconcileCephRBDMirror{
		client:           mgr.GetClient(),
		scheme:           mgr.GetScheme(),
		context:          context,
		peers:            make(map[string]*peerSpec),
		opConfig:         opConfig,
		opManagerContext: opManagerContext,
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the cephRBDMirror CRD object
	err = c.Watch(&source.Kind{Type: &cephv1.CephRBDMirror{TypeMeta: controllerTypeMeta}}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate())
	if err != nil {
		return err
	}

	// Watch all other resources
	for _, t := range objectsToWatch {
		err = c.Watch(&source.Kind{Type: t}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &cephv1.CephRBDMirror{},
		}, opcontroller.WatchPredicateForNonCRDObject(&cephv1.CephRBDMirror{TypeMeta: controllerTypeMeta}, mgr.GetScheme()))
		if err != nil {
			return err
		}
	}

	return nil
}

// Reconcile reads that state of the cluster for a cephRBDMirror object and makes changes based on the state read
// and what is in the cephRBDMirror.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephRBDMirror) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		r.updateStatus(r.client, request.NamespacedName, k8sutil.FailedStatus)
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileCephRBDMirror) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the cephRBDMirror instance
	cephRBDMirror := &cephv1.CephRBDMirror{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephRBDMirror)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("cephRBDMirror resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "failed to get cephRBDMirror")
	}

	// The CR was just created, initializing status fields
	if cephRBDMirror.Status == nil {
		r.updateStatus(r.client, request.NamespacedName, k8sutil.EmptyStatus)
	}

	// validate the pool settings
	if err := validateSpec(&cephRBDMirror.Spec); err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "invalid rbd-mirror CR %q spec", cephRBDMirror.Name)
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, _, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		logger.Debugf("CephCluster resource not ready in namespace %q, retrying in %q.", request.NamespacedName.Namespace, reconcileResponse.RequeueAfter.String())
		return reconcileResponse, nil
	}
	r.cephClusterSpec = &cephCluster.Spec

	// Populate clusterInfo
	// Always populate it during each reconcile
	r.clusterInfo, _, _, err = mon.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to populate cluster info")
	}

	// Detect desired CephCluster version
	runningCephVersion, desiredCephVersion, err := currentAndDesiredCephVersion(
		r.opManagerContext,
		r.opConfig.Image,
		cephRBDMirror.Namespace,
		controllerName,
		k8sutil.NewOwnerInfo(cephRBDMirror, r.scheme),
		r.context,
		r.cephClusterSpec,
		r.clusterInfo,
	)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return reconcile.Result{}, nil
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

	// Add bootstrap peer if any
	logger.Debug("reconciling ceph rbd mirror peers addition")
	reconcileResponse, err = r.reconcileAddBoostrapPeer(cephRBDMirror, request.NamespacedName)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to add ceph rbd mirror peer")
	}

	// CREATE/UPDATE
	logger.Debug("reconciling ceph rbd mirror deployments")
	reconcileResponse, err = r.reconcileCreateCephRBDMirror(cephRBDMirror)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to create ceph rbd mirror deployments")
	}

	// Set Ready status, we are done reconciling
	r.updateStatus(r.client, request.NamespacedName, k8sutil.ReadyStatus)

	// Return and do not requeue
	logger.Debug("done reconciling ceph rbd mirror")
	return reconcile.Result{}, nil

}

func (r *ReconcileCephRBDMirror) reconcileCreateCephRBDMirror(cephRBDMirror *cephv1.CephRBDMirror) (reconcile.Result, error) {
	if r.cephClusterSpec.External.Enable {
		_, err := opcontroller.ValidateCephVersionsBetweenLocalAndExternalClusters(r.context, r.clusterInfo)
		if err != nil {
			// This handles the case where the operator is running, the external cluster has been upgraded and a CR creation is called
			// If that's a major version upgrade we fail, if it's a minor version, we continue, it's not ideal but not critical
			return opcontroller.ImmediateRetryResult, errors.Wrap(err, "refusing to run new crd")
		}
	}

	err := r.start(cephRBDMirror)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to start rbd mirror")
	}

	return reconcile.Result{}, nil
}

// updateStatus updates an object with a given status
func (r *ReconcileCephRBDMirror) updateStatus(client client.Client, name types.NamespacedName, status string) {
	rbdMirror := &cephv1.CephRBDMirror{}
	err := client.Get(r.opManagerContext, name, rbdMirror)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephRBDMirror resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve rbd mirror %q to update status to %q. %v", name, status, err)
		return
	}

	if rbdMirror.Status == nil {
		rbdMirror.Status = &cephv1.Status{}
	}

	rbdMirror.Status.Phase = status
	if err := reporting.UpdateStatus(client, rbdMirror); err != nil {
		logger.Errorf("failed to set rbd mirror %q status to %q. %v", rbdMirror.Name, status, err)
		return
	}
	logger.Debugf("rbd mirror %q status updated to %q", name, status)
}
