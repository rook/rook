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

// Package objectzonegroup to manage a rook object zonegroup.
package zonegroup

import (
	"context"
	"fmt"
	"reflect"
	"syscall"
	"time"

	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	controllerName = "ceph-object-zonegroup-controller"
)

var waitForRequeueIfObjectRealmNotReady = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

var cephObjectZoneGroupKind = reflect.TypeOf(cephv1.CephObjectZoneGroup{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephObjectZoneGroupKind,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

// ReconcileObjectZoneGroup reconciles a ObjectZoneGroup object
type ReconcileObjectZoneGroup struct {
	client           client.Client
	scheme           *runtime.Scheme
	context          *clusterd.Context
	clusterInfo      *cephclient.ClusterInfo
	opManagerContext context.Context
}

// Add creates a new CephObjectZoneGroup Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context) reconcile.Reconciler {
	return &ReconcileObjectZoneGroup{
		client:           mgr.GetClient(),
		scheme:           mgr.GetScheme(),
		context:          context,
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

	// Watch for changes on the CephObjectZoneGroup CRD object
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephObjectZoneGroup{TypeMeta: controllerTypeMeta},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephObjectZoneGroup]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephObjectZoneGroup](mgr.GetScheme()),
		),
	)
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephObjectZoneGroup object and makes changes based on the state read
// and what is in the CephObjectZoneGroup.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileObjectZoneGroup) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	defer opcontroller.RecoverAndLogException()
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile: %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileObjectZoneGroup) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the CephObjectZoneGroup instance
	cephObjectZoneGroup := &cephv1.CephObjectZoneGroup{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephObjectZoneGroup)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephObjectZoneGroup resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "failed to get CephObjectZoneGroup")
	}
	// update observedGeneration local variable with current generation value,
	// because generation can be changed before reconcile got completed
	// CR status will be updated at end of reconcile, so to reflect the reconcile has finished
	observedGeneration := cephObjectZoneGroup.ObjectMeta.Generation

	// The CR was just created, initializing status fields
	if cephObjectZoneGroup.Status == nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.EmptyStatus)
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		if !cephObjectZoneGroup.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, nil
		}
		return reconcileResponse, nil
	}

	// DELETE: the CR was deleted
	if !cephObjectZoneGroup.GetDeletionTimestamp().IsZero() {
		logger.Debugf("deleting zone group CR %q", cephObjectZoneGroup.Name)

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	// Populate clusterInfo during each reconcile
	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace, &cephCluster.Spec)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to populate cluster info")
	}

	// validate the zone group settings
	err = validateZoneGroup(cephObjectZoneGroup)
	if err != nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcileFailedStatus)
		return reconcile.Result{}, errors.Wrapf(err, "invalid CephObjectZoneGroup CR %q", cephObjectZoneGroup.Name)
	}

	// Start object reconciliation, updating status for this
	r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcilingStatus)

	// Make sure an ObjectRealm Resource is present
	reconcileResponse, err = r.reconcileObjectRealm(cephObjectZoneGroup)
	if err != nil {
		return reconcileResponse, err
	}

	// Make sure Realm has been created in Ceph Cluster
	reconcileResponse, err = r.reconcileCephRealm(cephObjectZoneGroup)
	if err != nil {
		return reconcileResponse, err
	}

	// Create/Update Ceph Zone Group
	_, err = r.createCephZoneGroup(cephObjectZoneGroup)
	if err != nil {
		return r.setFailedStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, "failed to create ceph zone group", err)
	}

	// update ObservedGeneration in status at the end of reconcile
	// Set Ready status, we are done reconciling
	r.updateStatus(observedGeneration, request.NamespacedName, k8sutil.ReadyStatus)

	// Return and do not requeue
	logger.Debug("zone group done reconciling")
	return reconcile.Result{}, nil
}

func (r *ReconcileObjectZoneGroup) createCephZoneGroup(zoneGroup *cephv1.CephObjectZoneGroup) (reconcile.Result, error) {
	logger.Infof("creating object zone group %q in realm %q", zoneGroup.Name, zoneGroup.Spec.Realm)

	realmArg := fmt.Sprintf("--rgw-realm=%s", zoneGroup.Spec.Realm)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", zoneGroup.Name)
	objContext := object.NewContext(r.context, r.clusterInfo, zoneGroup.Name)

	// get period to see if master zone group exists yet
	output, err := object.RunAdminCommandNoMultisite(objContext, true, "period", "get", realmArg)
	if err != nil {
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
			return reconcile.Result{}, errors.Wrapf(err, "ceph period %q not found", zoneGroup.Spec.Realm)
		} else {
			return reconcile.Result{}, errors.Wrapf(err, "radosgw-admin period get failed with code %d", code)
		}
	}

	// check if master zone group does not exist yet for period
	masterZoneGroup, err := decodeMasterZoneGroup(output)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to parse `radosgw-admin period get` output")
	}

	zoneGroupIsMaster := false
	if masterZoneGroup == "" {
		zoneGroupIsMaster = true
	}

	// create zone group
	output, err = object.RunAdminCommandNoMultisite(objContext, true, "zonegroup", "get", realmArg, zoneGroupArg)
	if err == nil {
		return reconcile.Result{}, nil
	}

	if code, ok := exec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
		logger.Debugf("ceph zone group %q not found, running `radosgw-admin zonegroup create`", zoneGroup.Name)
		args := []string{
			"zonegroup",
			"create",
			realmArg,
			zoneGroupArg,
		}

		if zoneGroupIsMaster {
			// master zone group does not exist yet for realm
			args = append(args, "--master")
		}

		output, err = object.RunAdminCommandNoMultisite(objContext, false, args...)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to create ceph zone group %q for reason %q", zoneGroup.Name, output)
		}
	} else {
		return reconcile.Result{}, errors.Wrapf(err, "radosgw-admin zonegroup get failed with code %d for reason %q", code, output)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileObjectZoneGroup) reconcileObjectRealm(zoneGroup *cephv1.CephObjectZoneGroup) (reconcile.Result, error) {
	// Verify the object realm API object actually exists
	cephObjectRealm := &cephv1.CephObjectRealm{}
	err := r.client.Get(r.opManagerContext, types.NamespacedName{Name: zoneGroup.Spec.Realm, Namespace: zoneGroup.Namespace}, cephObjectRealm)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return waitForRequeueIfObjectRealmNotReady, errors.Wrapf(err, "realm %q not found", zoneGroup.Spec.Realm)
		}
		return waitForRequeueIfObjectRealmNotReady, errors.Wrapf(err, "error finding CephObjectRealm %s", zoneGroup.Spec.Realm)
	}

	logger.Infof("CephObjectRealm %q found for CephObjectZoneGroup %q", zoneGroup.Spec.Realm, zoneGroup.Name)
	return reconcile.Result{}, nil
}

func (r *ReconcileObjectZoneGroup) reconcileCephRealm(zoneGroup *cephv1.CephObjectZoneGroup) (reconcile.Result, error) {
	realmArg := fmt.Sprintf("--rgw-realm=%s", zoneGroup.Spec.Realm)
	objContext := object.NewContext(r.context, r.clusterInfo, zoneGroup.Name)

	_, err := object.RunAdminCommandNoMultisite(objContext, true, "realm", "get", realmArg)
	if err != nil {
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
			return waitForRequeueIfObjectRealmNotReady, errors.Wrapf(err, "ceph realm %q not found", zoneGroup.Spec.Realm)
		} else {
			return waitForRequeueIfObjectRealmNotReady, errors.Wrapf(err, "radosgw-admin realm get failed with code %d", code)
		}
	}

	logger.Infof("Realm %q found in Ceph cluster to create ceph zone group %q", zoneGroup.Spec.Realm, zoneGroup.Name)
	return reconcile.Result{}, nil
}

func (r *ReconcileObjectZoneGroup) setFailedStatus(observedGeneration int64, name types.NamespacedName, errMessage string, err error) (reconcile.Result, error) {
	r.updateStatus(observedGeneration, name, k8sutil.ReconcileFailedStatus)
	return reconcile.Result{}, errors.Wrapf(err, "%s", errMessage)
}

// updateStatus updates an zone group with a given status
func (r *ReconcileObjectZoneGroup) updateStatus(observedGeneration int64, name types.NamespacedName, status string) {
	objectZoneGroup := &cephv1.CephObjectZoneGroup{}
	if err := r.client.Get(r.opManagerContext, name, objectZoneGroup); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephObjectZoneGroup resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve object zone group %q to update status to %q. %v", name, status, err)
		return
	}
	if objectZoneGroup.Status == nil {
		objectZoneGroup.Status = &cephv1.Status{}
	}

	objectZoneGroup.Status.Phase = status
	if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
		objectZoneGroup.Status.ObservedGeneration = observedGeneration
	}
	if err := reporting.UpdateStatus(r.client, objectZoneGroup); err != nil {
		logger.Errorf("failed to set object zone group %q status to %q. %v", name, status, err)
		return
	}
	logger.Debugf("object zone group %q status updated to %q", name, status)
}
