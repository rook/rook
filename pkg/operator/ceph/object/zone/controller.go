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

// Package zone to manage a rook object zone.
package zone

import (
	"context"
	"fmt"
	"reflect"
	"syscall"
	"time"

	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
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
	"github.com/rook/rook/pkg/operator/ceph/pool"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	controllerName = "ceph-object-zone-controller"
)

var waitForRequeueIfObjectZoneGroupNotReady = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

var cephObjectZoneKind = reflect.TypeOf(cephv1.CephObjectZone{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephObjectZoneKind,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

// ReconcileObjectZone reconciles a ObjectZone object
type ReconcileObjectZone struct {
	client           client.Client
	scheme           *runtime.Scheme
	context          *clusterd.Context
	clusterInfo      *cephclient.ClusterInfo
	clusterSpec      *cephv1.ClusterSpec
	opManagerContext context.Context
	recorder         record.EventRecorder
}

// Add creates a new CephObjectZone Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context) reconcile.Reconciler {
	return &ReconcileObjectZone{
		client:           mgr.GetClient(),
		scheme:           mgr.GetScheme(),
		context:          context,
		opManagerContext: opManagerContext,
		recorder:         mgr.GetEventRecorderFor("rook-" + controllerName),
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the CephObjectZone CRD object
	err = c.Watch(&source.Kind{Type: &cephv1.CephObjectZone{TypeMeta: controllerTypeMeta}}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate())
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephObjectZone object and makes changes based on the state read
// and what is in the CephObjectZone.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileObjectZone) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, cephObjectZone, err := r.reconcile(request)

	return reporting.ReportReconcileResult(logger, r.recorder, request, &cephObjectZone, reconcileResponse, err)
}

func (r *ReconcileObjectZone) reconcile(request reconcile.Request) (reconcile.Result, cephv1.CephObjectZone, error) {
	// Fetch the CephObjectZone instance
	cephObjectZone := &cephv1.CephObjectZone{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephObjectZone)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephObjectZone resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, *cephObjectZone, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, *cephObjectZone, errors.Wrap(err, "failed to get CephObjectZone")
	}
	// update observedGeneration local variable with current generation value,
	// because generation can be changed before reconile got completed
	// CR status will be updated at end of reconcile, so to reflect the reconcile has finished
	observedGeneration := cephObjectZone.ObjectMeta.Generation

	// The CR was just created, initializing status fields
	if cephObjectZone.Status == nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.EmptyStatus)
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		//
		if !cephObjectZone.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, *cephObjectZone, nil
		}
		return reconcileResponse, *cephObjectZone, nil
	}
	r.clusterSpec = &cephCluster.Spec

	// DELETE: the CR was deleted
	if !cephObjectZone.GetDeletionTimestamp().IsZero() {
		logger.Debugf("deleting zone CR %q", cephObjectZone.Name)
		r.recorder.Eventf(cephObjectZone, v1.EventTypeNormal, string(cephv1.ReconcileStarted), "deleting CephObjectZOne %q", cephObjectZone.Name)

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, *cephObjectZone, nil
	}

	// Populate clusterInfo during each reconcile
	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace)
	if err != nil {
		return reconcile.Result{}, *cephObjectZone, errors.Wrap(err, "failed to populate cluster info")
	}

	// validate the zone settings
	err = r.validateZoneCR(cephObjectZone)
	if err != nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcileFailedStatus)
		return reconcile.Result{}, *cephObjectZone, errors.Wrapf(err, "invalid CephObjectZone CR %q", cephObjectZone.Name)
	}

	// Start object reconciliation, updating status for this
	r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcilingStatus)

	// Make sure an ObjectZoneGroup is present
	realmName, reconcileResponse, err := r.reconcileObjectZoneGroup(cephObjectZone)
	if err != nil {
		return reconcileResponse, *cephObjectZone, err
	}

	// Make sure zone group has been created in Ceph Cluster
	reconcileResponse, err = r.reconcileCephZoneGroup(cephObjectZone, realmName)
	if err != nil {
		return reconcileResponse, *cephObjectZone, err
	}

	// Create Ceph Zone
	_, err = r.createCephZone(cephObjectZone, realmName)
	if err != nil {
		return r.setFailedStatus(k8sutil.ObservedGenerationNotAvailable, cephObjectZone, request.NamespacedName, "failed to create ceph zone", err)
	}

	// update ObservedGeneration in status at the end of reconcile
	// Set Ready status, we are done reconciling
	r.updateStatus(observedGeneration, request.NamespacedName, k8sutil.ReadyStatus)

	// Return and do not requeue
	logger.Debug("zone done reconciling")
	return reconcile.Result{}, *cephObjectZone, nil
}

func (r *ReconcileObjectZone) createCephZone(zone *cephv1.CephObjectZone, realmName string) (reconcile.Result, error) {
	logger.Infof("creating object zone %q in zonegroup %q in realm %q", zone.Name, zone.Spec.ZoneGroup, realmName)

	realmArg := fmt.Sprintf("--rgw-realm=%s", realmName)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", zone.Spec.ZoneGroup)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", zone.Name)
	objContext := object.NewContext(r.context, r.clusterInfo, zone.Name)

	// get zone group to see if master zone exists yet
	output, err := object.RunAdminCommandNoMultisite(objContext, true, "zonegroup", "get", realmArg, zoneGroupArg)
	if err != nil {
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
			return reconcile.Result{}, errors.Wrapf(err, "ceph zone group %q not found", zone.Spec.ZoneGroup)
		} else {
			return reconcile.Result{}, errors.Wrapf(err, "radosgw-admin zonegroup get failed with code %d", code)
		}
	}

	// check if master zone does not exist yet for period
	zoneGroupJson, err := object.DecodeZoneGroupConfig(output)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to parse `radosgw-admin zonegroup get` output")
	}

	// create zone
	_, err = object.RunAdminCommandNoMultisite(objContext, true, "zone", "get", realmArg, zoneGroupArg, zoneArg)
	if err == nil {
		logger.Debugf("ceph zone %q already exists, new zone and pools will not be created", zone.Name)
		return reconcile.Result{}, nil
	}

	if code, ok := exec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
		logger.Debugf("ceph zone %q not found, running `radosgw-admin zone create`", zone.Name)

		zoneIsMaster := false
		if zoneGroupJson.MasterZoneID == "" {
			zoneIsMaster = true
		}

		err = r.createPoolsAndZone(objContext, zone, realmName, zoneIsMaster)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		return reconcile.Result{}, errors.Wrapf(err, "radosgw-admin zone get failed with code %d for reason %q", code, output)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileObjectZone) createPoolsAndZone(objContext *object.Context, zone *cephv1.CephObjectZone, realmName string, zoneIsMaster bool) error {
	// create pools for zone
	logger.Debugf("creating pools ceph zone %q", zone.Name)
	realmArg := fmt.Sprintf("--rgw-realm=%s", realmName)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", zone.Spec.ZoneGroup)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", zone.Name)

	err := object.CreatePools(objContext, r.clusterSpec, zone.Spec.MetadataPool, zone.Spec.DataPool)
	if err != nil {
		return errors.Wrapf(err, "failed to create pools for zone %v", zone.Name)
	}
	logger.Debugf("created pools ceph zone %q", zone.Name)

	accessKeyArg, secretKeyArg, err := object.GetRealmKeyArgs(r.opManagerContext, r.context, realmName, zone.Namespace)
	if err != nil {
		return errors.Wrap(err, "failed to get keys for realm")
	}
	args := []string{"zone", "create", realmArg, zoneGroupArg, zoneArg, accessKeyArg, secretKeyArg}

	if zoneIsMaster {
		// master zone does not exist yet for zone group
		args = append(args, "--master")
	}

	output, err := object.RunAdminCommandNoMultisite(objContext, false, args...)
	if err != nil {
		return errors.Wrapf(err, "failed to create ceph zone %q for reason %q", zone.Name, output)
	}
	logger.Debugf("created ceph zone %q", zone.Name)

	return nil
}

func (r *ReconcileObjectZone) reconcileObjectZoneGroup(zone *cephv1.CephObjectZone) (string, reconcile.Result, error) {
	// empty zoneGroup gets filled by r.client.Get()
	zoneGroup := &cephv1.CephObjectZoneGroup{}
	err := r.client.Get(r.opManagerContext, types.NamespacedName{Name: zone.Spec.ZoneGroup, Namespace: zone.Namespace}, zoneGroup)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return "", waitForRequeueIfObjectZoneGroupNotReady, err
		}
		return "", waitForRequeueIfObjectZoneGroupNotReady, errors.Wrapf(err, "error getting cephObjectZoneGroup %v", zone.Spec.ZoneGroup)
	}

	logger.Debugf("CephObjectZoneGroup %v found", zoneGroup.Name)
	return zoneGroup.Spec.Realm, reconcile.Result{}, nil
}

func (r *ReconcileObjectZone) reconcileCephZoneGroup(zone *cephv1.CephObjectZone, realmName string) (reconcile.Result, error) {
	realmArg := fmt.Sprintf("--rgw-realm=%s", realmName)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", zone.Spec.ZoneGroup)
	objContext := object.NewContext(r.context, r.clusterInfo, zone.Name)

	_, err := object.RunAdminCommandNoMultisite(objContext, true, "zonegroup", "get", realmArg, zoneGroupArg)
	if err != nil {
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
			return waitForRequeueIfObjectZoneGroupNotReady, errors.Wrapf(err, "ceph zone group %q not found", zone.Spec.ZoneGroup)
		} else {
			return waitForRequeueIfObjectZoneGroupNotReady, errors.Wrapf(err, "radosgw-admin zonegroup get failed with code %d", code)
		}
	}

	logger.Infof("Zone group %q found in Ceph cluster to create ceph zone %q", zone.Spec.ZoneGroup, zone.Name)
	return reconcile.Result{}, nil
}

// validateZoneCR validates the zone arguments
func (r *ReconcileObjectZone) validateZoneCR(z *cephv1.CephObjectZone) error {
	if z.Name == "" {
		return errors.New("missing name")
	}
	if z.Namespace == "" {
		return errors.New("missing namespace")
	}
	if z.Spec.ZoneGroup == "" {
		return errors.New("missing zonegroup")
	}
	if err := pool.ValidatePoolSpec(r.context, r.clusterInfo, r.clusterSpec, &z.Spec.MetadataPool); err != nil {
		return errors.Wrap(err, "invalid metadata pool spec")
	}
	if err := pool.ValidatePoolSpec(r.context, r.clusterInfo, r.clusterSpec, &z.Spec.DataPool); err != nil {
		return errors.Wrap(err, "invalid data pool spec")
	}
	return nil
}

func (r *ReconcileObjectZone) setFailedStatus(observedGeneration int64, cephObjectZone *cephv1.CephObjectZone, name types.NamespacedName, errMessage string, err error) (reconcile.Result, cephv1.CephObjectZone, error) {
	r.updateStatus(observedGeneration, name, k8sutil.ReconcileFailedStatus)
	return reconcile.Result{}, *cephObjectZone, errors.Wrapf(err, "%s", errMessage)
}

// updateStatus updates an zone with a given status
func (r *ReconcileObjectZone) updateStatus(observedGeneration int64, name types.NamespacedName, status string) {
	objectZone := &cephv1.CephObjectZone{}
	if err := r.client.Get(r.opManagerContext, name, objectZone); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephObjectZone resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve object zone %q to update status to %q. %v", name, status, err)
		return
	}
	if objectZone.Status == nil {
		objectZone.Status = &cephv1.Status{}
	}

	objectZone.Status.Phase = status
	if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
		objectZone.Status.ObservedGeneration = observedGeneration
	}
	if err := reporting.UpdateStatus(r.client, objectZone); err != nil {
		logger.Errorf("failed to set object zone %q status to %q. %v", name, status, err)
		return
	}
	logger.Debugf("object zone %q status updated to %q", name, status)
}
