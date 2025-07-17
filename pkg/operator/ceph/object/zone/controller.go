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
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"syscall"
	"time"

	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
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

type domainRootType struct {
	DomainRoot string `json:"domain_root"`
}

var waitForRequeueIfObjectZoneGroupNotReady = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}

// allow this to be overridden for unit tests
var createObjectStorePoolsFunc = object.CreateObjectStorePools

// allow this to be overridden for unit tests
var commitConfigChangesFunc = object.CommitConfigChanges

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
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephObjectZone{TypeMeta: controllerTypeMeta},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephObjectZone]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephObjectZone](mgr.GetScheme()),
		),
	)
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
	defer opcontroller.RecoverAndLogException()
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
	// because generation can be changed before reconcile got completed
	// CR status will be updated at end of reconcile, so to reflect the reconcile has finished
	observedGeneration := cephObjectZone.ObjectMeta.Generation
	// Set a finalizer so we can do cleanup before the object goes away
	generationUpdated, err := opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephObjectZone)
	if err != nil {
		return reconcile.Result{}, *cephObjectZone, errors.Wrap(err, "failed to add finalizer")
	}
	if generationUpdated {
		logger.Infof("reconciling the object zone %q after adding finalizer", cephObjectZone.Name)
		return reconcile.Result{}, *cephObjectZone, nil
	}

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
			// Remove finalizer
			err := opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephObjectZone)
			if err != nil {
				return reconcile.Result{}, *cephObjectZone, errors.Wrap(err, "failed to remove finalizer")
			}
			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, *cephObjectZone, nil
		}
		return reconcileResponse, *cephObjectZone, nil
	}
	r.clusterSpec = &cephCluster.Spec

	// Populate clusterInfo during each reconcile
	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace, r.clusterSpec)
	if err != nil {
		return reconcile.Result{}, *cephObjectZone, errors.Wrap(err, "failed to populate cluster info")
	}

	// validate the zone settings
	err = r.validateZoneCR(cephObjectZone)
	if err != nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcileFailedStatus)
		return reconcile.Result{}, *cephObjectZone, errors.Wrapf(err, "invalid CephObjectZone CR %q", cephObjectZone.Name)
	}

	// Make sure an ObjectZoneGroup is present
	realmName, reconcileResponse, err := r.getCephObjectZoneGroup(cephObjectZone)
	if err != nil {
		return reconcileResponse, *cephObjectZone, err
	}

	// DELETE: the CR was deleted
	if !cephObjectZone.GetDeletionTimestamp().IsZero() {
		res, err := r.deleteCephObjectZone(cephObjectZone, realmName)
		return res, *cephObjectZone, err
	}

	// Start object reconciliation, updating status for this
	r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcilingStatus)

	// Make sure zone group has been created in Ceph Cluster
	reconcileResponse, err = r.reconcileCephZoneGroup(cephObjectZone, realmName)
	if err != nil {
		return reconcileResponse, *cephObjectZone, err
	}

	// Create/Update Ceph Zone
	_, err = r.createorUpdateCephZone(cephObjectZone, realmName)
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

func (r *ReconcileObjectZone) createorUpdateCephZone(zone *cephv1.CephObjectZone, realmName string) (reconcile.Result, error) {
	logger.Infof("creating object zone %q in zonegroup %q in realm %q", zone.Name, zone.Spec.ZoneGroup, realmName)

	objContext := object.NewContext(r.context, r.clusterInfo, zone.Name)
	objContext.Realm = realmName
	objContext.ZoneGroup = zone.Spec.ZoneGroup
	objContext.Zone = zone.Name

	err := r.createPoolsAndZone(objContext, zone)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileObjectZone) createPoolsAndZone(objContext *object.Context, zone *cephv1.CephObjectZone) error {
	// create pools for zone
	logger.Debugf("creating pools ceph zone %q", zone.Name)
	err := object.ValidateObjectStorePoolsConfig(zone.Spec.MetadataPool, zone.Spec.DataPool, zone.Spec.SharedPools)
	if err != nil {
		return fmt.Errorf("invalid zone pools config: %w", err)
	}
	if object.IsNeedToCreateObjectStorePools(zone.Spec.SharedPools) {
		err = createObjectStorePoolsFunc(objContext, r.clusterSpec, zone.Spec.MetadataPool, zone.Spec.DataPool)
		if err != nil {
			return fmt.Errorf("unable to create pools for zone: %w", err)
		}
		logger.Debugf("created pools ceph zone %q", zone.Name)
	}

	err = r.createZoneIfNotExists(objContext, zone)
	if err != nil {
		return err
	}

	// Configure the zone for RADOS namespaces
	err = object.ConfigureSharedPoolsForZone(objContext, zone.Spec.SharedPools)
	if err != nil {
		return errors.Wrapf(err, "failed to configure rados namespaces for zone")
	}

	// Commit rgw zone config changes
	err = commitConfigChangesFunc(objContext)
	if err != nil {
		return errors.Wrapf(err, "failed to commit zone config changes")
	}

	return nil
}

func (r *ReconcileObjectZone) createZoneIfNotExists(objContext *object.Context, zone *cephv1.CephObjectZone) error {
	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", zone.Spec.ZoneGroup)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", zone.Name)
	// get zone group to see if master zone exists yet
	output, err := object.RunAdminCommandNoMultisite(objContext, true, "zonegroup", "get", realmArg, zoneGroupArg)
	if err != nil {
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
			return errors.Wrapf(err, "ceph zone group %q not found", zone.Spec.ZoneGroup)
		} else {
			return errors.Wrapf(err, "radosgw-admin zonegroup get failed with code %d", code)
		}
	}
	// check if master zone does not exist yet for period
	zoneGroupJson, err := object.DecodeZoneGroupConfig(output)
	if err != nil {
		return errors.Wrap(err, "failed to parse `radosgw-admin zonegroup get` output")
	}

	zoneIsMaster := false
	if zoneGroupJson.MasterZoneID == "" {
		zoneIsMaster = true
	}

	// create/update zone
	_, err = object.RunAdminCommandNoMultisite(objContext, true, "zone", "get", realmArg, zoneGroupArg, zoneArg)
	if err == nil {
		logger.Debugf("ceph zone %q already exists, new zone and pools will not be created but checking for update", zone.Name)
		zoneEndpointsModified, err := object.ShouldUpdateZoneEndpointList(zoneGroupJson.Zones, zone.Spec.CustomEndpoints, objContext.Zone)
		if err != nil {
			return err
		}
		if zoneEndpointsModified {
			zoneEndpoints := strings.Join(zone.Spec.CustomEndpoints, ",")
			logger.Debugf("Updating endpoints for zone %q are: %q", objContext.Zone, zoneEndpoints)
			endpointArg := fmt.Sprintf("--endpoints=%s", zoneEndpoints)
			err = object.JoinMultisite(objContext, endpointArg, zoneEndpoints, zone.Namespace)
			if err != nil {
				return err
			}
		}
		logger.Debugf("skip creating zone %q: already exists", zone.Name)
		return nil
	}

	if code, ok := exec.ExitStatus(err); ok && code != int(syscall.ENOENT) {
		return errors.Wrapf(err, "radosgw-admin zone get failed with code %d for reason %q", code, output)
	}
	logger.Debugf("ceph zone %q not found, running `radosgw-admin zone create`", zone.Name)

	accessKeyArg, secretKeyArg, err := object.GetRealmKeyArgs(r.opManagerContext, r.context, objContext.Realm, zone.Namespace)
	if err != nil {
		return errors.Wrap(err, "failed to get keys for realm")
	}
	args := []string{"zone", "create", realmArg, zoneGroupArg, zoneArg, accessKeyArg, secretKeyArg}

	if zoneIsMaster {
		// master zone does not exist yet for zone group
		args = append(args, "--master")
	}
	if len(zone.Spec.CustomEndpoints) > 0 {
		// If custom endpoint list defined set those values
		zoneEndpoints := strings.Join(zone.Spec.CustomEndpoints, ",")
		args = append(args, fmt.Sprintf("--endpoints=%s", zoneEndpoints))
	}
	output, err = object.RunAdminCommandNoMultisite(objContext, false, args...)
	if err != nil {
		return errors.Wrapf(err, "failed to create ceph zone %q for reason %q", zone.Name, output)
	}
	logger.Debugf("created ceph zone %q", zone.Name)
	return nil
}

func (r *ReconcileObjectZone) getCephObjectZoneGroup(zone *cephv1.CephObjectZone) (string, reconcile.Result, error) {
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

func (r *ReconcileObjectZone) deleteZone(objContext *object.Context) error {
	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)
	//	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", objContext.Zone)

	args := []string{"zone", "delete", realmArg, zoneArg}
	output, err := object.RunAdminCommandNoMultisite(objContext, false, args...)
	if err != nil {
		return errors.Wrapf(err, "failed to delete ceph zone %q for reason %q", objContext.Zone, output)
	}
	return nil
}

func (r *ReconcileObjectZone) removeZoneFromZonegroup(objContext *object.Context) error {
	realmArg := fmt.Sprintf("--rgw-realm=%s", objContext.Realm)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", objContext.ZoneGroup)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", objContext.Zone)

	args := []string{"zonegroup", "remove", realmArg, zoneGroupArg, zoneArg}
	output, err := object.RunAdminCommandNoMultisite(objContext, false, args...)
	if err != nil {
		return errors.Wrapf(err, "failed to delete ceph zone %q for reason %q", objContext.Zone, output)
	}

	args = []string{"period", "update", "--commit", realmArg, zoneGroupArg}
	output, err = object.RunAdminCommandNoMultisite(objContext, false, args...)
	if err != nil {
		return errors.Wrapf(err, "failed to commit updates in ceph zonegroup %q for reason %q", objContext.ZoneGroup, output)
	}
	return nil
}

func (r *ReconcileObjectZone) deleteZonePools(objContext *object.Context, zone *cephv1.CephObjectZone, realmName string) error {
	realmArg := fmt.Sprintf("--rgw-realm=%s", realmName)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", zone.Spec.ZoneGroup)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", zone.Name)

	zoneOutput, err := object.RunAdminCommandNoMultisite(objContext, true, "zone", "get", realmArg, zoneGroupArg, zoneArg)
	if err != nil {
		return errors.Wrapf(err, "failed to get zone %q", zone.Name)
	}
	poolPrefix, err := decodePoolPrefixfromZone(zoneOutput)
	if err != nil {
		return errors.Wrapf(err, "failed to parse pool prefix for zone json %v", zoneOutput)
	}

	logger.Debugf("deleting pools for ceph zone %q with prefix %q", zone.Name, poolPrefix)

	if object.EmptyPool(zone.Spec.DataPool) && object.EmptyPool(zone.Spec.MetadataPool) {
		logger.Info("skipping removal of pools since not specified in the object zone")
		return nil
	}
	err = object.DeletePools(objContext, false, poolPrefix)
	if err != nil {
		return errors.Wrap(err, "failed to delete rgw pools")
	}

	return nil
}

func decodePoolPrefixfromZone(data string) (string, error) {
	var domain domainRootType
	err := json.Unmarshal([]byte(data), &domain)
	if err != nil {
		return "", errors.Wrap(err, "failed to unmarshal json")
	}
	s := strings.Split(domain.DomainRoot, ".rgw.")
	return s[0], err
}

func (r *ReconcileObjectZone) deleteCephObjectZone(zone *cephv1.CephObjectZone, realmName string) (reconcile.Result, error) {
	logger.Debugf("deleting zone CR %q", zone.Name)
	objContext := object.NewContext(r.context, r.clusterInfo, zone.Name)
	objContext.Realm = realmName
	objContext.ZoneGroup = zone.Spec.ZoneGroup
	objContext.Zone = zone.Name
	zonePresent, err := object.CheckIfZonePresentInZoneGroup(objContext)
	if err != nil {
		return reconcile.Result{}, err
	}
	zoneIsMaster, err := object.CheckZoneIsMaster(objContext)
	if err != nil {
		return reconcile.Result{}, err
	}
	if zonePresent {
		deps, err := CephObjectZoneDependentStores(r.context, r.clusterInfo, zone, objContext)
		if err != nil {
			return reconcile.Result{}, err
		}
		if !deps.Empty() {
			err := reporting.ReportDeletionBlockedDueToDependents(r.opManagerContext, logger, r.client, zone, deps)
			return opcontroller.WaitForRequeueIfFinalizerBlocked, err
		}
		reporting.ReportDeletionNotBlockedDueToDependents(r.opManagerContext, logger, r.client, r.recorder, zone)
		if !zoneIsMaster {
			err = r.removeZoneFromZonegroup(objContext)
			if err != nil {
				return reconcile.Result{}, err
			}
			zonePresent = false
		}
	}

	// zone successfully removed from zonegroup proceed with delete
	// master zone cannot be removed from zonegroup, it will be always present
	if !zonePresent || zoneIsMaster {
		if !zone.Spec.PreservePoolsOnDelete {
			// This case zone is removed only after the successful pool deletion
			err = r.deleteZonePools(objContext, zone, realmName)
			if err != nil {
				res, _, err := r.setFailedStatus(k8sutil.ObservedGenerationNotAvailable, zone, types.NamespacedName{Namespace: zone.Namespace, Name: zone.Name}, "failed to delete ceph zone", err)
				return res, err
			}
		} else {
			logger.Infof("PreservePoolsOnDelete is set in object zone %s. Pools is not deleted, but Zone is not removed", objContext.Name)
		}
		err = r.deleteZone(objContext)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to delete zone %s", objContext.Name)
		}
		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, zone)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
		}
	}
	// Return and do not requeue. Successful deletion.
	return reconcile.Result{}, nil
}
