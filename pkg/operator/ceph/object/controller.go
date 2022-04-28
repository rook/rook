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

package object

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/coreos/pkg/capnslog"
	bktclient "github.com/kube-object-storage/lib-bucket-provisioner/pkg/client/clientset/versioned"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName = "ceph-object-controller"
)

var waitForRequeueIfObjectStoreNotReady = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

// List of object resources to watch by the controller
var objectsToWatch = []client.Object{
	&corev1.Secret{TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: corev1.SchemeGroupVersion.String()}},
	&corev1.Service{TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: corev1.SchemeGroupVersion.String()}},
	&appsv1.Deployment{TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: appsv1.SchemeGroupVersion.String()}},
}

var cephObjectStoreKind = reflect.TypeOf(cephv1.CephObjectStore{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephObjectStoreKind,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

var currentAndDesiredCephVersion = opcontroller.CurrentAndDesiredCephVersion

// allow this to be overridden for unit tests
var cephObjectStoreDependents = CephObjectStoreDependents

// ReconcileCephObjectStore reconciles a cephObjectStore object
type ReconcileCephObjectStore struct {
	client              client.Client
	bktclient           bktclient.Interface
	scheme              *runtime.Scheme
	context             *clusterd.Context
	clusterSpec         *cephv1.ClusterSpec
	clusterInfo         *cephclient.ClusterInfo
	objectStoreContexts map[string]*objectStoreHealth
	recorder            record.EventRecorder
	opManagerContext    context.Context
	opConfig            opcontroller.OperatorConfig
}

type objectStoreHealth struct {
	internalCtx    context.Context
	internalCancel context.CancelFunc
	started        bool
}

// Add creates a new cephObjectStore Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext, opConfig))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) reconcile.Reconciler {
	context.Client = mgr.GetClient()
	return &ReconcileCephObjectStore{
		client:              mgr.GetClient(),
		scheme:              mgr.GetScheme(),
		context:             context,
		bktclient:           bktclient.NewForConfigOrDie(context.KubeConfig),
		objectStoreContexts: make(map[string]*objectStoreHealth),
		recorder:            mgr.GetEventRecorderFor("rook-" + controllerName),
		opManagerContext:    opManagerContext,
		opConfig:            opConfig,
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the cephObjectStore CRD object
	err = c.Watch(&source.Kind{Type: &cephv1.CephObjectStore{TypeMeta: controllerTypeMeta}}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate())
	if err != nil {
		return err
	}

	// Watch all other resources
	for _, t := range objectsToWatch {
		err = c.Watch(&source.Kind{Type: t}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &cephv1.CephObjectStore{},
		}, opcontroller.WatchPredicateForNonCRDObject(&cephv1.CephObjectStore{TypeMeta: controllerTypeMeta}, mgr.GetScheme()))
		if err != nil {
			return err
		}
	}

	return nil
}

// Reconcile reads that state of the cluster for a cephObjectStore object and makes changes based on the state read
// and what is in the cephObjectStore.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephObjectStore) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, objectStore, err := r.reconcile(request)

	return reporting.ReportReconcileResult(logger, r.recorder, request,
		&objectStore, reconcileResponse, err)
}

func (r *ReconcileCephObjectStore) reconcile(request reconcile.Request) (reconcile.Result, cephv1.CephObjectStore, error) {
	// Fetch the cephObjectStore instance
	cephObjectStore := &cephv1.CephObjectStore{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephObjectStore)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("cephObjectStore resource not found. Ignoring since object must be deleted.")
			// If there was a previous error or if a user removed this resource's finalizer, it's
			// possible Rook didn't clean up the monitoring routine for this resource. Ensure the
			// routine is stopped when we see the resource is gone.
			cephObjectStore.Name = request.Name
			cephObjectStore.Namespace = request.Namespace
			r.stopMonitoring(cephObjectStore)
			return reconcile.Result{}, *cephObjectStore, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, *cephObjectStore, errors.Wrap(err, "failed to get cephObjectStore")
	}
	// update observedGeneration local variable with current generation value,
	// because generation can be changed before reconile got completed
	// CR status will be updated at end of reconcile, so to reflect the reconcile has finished
	observedGeneration := cephObjectStore.ObjectMeta.Generation

	// Set a finalizer so we can do cleanup before the object goes away
	err = opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephObjectStore)
	if err != nil {
		return reconcile.Result{}, *cephObjectStore, errors.Wrap(err, "failed to add finalizer")
	}

	// The CR was just created, initializing status fields
	if cephObjectStore.Status == nil {
		// The store is not available so let's not build the status Info yet
		updateStatus(r.opManagerContext, k8sutil.ObservedGenerationNotAvailable, r.client, request.NamespacedName, cephv1.ConditionProgressing, map[string]string{})
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		// We skip the deleteStore() function since everything is gone already
		//
		// Also, only remove the finalizer if the CephCluster is gone
		// If not, we should wait for it to be ready
		// This handles the case where the operator is not ready to accept Ceph command but the cluster exists
		if !cephObjectStore.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// don't leak the health checker routine if we are force deleting
			r.stopMonitoring(cephObjectStore)

			// Remove finalizer
			err := opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephObjectStore)
			if err != nil {
				return reconcile.Result{}, *cephObjectStore, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, *cephObjectStore, nil
		}

		return reconcileResponse, *cephObjectStore, nil
	}
	r.clusterSpec = &cephCluster.Spec

	// Initialize the channel for this object store
	// This allows us to track multiple ObjectStores in the same namespace
	_, ok := r.objectStoreContexts[monitoringChannelKey(cephObjectStore)]
	if !ok {
		internalCtx, internalCancel := context.WithCancel(r.opManagerContext)
		r.objectStoreContexts[monitoringChannelKey(cephObjectStore)] = &objectStoreHealth{
			internalCtx:    internalCtx,
			internalCancel: internalCancel,
		}
	}

	// Populate clusterInfo during each reconcile
	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace)
	if err != nil {
		return reconcile.Result{}, *cephObjectStore, errors.Wrap(err, "failed to populate cluster info")
	}

	// DELETE: the CR was deleted
	if !cephObjectStore.GetDeletionTimestamp().IsZero() {
		updateStatus(r.opManagerContext, k8sutil.ObservedGenerationNotAvailable, r.client, request.NamespacedName, cephv1.ConditionDeleting, buildStatusInfo(cephObjectStore))

		// Detect running Ceph version
		runningCephVersion, err := cephclient.LeastUptodateDaemonVersion(r.context, r.clusterInfo, config.MonType)
		if err != nil {
			return reconcile.Result{}, *cephObjectStore, errors.Wrapf(err, "failed to retrieve current ceph %q version", config.MonType)
		}
		r.clusterInfo.CephVersion = runningCephVersion
		r.clusterInfo.Context = r.opManagerContext

		// get the latest version of the object to check dependencies
		err = r.client.Get(r.opManagerContext, request.NamespacedName, cephObjectStore)
		if err != nil {
			return reconcile.Result{}, *cephObjectStore, errors.Wrapf(err, "failed to get latest CephObjectStore %q", request.NamespacedName.String())
		}
		objCtx, err := NewMultisiteContext(r.context, r.clusterInfo, cephObjectStore)
		if err != nil {
			return reconcile.Result{}, *cephObjectStore, errors.Wrapf(err, "failed to check for object buckets. failed to get object context")
		}
		opsCtx, err := NewMultisiteAdminOpsContext(objCtx, &cephObjectStore.Spec)
		if err != nil {
			return reconcile.Result{}, *cephObjectStore, errors.Wrapf(err, "failed to check for object buckets. failed to get admin ops API context")
		}
		deps, err := cephObjectStoreDependents(r.context, r.clusterInfo, cephObjectStore, objCtx, opsCtx)
		if err != nil {
			return reconcile.Result{}, *cephObjectStore, err
		}
		if !deps.Empty() {
			err := reporting.ReportDeletionBlockedDueToDependents(r.opManagerContext, logger, r.client, cephObjectStore, deps)
			return opcontroller.WaitForRequeueIfFinalizerBlocked, *cephObjectStore, err
		}
		reporting.ReportDeletionNotBlockedDueToDependents(r.opManagerContext, logger, r.client, r.recorder, cephObjectStore)

		// Cancel the context to stop monitoring the health of the object store
		r.stopMonitoring(cephObjectStore)

		cfg := clusterConfig{
			context:     r.context,
			store:       cephObjectStore,
			clusterSpec: r.clusterSpec,
			clusterInfo: r.clusterInfo,
		}
		cfg.deleteStore()

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephObjectStore)
		if err != nil {
			return reconcile.Result{}, *cephObjectStore, errors.Wrap(err, "failed to remove finalizer")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, *cephObjectStore, nil
	}

	if cephObjectStore.Spec.IsExternal() {
		// Check the ceph version of the running monitors
		desiredCephVersion, err := cephclient.LeastUptodateDaemonVersion(r.context, r.clusterInfo, config.MonType)
		if err != nil {
			return reconcile.Result{}, *cephObjectStore, errors.Wrapf(err, "failed to retrieve current ceph %q version", config.MonType)
		}
		r.clusterInfo.CephVersion = desiredCephVersion
	} else {
		// Detect desired CephCluster version
		runningCephVersion, desiredCephVersion, err := currentAndDesiredCephVersion(
			r.opManagerContext,
			r.opConfig.Image,
			cephObjectStore.Namespace,
			controllerName,
			k8sutil.NewOwnerInfo(cephObjectStore, r.scheme),
			r.context,
			r.clusterSpec,
			r.clusterInfo,
		)
		if err != nil {
			if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
				logger.Info(opcontroller.OperatorNotInitializedMessage)
				return opcontroller.WaitForRequeueIfOperatorNotInitialized, *cephObjectStore, nil
			}
			return reconcile.Result{}, *cephObjectStore, errors.Wrap(err, "failed to detect running and desired ceph version")
		}

		// If the version of the Ceph monitor differs from the CephCluster CR image version we assume
		// the cluster is being upgraded. So the controller will just wait for the upgrade to finish and
		// then versions should match. Obviously using the cmd reporter job adds up to the deployment time
		if !reflect.DeepEqual(*runningCephVersion, *desiredCephVersion) {
			// Upgrade is in progress, let's wait for the mons to be done
			return opcontroller.WaitForRequeueIfCephClusterIsUpgrading,
				*cephObjectStore,
				opcontroller.ErrorCephUpgradingRequeue(desiredCephVersion, runningCephVersion)
		}
		r.clusterInfo.CephVersion = *desiredCephVersion
	}

	// validate the store settings
	if err := r.validateStore(cephObjectStore); err != nil {
		return reconcile.Result{}, *cephObjectStore, errors.Wrapf(err, "invalid object store %q arguments", cephObjectStore.Name)
	}

	// CREATE/UPDATE
	_, err = r.reconcileCreateObjectStore(cephObjectStore, request.NamespacedName, cephCluster.Spec)
	if err != nil && kerrors.IsNotFound(err) {
		logger.Info(opcontroller.OperatorNotInitializedMessage)
		return opcontroller.WaitForRequeueIfOperatorNotInitialized, *cephObjectStore, nil
	} else if err != nil {
		result, err := r.setFailedStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, "failed to create object store deployments", err)
		return result, *cephObjectStore, err
	}

	// update ObservedGeneration in status at the end of reconcile
	// Set Progressing status, we are done reconciling, the health check go routine will update the status
	updateStatus(r.opManagerContext, observedGeneration, r.client, request.NamespacedName, cephv1.ConditionProgressing, buildStatusInfo(cephObjectStore))

	// Return and do not requeue
	logger.Debug("done reconciling")
	return reconcile.Result{}, *cephObjectStore, nil
}

func (r *ReconcileCephObjectStore) reconcileCreateObjectStore(cephObjectStore *cephv1.CephObjectStore, namespacedName types.NamespacedName, cluster cephv1.ClusterSpec) (reconcile.Result, error) {
	ownerInfo := k8sutil.NewOwnerInfo(cephObjectStore, r.scheme)
	cfg := clusterConfig{
		context:     r.context,
		clusterInfo: r.clusterInfo,
		store:       cephObjectStore,
		rookVersion: r.clusterSpec.CephVersion.Image,
		clusterSpec: r.clusterSpec,
		DataPathMap: config.NewStatelessDaemonDataPathMap(config.RgwType, cephObjectStore.Name, cephObjectStore.Namespace, r.clusterSpec.DataDirHostPath),
		client:      r.client,
		ownerInfo:   ownerInfo,
	}
	objContext, err := NewMultisiteContext(r.context, r.clusterInfo, cephObjectStore)
	if err != nil {
		return r.setFailedStatus(k8sutil.ObservedGenerationNotAvailable, namespacedName, "failed to setup object store context", err)
	}
	objContext.CephClusterSpec = cluster

	if cephObjectStore.Spec.IsExternal() {
		logger.Info("reconciling external object store")

		// RECONCILE SERVICE
		logger.Info("reconciling object store service")
		_, err = cfg.reconcileService(cephObjectStore)
		if err != nil {
			return r.setFailedStatus(k8sutil.ObservedGenerationNotAvailable, namespacedName, "failed to reconcile service", err)
		}

		// RECONCILE ENDPOINTS
		// Always add the endpoint AFTER the service otherwise it will get overridden
		logger.Info("reconciling external object store endpoint")
		err = cfg.reconcileExternalEndpoint(cephObjectStore)
		if err != nil {
			return r.setFailedStatus(k8sutil.ObservedGenerationNotAvailable, namespacedName, "failed to reconcile external endpoint", err)
		}

		if err := UpdateEndpoint(objContext, &cephObjectStore.Spec); err != nil {
			return r.setFailedStatus(k8sutil.ObservedGenerationNotAvailable, namespacedName, "failed to set endpoint", err)
		}
	} else {
		logger.Info("reconciling object store deployments")

		// Reconcile realm/zonegroup/zone CRs & update their names
		realmName, zoneGroupName, zoneName, reconcileResponse, err := r.reconcileMultisiteCRs(cephObjectStore)
		if err != nil {
			return reconcileResponse, err
		}

		// Reconcile Ceph Zone if Multisite
		if cephObjectStore.Spec.IsMultisite() {
			reconcileResponse, err := r.reconcileCephZone(cephObjectStore, zoneGroupName, realmName)
			if err != nil {
				return reconcileResponse, err
			}
		}

		objContext.Realm = realmName
		objContext.ZoneGroup = zoneGroupName
		objContext.Zone = zoneName
		logger.Debugf("realm for object-store is %q, zone group for object-store is %q, zone for object-store is %q", objContext.Realm, objContext.ZoneGroup, objContext.Zone)

		// RECONCILE SERVICE
		logger.Debug("reconciling object store service")
		serviceIP, err := cfg.reconcileService(cephObjectStore)
		if err != nil {
			return r.setFailedStatus(k8sutil.ObservedGenerationNotAvailable, namespacedName, "failed to reconcile service", err)
		}

		if err := UpdateEndpoint(objContext, &cephObjectStore.Spec); err != nil {
			return r.setFailedStatus(k8sutil.ObservedGenerationNotAvailable, namespacedName, "failed to set endpoint", err)
		}

		// Reconcile Pool Creation
		if !cephObjectStore.Spec.IsMultisite() {
			logger.Info("reconciling object store pools")
			err = CreatePools(objContext, r.clusterSpec, cephObjectStore.Spec.MetadataPool, cephObjectStore.Spec.DataPool)
			if err != nil {
				return r.setFailedStatus(k8sutil.ObservedGenerationNotAvailable, namespacedName, "failed to create object pools", err)
			}
		}

		// Reconcile Multisite Creation
		logger.Infof("setting multisite settings for object store %q", cephObjectStore.Name)
		err = setMultisite(objContext, cephObjectStore, serviceIP)
		if err != nil && kerrors.IsNotFound(err) {
			return reconcile.Result{}, err
		} else if err != nil {
			return r.setFailedStatus(k8sutil.ObservedGenerationNotAvailable, namespacedName, "failed to configure multisite for object store", err)
		}

		// Create or Update Store
		err = cfg.createOrUpdateStore(realmName, zoneGroupName, zoneName)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to create object store %q", cephObjectStore.Name)
		}
	}

	// Start monitoring
	if !cephObjectStore.Spec.HealthCheck.Bucket.Disabled {
		err = r.startMonitoring(cephObjectStore, objContext, namespacedName)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileCephObjectStore) reconcileCephZone(store *cephv1.CephObjectStore, zoneGroupName string, realmName string) (reconcile.Result, error) {
	realmArg := fmt.Sprintf("--rgw-realm=%s", realmName)
	zoneGroupArg := fmt.Sprintf("--rgw-zonegroup=%s", zoneGroupName)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", store.Spec.Zone.Name)
	objContext := NewContext(r.context, r.clusterInfo, store.Name)

	_, err := RunAdminCommandNoMultisite(objContext, true, "zone", "get", realmArg, zoneGroupArg, zoneArg)
	if err != nil {
		// ENOENT mean "No such file or directory"
		if code, err := exec.ExtractExitCode(err); err == nil && code == int(syscall.ENOENT) {
			return waitForRequeueIfObjectStoreNotReady, errors.Wrapf(err, "ceph zone %q not found", store.Spec.Zone.Name)
		} else {
			return waitForRequeueIfObjectStoreNotReady, errors.Wrapf(err, "radosgw-admin zone get failed with code %d", code)
		}
	}

	logger.Infof("Zone %q found in Ceph cluster will include object store %q", store.Spec.Zone.Name, store.Name)
	return reconcile.Result{}, nil
}

func (r *ReconcileCephObjectStore) reconcileMultisiteCRs(cephObjectStore *cephv1.CephObjectStore) (string, string, string, reconcile.Result, error) {
	if cephObjectStore.Spec.IsMultisite() {
		zoneName := cephObjectStore.Spec.Zone.Name
		zone := &cephv1.CephObjectZone{}
		err := r.client.Get(r.opManagerContext, types.NamespacedName{Name: zoneName, Namespace: cephObjectStore.Namespace}, zone)
		if err != nil {
			if kerrors.IsNotFound(err) {
				return "", "", "", waitForRequeueIfObjectStoreNotReady, err
			}
			return "", "", "", waitForRequeueIfObjectStoreNotReady, errors.Wrapf(err, "error getting CephObjectZone %q", cephObjectStore.Spec.Zone.Name)
		}
		logger.Debugf("CephObjectZone resource %s found", zone.Name)

		zonegroup := &cephv1.CephObjectZoneGroup{}
		err = r.client.Get(r.opManagerContext, types.NamespacedName{Name: zone.Spec.ZoneGroup, Namespace: cephObjectStore.Namespace}, zonegroup)
		if err != nil {
			if kerrors.IsNotFound(err) {
				return "", "", "", waitForRequeueIfObjectStoreNotReady, err
			}
			return "", "", "", waitForRequeueIfObjectStoreNotReady, errors.Wrapf(err, "error getting CephObjectZoneGroup %q", zone.Spec.ZoneGroup)
		}
		logger.Debugf("CephObjectZoneGroup resource %s found", zonegroup.Name)

		realm := &cephv1.CephObjectRealm{}
		err = r.client.Get(r.opManagerContext, types.NamespacedName{Name: zonegroup.Spec.Realm, Namespace: cephObjectStore.Namespace}, realm)
		if err != nil {
			if kerrors.IsNotFound(err) {
				return "", "", "", waitForRequeueIfObjectStoreNotReady, err
			}
			return "", "", "", waitForRequeueIfObjectStoreNotReady, errors.Wrapf(err, "error getting CephObjectRealm %q", zonegroup.Spec.Realm)
		}
		logger.Debugf("CephObjectRealm resource %s found", realm.Name)

		return realm.Name, zonegroup.Name, zone.Name, reconcile.Result{}, nil
	}

	return cephObjectStore.Name, cephObjectStore.Name, cephObjectStore.Name, reconcile.Result{}, nil
}

func monitoringChannelKey(o *cephv1.CephObjectStore) string {
	return types.NamespacedName{Namespace: o.Namespace, Name: o.Name}.String()
}

func (r *ReconcileCephObjectStore) startMonitoring(objectstore *cephv1.CephObjectStore, objContext *Context, namespacedName types.NamespacedName) error {
	channelKey := monitoringChannelKey(objectstore)

	// Start monitoring object store
	if r.objectStoreContexts[channelKey].started {
		logger.Info("external rgw endpoint monitoring go routine already running!")
		return nil
	}

	rgwChecker, err := newBucketChecker(r.context, objContext, r.client, namespacedName, &objectstore.Spec)
	if err != nil {
		return errors.Wrapf(err, "failed to start rgw health checker for CephObjectStore %q, will re-reconcile", namespacedName.String())
	}

	logger.Infof("starting rgw health checker for CephObjectStore %q", namespacedName.String())
	go rgwChecker.checkObjectStore(r.objectStoreContexts, channelKey)

	// Set the monitoring flag so we don't start more than one go routine
	r.objectStoreContexts[channelKey].started = true

	return nil
}

// cancel monitoring. This is a noop if monitoring is not running.
func (r *ReconcileCephObjectStore) stopMonitoring(objectstore *cephv1.CephObjectStore) {
	channelKey := monitoringChannelKey(objectstore)

	_, monitoringContextExists := r.objectStoreContexts[channelKey]
	if monitoringContextExists {
		// stop the monitoring routine
		r.objectStoreContexts[channelKey].internalCancel()

		// remove the monitoring routine from the map
		delete(r.objectStoreContexts, channelKey)
	}
}
