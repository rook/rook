/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package objectuser to manage a rook object store user.
package objectuser

import (
	"context"
	"fmt"
	"reflect"

	"github.com/ceph/go-ceph/rgw/admin"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	appName        = object.AppName
	controllerName = "ceph-object-store-user-controller"
)

// newMultisiteAdminOpsCtxFunc help us mocking the admin ops API client in unit test
var newMultisiteAdminOpsCtxFunc = object.NewMultisiteAdminOpsContext

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

var cephObjectStoreUserKind = reflect.TypeOf(cephv1.CephObjectStoreUser{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephObjectStoreUserKind,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

// ReconcileObjectStoreUser reconciles a ObjectStoreUser object
type ReconcileObjectStoreUser struct {
	client            client.Client
	scheme            *runtime.Scheme
	context           *clusterd.Context
	objContext        *object.AdminOpsContext
	advertiseEndpoint string
	cephClusterSpec   *cephv1.ClusterSpec
	clusterInfo       *cephclient.ClusterInfo
	opManagerContext  context.Context
	recorder          record.EventRecorder
}

// Add creates a new CephObjectStoreUser Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context) reconcile.Reconciler {
	return &ReconcileObjectStoreUser{
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

	// Watch for changes on the CephObjectStoreUser CRD object
	err = c.Watch(source.Kind[client.Object](mgr.GetCache(), &cephv1.CephObjectStoreUser{TypeMeta: controllerTypeMeta}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate()))
	if err != nil {
		return err
	}

	// Watch secrets
	ownerRequest := handler.EnqueueRequestForOwner(
		mgr.GetScheme(),
		mgr.GetRESTMapper(),
		&cephv1.CephObjectStoreUser{},
	)
	secretSource := source.Kind[client.Object](mgr.GetCache(), &corev1.Secret{TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: corev1.SchemeGroupVersion.String()}}, ownerRequest,
		opcontroller.WatchPredicateForNonCRDObject(&cephv1.CephObjectStoreUser{TypeMeta: controllerTypeMeta}, mgr.GetScheme()),
	)
	err = c.Watch(secretSource)
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephObjectStoreUser object and makes changes based on the state read
// and what is in the CephObjectStoreUser.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileObjectStoreUser) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, cephObjectStoreUser, err := r.reconcile(request)

	return reporting.ReportReconcileResult(logger, r.recorder, request, &cephObjectStoreUser, reconcileResponse, err)

}

func (r *ReconcileObjectStoreUser) reconcile(request reconcile.Request) (reconcile.Result, cephv1.CephObjectStoreUser, error) {
	// Fetch the CephObjectStoreUser instance
	cephObjectStoreUser := &cephv1.CephObjectStoreUser{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephObjectStoreUser)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephObjectStoreUser resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, *cephObjectStoreUser, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, *cephObjectStoreUser, errors.Wrap(err, "failed to get CephObjectStoreUser")
	}
	// update observedGeneration local variable with current generation value,
	// because generation can be changed before reconcile got completed
	// CR status will be updated at end of reconcile, so to reflect the reconcile has finished
	observedGeneration := cephObjectStoreUser.ObjectMeta.Generation

	// Set a finalizer so we can do cleanup before the object goes away
	err = opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephObjectStoreUser)
	if err != nil {
		return reconcile.Result{}, *cephObjectStoreUser, errors.Wrap(err, "failed to add finalizer")
	}

	clusterNamespace := request.NamespacedName
	clusterNamespace.Namespace = clusterStoreNamespace(cephObjectStoreUser)

	// The CR was just created, initializing status fields
	if cephObjectStoreUser.Status == nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.EmptyStatus)
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, clusterNamespace, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		// We skip the deleteUser() function since everything is gone already
		//
		// Also, only remove the finalizer if the CephCluster is gone
		// If not, we should wait for it to be ready
		// This handles the case where the operator is not ready to accept Ceph command but the cluster exists
		if !cephObjectStoreUser.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Remove finalizer
			err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephObjectStoreUser)
			if err != nil {
				return reconcile.Result{}, *cephObjectStoreUser, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, *cephObjectStoreUser, nil
		}
		return reconcileResponse, *cephObjectStoreUser, nil
	}
	r.cephClusterSpec = &cephCluster.Spec

	// Populate clusterInfo during each reconcile
	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, clusterNamespace.Namespace, r.cephClusterSpec)
	if err != nil {
		return reconcile.Result{}, *cephObjectStoreUser, errors.Wrap(err, "failed to populate cluster info")
	}

	// Validate the object store has been initialized
	err = r.initializeObjectStoreContext(cephObjectStoreUser)
	if err != nil {
		if !cephObjectStoreUser.GetDeletionTimestamp().IsZero() {
			// Remove finalizer
			err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephObjectStoreUser)
			if err != nil {
				return reconcile.Result{}, *cephObjectStoreUser, errors.Wrap(err, "failed to remove finalizer")
			}
			r.recorder.Event(cephObjectStoreUser, corev1.EventTypeNormal, string(cephv1.ReconcileSucceeded), "successfully removed finalizer")

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, *cephObjectStoreUser, nil
		}
		logger.Debugf("ObjectStore resource not ready in namespace %q, retrying in %q. %v",
			clusterNamespace.Namespace, opcontroller.WaitForRequeueIfCephClusterNotReady.RequeueAfter.String(), err)
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcileFailedStatus)
		return opcontroller.WaitForRequeueIfCephClusterNotReady, *cephObjectStoreUser, err
	}

	// Generate user config
	userConfig := generateUserConfig(cephObjectStoreUser)

	// DELETE: the CR was deleted
	if !cephObjectStoreUser.GetDeletionTimestamp().IsZero() {
		logger.Debugf("deleting object store user %q", request.NamespacedName)
		r.recorder.Eventf(cephObjectStoreUser, corev1.EventTypeNormal, string(cephv1.ReconcileStarted), "deleting CephObjectStoreUser %q", cephObjectStoreUser.Name)

		err := r.deleteUser(cephObjectStoreUser)
		if err != nil {
			return reconcile.Result{}, *cephObjectStoreUser, errors.Wrapf(err, "failed to delete ceph object user %q", cephObjectStoreUser.Name)
		}

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephObjectStoreUser)
		if err != nil {
			return reconcile.Result{}, *cephObjectStoreUser, errors.Wrap(err, "failed to remove finalizer")
		}
		r.recorder.Event(cephObjectStoreUser, corev1.EventTypeNormal, string(cephv1.ReconcileSucceeded), "successfully removed finalizer")

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, *cephObjectStoreUser, nil
	}

	// validate the user settings
	err = r.validateUser(cephObjectStoreUser)
	if err != nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcileFailedStatus)
		return reconcile.Result{}, *cephObjectStoreUser, errors.Wrapf(err, "invalid pool CR %q spec", cephObjectStoreUser.Name)
	}

	// CREATE/UPDATE CEPH USER
	reconcileResponse, err = r.reconcileCephUser(cephObjectStoreUser, userConfig)
	if err != nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcileFailedStatus)
		return reconcileResponse, *cephObjectStoreUser, err
	}

	// CREATE/UPDATE KUBERNETES SECRET
	store, err := r.getObjectStore(cephObjectStoreUser.Spec.Store)
	if err != nil {
		return reconcile.Result{}, *cephObjectStoreUser, errors.Wrapf(err, "failed to get object store %q", cephObjectStoreUser.Spec.Store)
	}

	tlsSecretName := store.Spec.Gateway.SSLCertificateRef
	reconcileResponse, err = r.reconcileCephUserSecret(cephObjectStoreUser, userConfig, tlsSecretName)
	if err != nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcileFailedStatus)
		return reconcileResponse, *cephObjectStoreUser, err
	}

	// update ObservedGeneration in status at the end of reconcile
	// Set Ready status, we are done reconciling
	r.updateStatus(observedGeneration, request.NamespacedName, k8sutil.ReadyStatus)

	// Return and do not requeue
	logger.Debug("done reconciling")
	return reconcile.Result{}, *cephObjectStoreUser, nil
}

func (r *ReconcileObjectStoreUser) reconcileCephUser(cephObjectStoreUser *cephv1.CephObjectStoreUser, userConfig *admin.User) (reconcile.Result, error) {
	err := r.createOrUpdateCephUser(cephObjectStoreUser, userConfig)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to create/update object store user %q", cephObjectStoreUser.Name)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileObjectStoreUser) createOrUpdateCephUser(u *cephv1.CephObjectStoreUser, userConfig *admin.User) error {
	logger.Infof("creating ceph object user %q in namespace %q", u.Name, u.Namespace)

	logCreateOrUpdate := fmt.Sprintf("retrieved existing ceph object user %q", u.Name)
	var user admin.User
	var err error
	user, err = r.objContext.AdminOpsClient.GetUser(r.opManagerContext, *userConfig)
	if err != nil {
		if errors.Is(err, admin.ErrNoSuchUser) {
			user, err = r.objContext.AdminOpsClient.CreateUser(r.opManagerContext, *userConfig)
			if err != nil {
				return errors.Wrapf(err, "failed to create ceph object user %v", &userConfig.ID)
			}
			logCreateOrUpdate = fmt.Sprintf("created ceph object user %q", u.Name)
		} else {
			return errors.Wrapf(err, "failed to get details from ceph object user %q", u.Name)
		}
	}

	// Update max bucket if necessary
	logger.Tracef("user capabilities(id: %s, caps: %#v, user caps: %s, op mask: %s)",
		user.ID, user.Caps, user.UserCaps, user.OpMask)
	if *user.MaxBuckets != *userConfig.MaxBuckets {
		user, err = r.objContext.AdminOpsClient.ModifyUser(r.opManagerContext, *userConfig)
		if err != nil {
			return errors.Wrapf(err, "failed to update ceph object user %q max buckets", userConfig.ID)
		}
		logCreateOrUpdate = fmt.Sprintf("updated ceph object user %q", u.Name)
	}

	// Update caps if necessary
	user.UserCaps = generateUserCaps(user)
	if user.UserCaps != userConfig.UserCaps {
		// If they are no caps to be removed, the API will return an error "missing user capabilities"
		if user.UserCaps != "" {
			logger.Tracef("remove capabilities %s from user %s", user.UserCaps, userConfig.ID)
			_, err = r.objContext.AdminOpsClient.RemoveUserCap(r.opManagerContext, userConfig.ID, user.UserCaps)
			if err != nil {
				return errors.Wrapf(err, "failed to remove current ceph object user %q capabilities", userConfig.ID)
			}
		}
		if userConfig.UserCaps != "" {
			logger.Tracef("set capabilities %s for user %s", userConfig.UserCaps, userConfig.ID)
			_, err = r.objContext.AdminOpsClient.AddUserCap(r.opManagerContext, userConfig.ID, userConfig.UserCaps)
			if err != nil {
				return errors.Wrapf(err, "failed to update ceph object user %q capabilities", userConfig.ID)
			}
		}
		logCreateOrUpdate = fmt.Sprintf("updated ceph object user %q", u.Name)
	}

	var quotaEnabled = false
	var maxSize int64 = -1
	var maxObjects int64 = -1
	if u.Spec.Quotas != nil {
		if u.Spec.Quotas.MaxObjects != nil {
			maxObjects = *u.Spec.Quotas.MaxObjects
			quotaEnabled = true
		}
		if u.Spec.Quotas.MaxSize != nil {
			maxSize = u.Spec.Quotas.MaxSize.Value()
			quotaEnabled = true
		}
	}
	userQuota := admin.QuotaSpec{
		UID:        u.Name,
		Enabled:    &quotaEnabled,
		MaxSize:    &maxSize,
		MaxObjects: &maxObjects,
	}
	err = r.objContext.AdminOpsClient.SetUserQuota(r.opManagerContext, userQuota)
	if err != nil {
		return errors.Wrapf(err, "failed to set quotas for user %q", u.Name)
	}

	// Set access and secret key
	if len(userConfig.Keys) == 0 {
		userConfig.Keys = make([]admin.UserKeySpec, 1)
	}
	userConfig.Keys[0].AccessKey = user.Keys[0].AccessKey
	userConfig.Keys[0].SecretKey = user.Keys[0].SecretKey
	logger.Info(logCreateOrUpdate)

	return nil
}

func (r *ReconcileObjectStoreUser) initializeObjectStoreContext(u *cephv1.CephObjectStoreUser) error {
	err := r.objectStoreInitialized(u)
	if err != nil {
		return errors.Wrapf(err, "failed to detect if object store %q is initialized", u.Spec.Store)
	}

	store, err := r.getObjectStore(u.Spec.Store)
	if err != nil {
		return errors.Wrapf(err, "failed to get object store %q", u.Spec.Store)
	}

	// Check if the object store allows users to be created in any namespace
	if u.Spec.ClusterNamespace != "" && u.Spec.ClusterNamespace != u.Namespace {
		if !userInNamespaceAllowed(u.Namespace, store.Spec.AllowUsersInNamespaces) {
			return fmt.Errorf(
				"object store %q does not allow creating users namespace %q, the namespace must first be added to allowUsersInNamespaces",
				u.Spec.Store,
				u.Namespace)
		}
	}

	advertiseEndpoint, err := store.GetAdvertiseEndpointUrl()
	if err != nil {
		return errors.Wrapf(err, "failed to get CephObjectStore %q advertise endpoint for object store user", u.Spec.Store)
	}
	r.advertiseEndpoint = advertiseEndpoint

	objContext, err := object.NewMultisiteContext(r.context, r.clusterInfo, store)
	if err != nil {
		return errors.Wrapf(err, "Multisite failed to set on object context for object store user")
	}

	opsContext, err := newMultisiteAdminOpsCtxFunc(objContext, &store.Spec)
	if err != nil {
		return errors.Wrap(err, "failed to initialized rgw admin ops client api")
	}
	r.objContext = opsContext

	return nil
}

func userInNamespaceAllowed(requestedNamespace string, allowedNamespaces []string) bool {
	// Check if there is access to create the user in another namespace
	for _, allowedNamespace := range allowedNamespaces {
		if allowedNamespace == "*" || allowedNamespace == requestedNamespace {
			logger.Debugf("allow creating object user in namespace %q", requestedNamespace)
			return true
		}
	}
	return false
}

func generateUserCaps(user admin.User) string {
	var caps string
	for _, c := range user.Caps {
		caps += fmt.Sprintf("%s=%s;", c.Type, c.Perm)
	}
	return caps
}

func generateUserConfig(user *cephv1.CephObjectStoreUser) *admin.User {
	// Set DisplayName to match Name if DisplayName is not set
	displayName := user.Spec.DisplayName
	if len(displayName) == 0 {
		displayName = user.Name
	}

	// create the user
	userConfig := &admin.User{
		ID:          user.Name,
		DisplayName: displayName,
		Keys:        make([]admin.UserKeySpec, 0),
	}

	defaultMaxBuckets := 1000
	userConfig.MaxBuckets = &defaultMaxBuckets
	if user.Spec.Quotas != nil && user.Spec.Quotas.MaxBuckets != nil {
		userConfig.MaxBuckets = user.Spec.Quotas.MaxBuckets
	}

	if user.Spec.Capabilities != nil {
		if user.Spec.Capabilities.User != "" {
			userConfig.UserCaps += fmt.Sprintf("users=%s;", user.Spec.Capabilities.User)
		}
		if user.Spec.Capabilities.Users != "" {
			userConfig.UserCaps += fmt.Sprintf("users=%s;", user.Spec.Capabilities.Users)
		}
		if user.Spec.Capabilities.Bucket != "" {
			userConfig.UserCaps += fmt.Sprintf("buckets=%s;", user.Spec.Capabilities.Bucket)
		}
		if user.Spec.Capabilities.Buckets != "" {
			userConfig.UserCaps += fmt.Sprintf("buckets=%s;", user.Spec.Capabilities.Buckets)
		}
		if user.Spec.Capabilities.MetaData != "" {
			userConfig.UserCaps += fmt.Sprintf("metadata=%s;", user.Spec.Capabilities.MetaData)
		}
		if user.Spec.Capabilities.Usage != "" {
			userConfig.UserCaps += fmt.Sprintf("usage=%s;", user.Spec.Capabilities.Usage)
		}
		if user.Spec.Capabilities.Zone != "" {
			userConfig.UserCaps += fmt.Sprintf("zone=%s;", user.Spec.Capabilities.Zone)
		}
		if user.Spec.Capabilities.Roles != "" {
			userConfig.UserCaps += fmt.Sprintf("roles=%s;", user.Spec.Capabilities.Roles)
		}
		if user.Spec.Capabilities.AMZCache != "" {
			userConfig.UserCaps += fmt.Sprintf("amz-cache=%s;", user.Spec.Capabilities.AMZCache)
		}
		if user.Spec.Capabilities.BiLog != "" {
			userConfig.UserCaps += fmt.Sprintf("bilog=%s;", user.Spec.Capabilities.BiLog)
		}
		if user.Spec.Capabilities.Info != "" {
			userConfig.UserCaps += fmt.Sprintf("info=%s;", user.Spec.Capabilities.Info)
		}
		if user.Spec.Capabilities.MdLog != "" {
			userConfig.UserCaps += fmt.Sprintf("mdlog=%s;", user.Spec.Capabilities.MdLog)
		}
		if user.Spec.Capabilities.DataLog != "" {
			userConfig.UserCaps += fmt.Sprintf("datalog=%s;", user.Spec.Capabilities.DataLog)
		}
		if user.Spec.Capabilities.UserPolicy != "" {
			userConfig.UserCaps += fmt.Sprintf("user-policy=%s;", user.Spec.Capabilities.UserPolicy)
		}
		if user.Spec.Capabilities.OidcProvider != "" {
			userConfig.UserCaps += fmt.Sprintf("oidc-provider=%s;", user.Spec.Capabilities.OidcProvider)
		}
		if user.Spec.Capabilities.RateLimit != "" {
			userConfig.UserCaps += fmt.Sprintf("ratelimit=%s;", user.Spec.Capabilities.RateLimit)
		}
	}

	return userConfig
}

func generateCephUserSecretName(u *cephv1.CephObjectStoreUser) string {
	return fmt.Sprintf("rook-ceph-object-user-%s-%s", u.Spec.Store, u.Name)
}

func generateStatusInfo(u *cephv1.CephObjectStoreUser) map[string]string {
	m := make(map[string]string)
	m["secretName"] = generateCephUserSecretName(u)
	return m
}

func (r *ReconcileObjectStoreUser) generateCephUserSecret(u *cephv1.CephObjectStoreUser, userConfig *admin.User, tlsSecretName string) *corev1.Secret {
	// Store the keys in a secret
	secrets := map[string]string{
		"AccessKey": userConfig.Keys[0].AccessKey,
		"SecretKey": userConfig.Keys[0].SecretKey,
		"Endpoint":  r.objContext.Endpoint,
	}
	if tlsSecretName != "" {
		secrets["SSLCertSecretName"] = tlsSecretName
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateCephUserSecretName(u),
			Namespace: u.Namespace,
			Labels: map[string]string{
				"app":               appName,
				"user":              u.Name,
				"rook_cluster":      u.Namespace,
				"rook_object_store": u.Spec.Store,
			},
		},
		StringData: secrets,
		Type:       k8sutil.RookType,
	}
	return secret
}

func (r *ReconcileObjectStoreUser) reconcileCephUserSecret(cephObjectStoreUser *cephv1.CephObjectStoreUser, userConfig *admin.User, tlsSecretName string) (reconcile.Result, error) {
	// Generate Kubernetes Secret
	secret := r.generateCephUserSecret(cephObjectStoreUser, userConfig, tlsSecretName)

	// Set owner ref to the object store user object
	err := controllerutil.SetControllerReference(cephObjectStoreUser, secret, r.scheme)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to set owner reference of ceph object user secret %q", secret.Name)
	}

	// Create Kubernetes Secret
	err = opcontroller.CreateOrUpdateObject(r.opManagerContext, r.client, secret)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to create or update ceph object user %q secret", secret.Name)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileObjectStoreUser) objectStoreInitialized(cephObjectStoreUser *cephv1.CephObjectStoreUser) error {
	cephObjectStore, err := r.getObjectStore(cephObjectStoreUser.Spec.Store)
	if err != nil {
		return err
	}
	logger.Debug("CephObjectStore exists")

	// If the rgw is external just return
	// since there are no pods running
	if cephObjectStore.Spec.IsExternal() {
		return nil
	}

	// There are no pods running when the cluster is external
	// Unless you pass the admin key...
	pods, err := r.getRgwPodList(cephObjectStoreUser)
	if err != nil {
		return err
	}

	// check if at least one pod is running
	if len(pods.Items) > 0 {
		logger.Debugf("%d RGW pods found where object store user %q is created", len(pods.Items), cephObjectStoreUser.Name)
		return nil
	}

	return errors.New("no rgw pod found")
}

func (r *ReconcileObjectStoreUser) getObjectStore(storeName string) (*cephv1.CephObjectStore, error) {
	// check if CephObjectStore CR is created
	objectStores := &cephv1.CephObjectStoreList{}
	err := r.client.List(r.opManagerContext, objectStores)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil, errors.Wrapf(err, "CephObjectStore %q could not be found", storeName)
		}
		return nil, errors.Wrap(err, "failed to get CephObjectStore")
	}

	for _, store := range objectStores.Items {
		if store.Name == storeName {
			logger.Infof("CephObjectStore %q found", storeName)
			return &store, nil
		}
	}

	return nil, errors.Errorf("CephObjectStore %q could not be found", storeName)
}

func (r *ReconcileObjectStoreUser) getRgwPodList(cephObjectStoreUser *cephv1.CephObjectStoreUser) (*corev1.PodList, error) {
	pods := &corev1.PodList{}

	// check if ObjectStore is initialized
	// rook does this by starting the RGW pod(s)
	listOpts := []client.ListOption{
		client.InNamespace(clusterStoreNamespace(cephObjectStoreUser)),
		client.MatchingLabels(labelsForRgw(cephObjectStoreUser.Spec.Store)),
	}

	err := r.client.List(r.opManagerContext, pods, listOpts...)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return pods, errors.Wrap(err, "no rgw pod could not be found")
		}
		return pods, errors.Wrap(err, "failed to list rgw pods")
	}

	return pods, nil
}

// Namespace where the object store and cluster are expected to be found
func clusterStoreNamespace(user *cephv1.CephObjectStoreUser) string {
	if user.Spec.ClusterNamespace != "" {
		return user.Spec.ClusterNamespace
	}
	return user.Namespace
}

// Delete the user
func (r *ReconcileObjectStoreUser) deleteUser(u *cephv1.CephObjectStoreUser) error {
	err := r.objContext.AdminOpsClient.RemoveUser(r.opManagerContext, admin.User{ID: u.Name})
	if err != nil {
		if errors.Is(err, admin.ErrNoSuchUser) {
			logger.Warningf("user %q does not exist, nothing to remove", u.Name)
			return nil
		}
		return errors.Wrapf(err, "failed to delete ceph object user %q.", u.Name)
	}

	logger.Infof("ceph object user %q deleted successfully", u.Name)
	return nil
}

// validateUser validates the user arguments
func (r *ReconcileObjectStoreUser) validateUser(u *cephv1.CephObjectStoreUser) error {
	if u.Name == "" {
		return errors.New("missing name")
	}
	if u.Namespace == "" {
		return errors.New("missing namespace")
	}
	if u.Spec.Store == "" {
		return errors.New("missing store")
	}
	return nil
}

func labelsForRgw(name string) map[string]string {
	return map[string]string{"rgw": name, k8sutil.AppAttr: appName}
}

// updateStatus updates an object with a given status
func (r *ReconcileObjectStoreUser) updateStatus(observedGeneration int64, name types.NamespacedName, status string) {
	user := &cephv1.CephObjectStoreUser{}
	if err := r.client.Get(r.opManagerContext, name, user); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephObjectStoreUser resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve object store user %q to update status to %q. %v", name, status, err)
		return
	}
	if user.Status == nil {
		user.Status = &cephv1.ObjectStoreUserStatus{}
	}

	user.Status.Phase = status
	if user.Status.Phase == k8sutil.ReadyStatus {
		user.Status.Info = generateStatusInfo(user)
	}
	if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
		user.Status.ObservedGeneration = observedGeneration
	}
	if err := reporting.UpdateStatus(r.client, user); err != nil {
		logger.Errorf("failed to set object store user %q status to %q. %v", name, status, err)
		return
	}
	logger.Debugf("object store user %q status updated to %q", name, status)
}
