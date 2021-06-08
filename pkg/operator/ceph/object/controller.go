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
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

// ReconcileCephObjectStore reconciles a cephObjectStore object
type ReconcileCephObjectStore struct {
	client              client.Client
	bktclient           bktclient.Interface
	scheme              *runtime.Scheme
	context             *clusterd.Context
	clusterSpec         *cephv1.ClusterSpec
	clusterInfo         *cephclient.ClusterInfo
	objectStoreChannels map[string]*objectStoreHealth
}

type objectStoreHealth struct {
	stopChan          chan struct{}
	monitoringRunning bool
}

// Add creates a new cephObjectStore Controller and adds it to the Manager. The Manager will set fields on the Controller
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
	context.Client = mgr.GetClient()
	return &ReconcileCephObjectStore{
		client:              mgr.GetClient(),
		scheme:              mgrScheme,
		context:             context,
		bktclient:           bktclient.NewForConfigOrDie(context.KubeConfig),
		objectStoreChannels: make(map[string]*objectStoreHealth),
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

	// Build Handler function to return the list of ceph object
	// This is used by the watchers below
	handlerFunc, err := opcontroller.ObjectToCRMapper(mgr.GetClient(), &cephv1.CephObjectStoreList{}, mgr.GetScheme())
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

// Reconcile reads that state of the cluster for a cephObjectStore object and makes changes based on the state read
// and what is in the cephObjectStore.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephObjectStore) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileCephObjectStore) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the cephObjectStore instance
	cephObjectStore := &cephv1.CephObjectStore{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cephObjectStore)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("cephObjectStore resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "failed to get cephObjectStore")
	}

	// Set a finalizer so we can do cleanup before the object goes away
	err = opcontroller.AddFinalizerIfNotPresent(r.client, cephObjectStore)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer")
	}

	// The CR was just created, initializing status fields
	if cephObjectStore.Status == nil {
		// The store is not available so let's not build the status Info yet
		updateStatus(r.client, request.NamespacedName, cephv1.ConditionProgressing, map[string]string{})
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
		if !cephObjectStore.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Remove finalizer
			err := opcontroller.RemoveFinalizer(r.client, cephObjectStore)
			if err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, nil
		}

		return reconcileResponse, nil
	}
	r.clusterSpec = &cephCluster.Spec

	// Initialize the channel for this object store
	// This allows us to track multiple ObjectStores in the same namespace
	_, ok := r.objectStoreChannels[cephObjectStore.Name]
	if !ok {
		r.objectStoreChannels[cephObjectStore.Name] = &objectStoreHealth{
			stopChan:          make(chan struct{}),
			monitoringRunning: false,
		}
	}

	// Populate clusterInfo during each reconcile
	r.clusterInfo, _, _, err = mon.LoadClusterInfo(r.context, request.NamespacedName.Namespace)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to populate cluster info")
	}

	// Populate CephVersion
	currentCephVersion, err := cephclient.LeastUptodateDaemonVersion(r.context, r.clusterInfo, opconfig.MonType)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, nil
		}
		return reconcile.Result{}, errors.Wrapf(err, "failed to retrieve current ceph %q version", opconfig.MonType)
	}
	r.clusterInfo.CephVersion = currentCephVersion

	// DELETE: the CR was deleted
	if !cephObjectStore.GetDeletionTimestamp().IsZero() {
		logger.Debugf("deleting store %q", cephObjectStore.Name)

		if ok {
			select {
			case <-r.objectStoreChannels[cephObjectStore.Name].stopChan:
				// channel was closed
				break
			default:
				// Close the channel to stop the healthcheck of the endpoint
				close(r.objectStoreChannels[cephObjectStore.Name].stopChan)
			}

			response, okToDelete := r.verifyObjectBucketCleanup(cephObjectStore)
			if !okToDelete {
				// If the object store cannot be deleted, requeue the request for deletion to see if the conditions
				// will eventually be satisfied such as the object buckets being removed
				return response, nil
			}

			response, okToDelete = r.verifyObjectUserCleanup(cephObjectStore)
			if !okToDelete {
				// If the object store cannot be deleted, requeue the request for deletion to see if the conditions
				// will eventually be satisfied such as the object users being removed
				return response, nil
			}

			cfg := clusterConfig{
				context:     r.context,
				store:       cephObjectStore,
				clusterSpec: r.clusterSpec,
				clusterInfo: r.clusterInfo,
			}
			cfg.deleteStore()

			// Remove object store from the map
			delete(r.objectStoreChannels, cephObjectStore.Name)
		}

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.client, cephObjectStore)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	// validate the store settings
	if err := r.validateStore(cephObjectStore); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "invalid object store %q arguments", cephObjectStore.Name)
	}

	// If the CephCluster has enabled the "pg_autoscaler" module and is running Nautilus
	// we force the pg_autoscale_mode to "on"
	_, propertyExists := cephObjectStore.Spec.DataPool.Parameters[cephclient.PgAutoscaleModeProperty]
	if mgr.IsModuleInSpec(cephCluster.Spec.Mgr.Modules, mgr.PgautoscalerModuleName) &&
		!currentCephVersion.IsAtLeastOctopus() &&
		!propertyExists {
		if len(cephObjectStore.Spec.DataPool.Parameters) == 0 {
			cephObjectStore.Spec.DataPool.Parameters = make(map[string]string)
		}
		cephObjectStore.Spec.DataPool.Parameters[cephclient.PgAutoscaleModeProperty] = cephclient.PgAutoscaleModeOn
	}

	// CREATE/UPDATE
	_, err = r.reconcileCreateObjectStore(cephObjectStore, request.NamespacedName, cephCluster.Spec)
	if err != nil {
		return r.setFailedStatus(request.NamespacedName, "failed to create object store deployments", err)
	}

	// Set Progressing status, we are done reconciling, the health check go routine will update the status
	updateStatus(r.client, request.NamespacedName, cephv1.ConditionProgressing, buildStatusInfo(cephObjectStore))

	// Return and do not requeue
	logger.Debug("done reconciling")
	return reconcile.Result{}, nil
}

func (r *ReconcileCephObjectStore) reconcileCreateObjectStore(cephObjectStore *cephv1.CephObjectStore, namespacedName types.NamespacedName, cluster cephv1.ClusterSpec) (reconcile.Result, error) {
	ownerInfo := k8sutil.NewOwnerInfo(cephObjectStore, r.scheme)
	cfg := clusterConfig{
		context:     r.context,
		clusterInfo: r.clusterInfo,
		store:       cephObjectStore,
		rookVersion: r.clusterSpec.CephVersion.Image,
		clusterSpec: r.clusterSpec,
		DataPathMap: opconfig.NewStatelessDaemonDataPathMap(opconfig.RgwType, cephObjectStore.Name, cephObjectStore.Namespace, r.clusterSpec.DataDirHostPath),
		client:      r.client,
		ownerInfo:   ownerInfo,
	}
	objContext := NewContext(r.context, r.clusterInfo, cephObjectStore.Name)
	objContext.UID = string(cephObjectStore.UID)

	var err error

	if cephObjectStore.Spec.IsExternal() {
		logger.Info("reconciling external object store")

		// RECONCILE SERVICE
		logger.Info("reconciling object store service")
		_, err = cfg.reconcileService(cephObjectStore)
		if err != nil {
			return r.setFailedStatus(namespacedName, "failed to reconcile service", err)
		}

		// RECONCILE ENDPOINTS
		// Always add the endpoint AFTER the service otherwise it will get overridden
		logger.Info("reconciling external object store endpoint")
		err = cfg.reconcileExternalEndpoint(cephObjectStore)
		if err != nil {
			return r.setFailedStatus(namespacedName, "failed to reconcile external endpoint", err)
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
			return r.setFailedStatus(namespacedName, "failed to reconcile service", err)
		}

		// Reconcile Pool Creation
		if !cephObjectStore.Spec.IsMultisite() {
			logger.Info("reconciling object store pools")
			err = CreatePools(objContext, r.clusterSpec, cephObjectStore.Spec.MetadataPool, cephObjectStore.Spec.DataPool)
			if err != nil {
				return r.setFailedStatus(namespacedName, "failed to create object pools", err)
			}
		}

		// Reconcile Multisite Creation
		logger.Infof("setting multisite settings for object store %q", cephObjectStore.Name)
		err = setMultisite(objContext, cephObjectStore, serviceIP)
		if err != nil {
			return r.setFailedStatus(namespacedName, "failed to configure multisite for object store", err)
		}

		// Create or Update Store
		err = cfg.createOrUpdateStore(realmName, zoneGroupName, zoneName)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to create object store %q", cephObjectStore.Name)
		}
	}

	// Start monitoring
	if !cephObjectStore.Spec.HealthCheck.Bucket.Disabled {
		r.startMonitoring(cephObjectStore, objContext, namespacedName)
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
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
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
		err := r.client.Get(context.TODO(), types.NamespacedName{Name: zoneName, Namespace: cephObjectStore.Namespace}, zone)
		if err != nil {
			if kerrors.IsNotFound(err) {
				return "", "", "", waitForRequeueIfObjectStoreNotReady, err
			}
			return "", "", "", waitForRequeueIfObjectStoreNotReady, errors.Wrapf(err, "error getting CephObjectZone %q", cephObjectStore.Spec.Zone.Name)
		}
		logger.Debugf("CephObjectZone resource %s found", zone.Name)

		zonegroup := &cephv1.CephObjectZoneGroup{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: zone.Spec.ZoneGroup, Namespace: cephObjectStore.Namespace}, zonegroup)
		if err != nil {
			if kerrors.IsNotFound(err) {
				return "", "", "", waitForRequeueIfObjectStoreNotReady, err
			}
			return "", "", "", waitForRequeueIfObjectStoreNotReady, errors.Wrapf(err, "error getting CephObjectZoneGroup %q", zone.Spec.ZoneGroup)
		}
		logger.Debugf("CephObjectZoneGroup resource %s found", zonegroup.Name)

		realm := &cephv1.CephObjectRealm{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: zonegroup.Spec.Realm, Namespace: cephObjectStore.Namespace}, realm)
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

func (r *ReconcileCephObjectStore) verifyObjectBucketCleanup(objectstore *cephv1.CephObjectStore) (reconcile.Result, bool) {
	bktProvisioner := GetObjectBucketProvisioner(r.context, objectstore.Namespace)
	bktProvisioner = strings.Replace(bktProvisioner, "/", "-", -1)
	selector := fmt.Sprintf("bucket-provisioner=%s", bktProvisioner)
	objectBuckets, err := r.bktclient.ObjectbucketV1alpha1().ObjectBuckets().List(context.TODO(), metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		logger.Errorf("failed to delete object store. failed to list buckets for objectstore %q in namespace %q", objectstore.Name, objectstore.Namespace)
		return opcontroller.WaitForRequeueIfFinalizerBlocked, false
	}

	if len(objectBuckets.Items) == 0 {
		logger.Infof("no buckets found for objectstore %q in namespace %q", objectstore.Name, objectstore.Namespace)
		return reconcile.Result{}, true
	}

	bucketNames := make([]string, 0)
	for _, bucket := range objectBuckets.Items {
		bucketNames = append(bucketNames, bucket.Name)
	}

	logger.Errorf("failed to delete object store. buckets for objectstore %q in namespace %q are not cleaned up. remaining buckets: %+v", objectstore.Name, objectstore.Namespace, bucketNames)
	return opcontroller.WaitForRequeueIfFinalizerBlocked, false
}

func (r *ReconcileCephObjectStore) startMonitoring(objectstore *cephv1.CephObjectStore, objContext *Context, namespacedName types.NamespacedName) {
	// Start monitoring object store
	if r.objectStoreChannels[objectstore.Name].monitoringRunning {
		logger.Debug("external rgw endpoint monitoring go routine already running!")
		return
	}

	// Set the monitoring flag so we don't start more than one go routine
	r.objectStoreChannels[objectstore.Name].monitoringRunning = true

	var port int32

	if objectstore.Spec.IsTLSEnabled() {
		port = objectstore.Spec.Gateway.SecurePort
	} else if objectstore.Spec.Gateway.Port != 0 {
		port = objectstore.Spec.Gateway.Port
	} else {
		logger.Error("At least one of Port or SecurePort should be non-zero")
		return
	}

	rgwChecker := newBucketChecker(r.context, objContext, port, r.client, namespacedName, &objectstore.Spec)

	// Fetch the admin ops user
	accessKey, secretKey, err := GetAdminOPSUserCredentials(r.context, r.clusterInfo, objContext, objectstore)
	if err != nil {
		logger.Errorf("failed to create or retrieve rgw admin ops user. %v", err)
		return
	}
	rgwChecker.objContext.adminOpsUserAccessKey = accessKey
	rgwChecker.objContext.adminOpsUserSecretKey = secretKey

	logger.Info("starting rgw healthcheck")
	go rgwChecker.checkObjectStore(r.objectStoreChannels[objectstore.Name].stopChan)
}

func (r *ReconcileCephObjectStore) verifyObjectUserCleanup(objectstore *cephv1.CephObjectStore) (reconcile.Result, bool) {
	ctx := context.TODO()
	cephObjectUsers, err := r.context.RookClientset.CephV1().CephObjectStoreUsers(objectstore.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to delete object store. failed to list user for objectstore %q in namespace %q", objectstore.Name, objectstore.Namespace)
		return opcontroller.WaitForRequeueIfFinalizerBlocked, false
	}

	if len(cephObjectUsers.Items) == 0 {
		logger.Infof("no users found for objectstore %q in namespace %q", objectstore.Name, objectstore.Namespace)
		return reconcile.Result{}, true
	}

	userNames := make([]string, 0)
	for _, user := range cephObjectUsers.Items {
		userNames = append(userNames, user.Name)
	}

	logger.Errorf("failed to delete object store. users for objectstore %q in namespace %q are not cleaned up. remaining users: %+v", objectstore.Name, objectstore.Namespace, userNames)
	return opcontroller.WaitForRequeueIfFinalizerBlocked, false
}
