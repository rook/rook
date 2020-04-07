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
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
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
	controllerName = "ceph-nfs-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

// List of object resources to watch by the controller
var objectsToWatch = []runtime.Object{
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

// ReconcileCephNFS reconciles a cephNFS object
type ReconcileCephNFS struct {
	client          client.Client
	scheme          *runtime.Scheme
	context         *clusterd.Context
	cephClusterSpec *cephv1.ClusterSpec
	clusterInfo     *cephconfig.ClusterInfo
}

// Add creates a new cephNFS Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context) error {
	return add(mgr, newReconciler(mgr, context))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context) reconcile.Reconciler {
	// Add the cephv1 scheme to the manager scheme so that the controller knows about it
	mgrScheme := mgr.GetScheme()
	cephv1.AddToScheme(mgr.GetScheme())

	return &ReconcileCephNFS{
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

	// Build Handler function to return the list of nfs gateways
	handerFunc, err := opcontroller.ObjectToCRMapper(mgr.GetClient(), &cephv1.CephNFSList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	// Watch for changes on the CephCluster CR object
	err = c.Watch(&source.Kind{Type: &cephv1.CephCluster{}}, &handler.EnqueueRequestsFromMapFunc{ToRequests: handerFunc}, opcontroller.WatchControllerPredicate())
	if err != nil {
		return err
	}

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
func (r *ReconcileCephNFS) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime loggin interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileCephNFS) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the cephNFS instance
	cephNFS := &cephv1.CephNFS{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cephNFS)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("cephNFS resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "failed to get cephNFS")
	}

	// The CR was just created, initializing status fields
	if cephNFS.Status == nil {
		cephNFS.Status = &cephv1.Status{}
		cephNFS.Status.Phase = k8sutil.Created
		err := opcontroller.UpdateStatus(r.client, cephNFS)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to set status")
		}
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephClusterSpec, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.client, r.context, request.NamespacedName, controllerName)
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
	err = opcontroller.AddFinalizerIfNotPresent(r.client, cephNFS)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer")
	}

	// DELETE: the CR was deleted
	if !cephNFS.GetDeletionTimestamp().IsZero() {
		logger.Infof("deleting ceph nfs %q", cephNFS.Name)
		err := r.removeServersFromDatabase(cephNFS, 0)
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

	// validate the store settings
	if err := validateGanesha(r.context, cephNFS); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "invalid ceph nfs %q arguments", cephNFS.Name)
	}

	// CREATE/UPDATE
	logger.Debug("reconciling ceph nfs deployments")
	reconcileResponse, err = r.reconcileCreateCephNFS(cephNFS)
	if err != nil {
		return r.setFailedStatus(cephNFS, "failed to create ceph nfs deployments", err)
	}

	// Set Ready status, we are done reconciling
	cephNFS.Status.Phase = k8sutil.ReadyStatus
	err = opcontroller.UpdateStatus(r.client, cephNFS)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to set status")
	}

	// Return and do not requeue
	logger.Debug("done reconciling ceph nfs")
	return reconcile.Result{}, nil

}

func (r *ReconcileCephNFS) reconcileCreateCephNFS(cephNFS *cephv1.CephNFS) (reconcile.Result, error) {
	if r.cephClusterSpec.External.Enable {
		_, err := opcontroller.ValidateCephVersionsBetweenLocalAndExternalClusters(r.context, cephNFS.Namespace, r.clusterInfo.CephVersion)
		if err != nil {
			// This handles the case where the operator is running, the external cluster has been upgraded and a CR creation is called
			// If that's a major version upgrade we fail, if it's a minor version, we continue, it's not ideal but not critical
			return reconcile.Result{}, errors.Wrapf(err, "refusing to run new crd")
		}
	}

	deployments, err := r.context.Clientset.AppsV1().Deployments(cephNFS.Namespace).List(metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", AppName)})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Infof("creating ceph nfs %q", cephNFS.Name)
			err := r.upCephNFS(cephNFS, 0)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "failed to create ceph nfs %q", cephNFS.Name)
			}
		} else {
			return reconcile.Result{}, errors.Wrapf(err, "failed to list ceph nfs deployments")
		}
	} else {
		// Scale up case (CR value cephNFS.Spec.Server.Active changed)
		nfsServerListNum := len(deployments.Items)
		if nfsServerListNum < cephNFS.Spec.Server.Active {
			logger.Infof("scaling up ceph nfs %q from %d to %d", cephNFS.Name, nfsServerListNum, cephNFS.Spec.Server.Active)
			err := r.upCephNFS(cephNFS, nfsServerListNum)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "failed to scale up ceph nfs %q", cephNFS.Name)
			}
		}

		// Scale down case (CR value cephNFS.Spec.Server.Active changed)
		if nfsServerListNum > cephNFS.Spec.Server.Active {
			logger.Infof("scaling down ceph nfs %q from %d to %d", cephNFS.Name, nfsServerListNum, cephNFS.Spec.Server.Active)
			err := r.downCephNFS(cephNFS, nfsServerListNum)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "failed to scale down ceph nfs %q", cephNFS.Name)
			}
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileCephNFS) setFailedStatus(cephNFS *cephv1.CephNFS, errMessage string, err error) (reconcile.Result, error) {
	cephNFS.Status.Phase = k8sutil.ReconcileFailedStatus
	errStatus := opcontroller.UpdateStatus(r.client, cephNFS)
	if errStatus != nil {
		logger.Errorf("failed to set status. %v", errStatus)
	}

	return reconcile.Result{}, errors.Wrapf(err, "%s", errMessage)
}
