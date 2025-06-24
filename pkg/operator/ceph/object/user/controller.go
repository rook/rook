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
	"slices"
	"sort"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
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
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephObjectStoreUser{TypeMeta: controllerTypeMeta},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephObjectStoreUser]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephObjectStoreUser](mgr.GetScheme()),
		),
	)
	if err != nil {
		return err
	}

	// Watch secrets
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.Secret{TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: corev1.SchemeGroupVersion.String()}},
			handler.TypedEnqueueRequestForOwner[*corev1.Secret](
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&cephv1.CephObjectStoreUser{},
			),
			opcontroller.WatchPredicateForNonCRDObject[*corev1.Secret](&cephv1.CephObjectStoreUser{TypeMeta: controllerTypeMeta}, mgr.GetScheme()),
		),
	)
	if err != nil {
		return err
	}

	// watch secrets referenced by CephObjectStoreUser.spec.keys
	const (
		secretNameField = "spec.keys.secretNames"
	)

	err = mgr.GetFieldIndexer().IndexField(context.TODO(), &cephv1.CephObjectStoreUser{TypeMeta: controllerTypeMeta}, secretNameField, func(obj client.Object) []string {
		var secretNames []string
		for _, k := range obj.(*cephv1.CephObjectStoreUser).Spec.Keys {
			if k.AccessKeyRef != nil && k.AccessKeyRef.Name != "" {
				secretNames = append(secretNames, k.AccessKeyRef.Name)
			}
			if k.SecretKeyRef != nil && k.SecretKeyRef.Name != "" {
				secretNames = append(secretNames, k.SecretKeyRef.Name)
			}
		}
		slices.Sort(secretNames)
		return slices.Compact(secretNames)
	})
	if err != nil {
		return errors.Wrapf(err, "failed to setup IndexField for CephObjectStoreUser.Spec.Keys")
	}

	// Always trigger a reconcile when a secret is deleted. This will cause a
	// reconciliation failure to happen immediately in hopes of alerting the end
	// user to the configuration problem.
	changedOrDeleted := predicate.Or(
		predicate.TypedResourceVersionChangedPredicate[*corev1.Secret]{},
		predicate.TypedFuncs[*corev1.Secret]{
			DeleteFunc: func(e event.TypedDeleteEvent[*corev1.Secret]) bool {
				return true
			},
		},
	)

	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.Secret{},
			handler.TypedEnqueueRequestsFromMapFunc(
				func(ctx context.Context, secret *corev1.Secret) []reconcile.Request {
					referencingUsers := &cephv1.CephObjectStoreUserList{}
					err := r.(*ReconcileObjectStoreUser).client.List(ctx, referencingUsers, &client.ListOptions{
						FieldSelector: fields.OneTermEqualSelector(secretNameField, secret.GetName()),
						Namespace:     secret.GetNamespace(),
					})
					if err != nil {
						logger.Errorf("failed to list CephObjectStoreUser(s) while handling event for secret %q in namespace %q. %v", secret.GetName(), secret.GetNamespace(), err)
						return []reconcile.Request{}
					}

					requests := make([]reconcile.Request, len(referencingUsers.Items))
					for i, item := range referencingUsers.Items {
						requests[i] = reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      item.GetName(),
								Namespace: item.GetNamespace(),
							},
						}
					}
					logger.Tracef("CephObjectStoreUsers referencing Secret %q in namespace %q: %v", secret.GetName(), secret.GetNamespace(), requests)
					return requests
				},
			),
			changedOrDeleted,
		),
	)
	if err != nil {
		return errors.Wrapf(err, "failed to configure watch for Secret(s)")
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephObjectStoreUser object and makes changes based on the state read
// and what is in the CephObjectStoreUser.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileObjectStoreUser) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	defer opcontroller.RecoverAndLogException()
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, cephObjectStoreUser, err := r.reconcile(request)
	if err != nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcileFailedStatus)
		logger.Errorf("failed to reconcile %v", err)
	}

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
	generationUpdated, err := opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephObjectStoreUser)
	if err != nil {
		return reconcile.Result{}, *cephObjectStoreUser, errors.Wrap(err, "failed to add finalizer")
	}
	if generationUpdated {
		logger.Infof("reconciling the object user %q after adding finalizer", cephObjectStoreUser.Name)
		return reconcile.Result{}, *cephObjectStoreUser, nil
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
		return opcontroller.WaitForRequeueIfCephClusterNotReady, *cephObjectStoreUser, err
	}

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

	// CR is not deleted, continue reconciling

	// Generate user config
	userConfig := generateUserConfig(cephObjectStoreUser)

	referencedSecrets := &map[types.UID]*corev1.Secret{}
	// Set any provided key pair(s)
	if len(cephObjectStoreUser.Spec.Keys) > 0 {
		var keys *[]admin.UserKeySpec
		keys, referencedSecrets, err = r.generateUserKeySpec(cephObjectStoreUser)
		if err != nil {
			return reconcile.Result{}, *cephObjectStoreUser, errors.Wrapf(err, "failed to generate UserKeySpec for %q", cephObjectStoreUser.Name)
		}

		userConfig.Keys = *keys
	}

	// validate the user settings
	err = r.validateUser(cephObjectStoreUser)
	if err != nil {
		return reconcile.Result{}, *cephObjectStoreUser, errors.Wrapf(err, "invalid pool CR %q spec", cephObjectStoreUser.Name)
	}

	// CREATE/UPDATE CEPH USER
	reconcileResponse, err = r.reconcileCephUser(cephObjectStoreUser, userConfig)
	if err != nil {
		return reconcileResponse, *cephObjectStoreUser, err
	}

	// Update status of referenced secrets only after the rgw user has
	// reconciled. Update even when no secrets are referenced as this could be a
	// transition from explicit keys -> automatic secret generation.
	r.updateKeyStatus(request.NamespacedName, referencedSecrets)

	// CREATE/UPDATE KUBERNETES SECRET
	store, err := r.getObjectStore(cephObjectStoreUser)
	if err != nil {
		return reconcile.Result{}, *cephObjectStoreUser, errors.Wrapf(err, "failed to get object store %q", cephObjectStoreUser.Spec.Store)
	}

	tlsSecretName := store.Spec.Gateway.SSLCertificateRef
	reconcileResponse, err = r.reconcileCephUserSecret(cephObjectStoreUser, userConfig, tlsSecretName)
	if err != nil {
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
	// lookup user by name only and not by access key
	user, err = r.objContext.AdminOpsClient.GetUser(r.opManagerContext, admin.User{ID: u.Name})
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

	quotaEnabled := false
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

	if len(userConfig.Keys) == 0 {
		// use the keys already set on the user & remove all but one key
		if len(user.Keys) == 0 {
			// something is wrong, there should be at least one key
			return errors.Wrapf(err, "no keys set for user %q", u.Name)
		}

		userConfig.Keys = []admin.UserKeySpec{user.Keys[0]}
		logger.Debugf("reducing user %q keypairs to %v", u.Name, userConfig.Keys)
	}

	if err := r.reconcileUserKeys(u.Name, userConfig.Keys); err != nil {
		return errors.Wrapf(err, "failed to reconcile keys for user %q", u.Name)
	}
	logger.Info(logCreateOrUpdate)

	return nil
}

func (r *ReconcileObjectStoreUser) initializeObjectStoreContext(u *cephv1.CephObjectStoreUser) error {
	err := r.objectStoreInitialized(u)
	if err != nil {
		return errors.Wrapf(err, "failed to detect if object store %q is initialized", u.Spec.Store)
	}

	store, err := r.getObjectStore(u)
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
	if err := controllerutil.SetControllerReference(cephObjectStoreUser, secret, r.scheme); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to set owner reference of ceph object user secret %q", secret.Name)
	}

	// Create Kubernetes Secret
	if err := opcontroller.CreateOrUpdateObject(r.opManagerContext, r.client, secret); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to create or update ceph object user %q secret", secret.Name)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileObjectStoreUser) objectStoreInitialized(cephObjectStoreUser *cephv1.CephObjectStoreUser) error {
	cephObjectStore, err := r.getObjectStore(cephObjectStoreUser)
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

func (r *ReconcileObjectStoreUser) getObjectStore(user *cephv1.CephObjectStoreUser) (*cephv1.CephObjectStore, error) {
	storeName := user.Spec.Store
	nsName := client.ObjectKeyFromObject(user)

	// check if CephObjectStore CR is created
	objectStores := &cephv1.CephObjectStoreList{}
	err := r.client.List(r.opManagerContext, objectStores)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil, errors.Wrapf(err, "could not find CephObjectStore %q referenced by CephObjectStoreUser %q", storeName, nsName)
		}
		return nil, errors.Wrapf(err, "failed to list CephObjectStore(s) for CephObjectStoreUser %q", nsName)
	}

	for _, store := range objectStores.Items {
		if store.Name == storeName {
			logger.Debugf("found CephObjectStore %q referenced by CephObjectStoreUser %q", storeName, nsName)
			return &store, nil
		}
	}

	return nil, errors.Errorf("could not find CephObjectStore %q referenced by CephObjectStoreUser %q", storeName, nsName)
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

// updates `.status.keys`. This functionality is not included as part of
// updateStatus() so that the list of referenced secrets, if any, can be
// updated at the same time the rgw user key set is reconciled. This avoids the
// need to regenerate the list of referenced secrets a second time when the
// reconcile has completed and the overall resource status is updated.
func (r *ReconcileObjectStoreUser) updateKeyStatus(name types.NamespacedName, referencedSecrets *map[types.UID]*corev1.Secret) {
	user := &cephv1.CephObjectStoreUser{}
	if err := r.client.Get(r.opManagerContext, name, user); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephObjectStoreUser resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve CephObjectStoreUser %q to update .status.keys. %v", name, err)
		return
	}
	if user.Status == nil {
		user.Status = &cephv1.ObjectStoreUserStatus{}
	}

	logger.Debugf("updating CephObjectStoreUser %q .status.keys to %+v.", name.Name, referencedSecrets)

	keyStatus := []cephv1.SecretReference{}

	for _, secret := range *referencedSecrets {
		keyStatus = append(keyStatus, cephv1.SecretReference{
			SecretReference: corev1.SecretReference{
				Name:      secret.Name,
				Namespace: secret.Namespace,
			},
			UID:             secret.UID,
			ResourceVersion: secret.ResourceVersion,
		})
	}

	// assume map key ordering is unstable between reconciles and sort the slice
	// by secret name
	sort.Slice(keyStatus, func(i, j int) bool {
		return keyStatus[i].Name < keyStatus[j].Name
	})

	user.Status.Keys = keyStatus

	if err := reporting.UpdateStatus(r.client, user); err != nil {
		logger.Warningf("failed to update CephObjectStoreUser %q .status.keys. %v", name, err)
		return
	}
	logger.Debugf("updated CephObjectStoreUser %q .status.keys.", name)
}

// getSecretValue returns the value of key in a kubernetes secret
func (r *ReconcileObjectStoreUser) getSecretValue(selector *corev1.SecretKeySelector, namespace string) (string, *corev1.Secret, error) {
	secret := &corev1.Secret{}
	namespacedName := types.NamespacedName{
		Name:      selector.Name,
		Namespace: namespace,
	}
	if err := r.client.Get(r.opManagerContext, namespacedName, secret); err != nil {
		return "", nil, errors.Wrapf(err, "failed to get secret %q", namespacedName)
	}
	value, ok := secret.Data[selector.Key]
	if !ok {
		return "", nil, errors.Errorf("failed to find key %q in secret %q", selector.Key, namespacedName)
	}

	return string(value), secret, nil
}

// reconcileUserKeys ensures the user's RGW keys match exactly the targetKeys slice.  Any keys set on the user but not present in targetKeys are purged.
func (r *ReconcileObjectStoreUser) reconcileUserKeys(userID string, targetKeys []admin.UserKeySpec) error {
	ctx := r.opManagerContext
	client := r.objContext.AdminOpsClient

	// fetch the current user keys
	userInfo, err := client.GetUser(ctx, admin.User{ID: userID})
	if err != nil {
		return errors.Wrapf(err, "failed to get user %q", userID)
	}

	// create a lookup for the targetkeys (by AccessKey)
	targetMap := make(map[string]admin.UserKeySpec, len(targetKeys))
	for _, k := range targetKeys {
		targetMap[k.AccessKey] = k
	}

	syncdKeys := make([]admin.UserKeySpec, len(targetKeys))

	// remove any keys configured for the user that aren't present in targetKeys
	for _, existingKey := range userInfo.Keys {
		targetKey, found := targetMap[existingKey.AccessKey]
		if !found {
			// RemoveKey() requires the UID to be set but GetUser() returns the list of keys with only .User set
			rmKey := existingKey
			rmKey.UID = userID

			if err := client.RemoveKey(ctx, rmKey); err != nil {
				return errors.Wrapf(err, "failed to remove key %q from user %q", existingKey.AccessKey, userID)
			}
			logger.Debugf("removed key %q from user %q as it is not in the target list", existingKey.AccessKey, userID)
			continue
		}
		if existingKey.SecretKey != targetKey.SecretKey {
			// key exists but needs to be updated; delete it so it will be recreated
			// RemoveKey() requires the UID to be set but GetUser() returns the list of keys with only .User set
			rmKey := existingKey
			rmKey.UID = userID

			if err := client.RemoveKey(ctx, rmKey); err != nil {
				return errors.Wrapf(err, "failed to remove key %q from user %q", existingKey.AccessKey, userID)
			}
			logger.Debugf("removed key %q from user %q needs as it needs to be recreated", existingKey.AccessKey, userID)
			continue
		}
		// else key exists and is correct, no action needed
		// defer removal from targetMap until after the loop to avoid changing the map while iterating
		syncdKeys = append(syncdKeys, targetKey)
	}

	// remove any keys from targetKeys that are already in sync
	for _, k := range syncdKeys {
		delete(targetMap, k.AccessKey)
	}

	// create each desired key
	for _, k := range targetMap {
		if _, err := client.CreateKey(ctx, k); err != nil {
			return errors.Wrapf(err, "failed to create key %q for user %q", k.AccessKey, userID)
		}
		logger.Debugf("created key %q for user %q", k.AccessKey, userID)
	}

	return nil
}

// Construct a []admin.UserKeySpec from all secrets referenced from CephObjectStoreUser.Spec.Keys
func (r *ReconcileObjectStoreUser) generateUserKeySpec(user *cephv1.CephObjectStoreUser) (*[]admin.UserKeySpec, *map[types.UID]*corev1.Secret, error) {
	referencedSecrets := make(map[types.UID]*corev1.Secret)

	keys := make([]admin.UserKeySpec, len(user.Spec.Keys))
	for _, key := range user.Spec.Keys {
		accessKey, secret, err := r.getSecretValue(key.AccessKeyRef, user.Namespace)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to get secret value")
		}
		referencedSecrets[secret.UID] = secret

		secretKey, secret, err := r.getSecretValue(key.SecretKeyRef, user.Namespace)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to get secret value")
		}
		referencedSecrets[secret.UID] = secret

		keys = append(keys, admin.UserKeySpec{
			UID:       user.Name,
			AccessKey: accessKey,
			SecretKey: secretKey,
			KeyType:   "s3",
		})
	}

	return &keys, &referencedSecrets, nil
}
