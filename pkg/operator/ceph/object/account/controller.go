/*
Copyright 2026 The Rook Authors. All rights reserved.

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

// Package account manages RGW accounts.
package account

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	controllerName          = "ceph-object-store-account-controller"
	rgwAccountNameMaxLength = 64
)

// newMultisiteAdminOpsCtxFunc helps us mocking the admin ops API client in unit test
var newMultisiteAdminOpsCtxFunc = object.NewMultisiteAdminOpsContext

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       reflect.TypeFor[cephv1.CephObjectStoreAccount]().Name(),
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

// ReconcileObjectStoreAccount reconciles a CephObjectStoreAccount object
type ReconcileObjectStoreAccount struct {
	client           client.Client
	scheme           *runtime.Scheme
	context          *clusterd.Context
	objContext       *object.AdminOpsContext
	cephClusterSpec  *cephv1.ClusterSpec
	clusterInfo      *cephclient.ClusterInfo
	opManagerContext context.Context
	recorder         events.EventRecorder
}

// Add creates a new CephObjectStoreAccount Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// +kubebuilder:rbac:groups=ceph.rook.io,resources=cephobjectstoreaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ceph.rook.io,resources=cephobjectstoreaccounts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ceph.rook.io,resources=cephobjectstoreaccounts/finalizers,verbs=update
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context) reconcile.Reconciler {
	return &ReconcileObjectStoreAccount{
		client:           mgr.GetClient(),
		scheme:           mgr.GetScheme(),
		context:          context,
		opManagerContext: opManagerContext,
		recorder:         mgr.GetEventRecorder("rook-" + controllerName),
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the CephObjectStoreAccount CRD object
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephObjectStoreAccount{TypeMeta: controllerTypeMeta},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephObjectStoreAccount]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephObjectStoreAccount](mgr.GetScheme()),
		),
	)
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephObjectStoreAccount object and makes changes based on the state read
// and what is in the CephObjectStoreAccount.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileObjectStoreAccount) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	defer opcontroller.RecoverAndLogException()
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, cephObjectStoreAccount, err := r.reconcile(request)
	if err != nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcileFailedStatus)
		log.NamedError(request.NamespacedName, logger, "failed to reconcile %v", err)
	}

	return reporting.ReportReconcileResult(logger, r.recorder, request, &cephObjectStoreAccount, reconcileResponse, err)
}

func (r *ReconcileObjectStoreAccount) reconcile(request reconcile.Request) (reconcile.Result, cephv1.CephObjectStoreAccount, error) {
	// Fetch the CephObjectStoreAccount instance
	cephObjectStoreAccount := &cephv1.CephObjectStoreAccount{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephObjectStoreAccount)
	if err != nil {
		if kerrors.IsNotFound(err) {
			log.NamedDebug(request.NamespacedName, logger, "CephObjectStoreAccount resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, *cephObjectStoreAccount, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, *cephObjectStoreAccount, errors.Wrap(err, "failed to get CephObjectStoreAccount")
	}

	// update observedGeneration local variable with current generation value,
	// because generation can be changed before reconcile got completed
	// CR status will be updated at end of reconcile, so to reflect the reconcile has finished
	observedGeneration := cephObjectStoreAccount.ObjectMeta.Generation

	// Set a finalizer so we can do cleanup before the object goes away
	generationUpdated, err := opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephObjectStoreAccount)
	if err != nil {
		return reconcile.Result{}, *cephObjectStoreAccount, errors.Wrap(err, "failed to add finalizer")
	}
	if generationUpdated {
		log.NamedInfo(request.NamespacedName, logger, "reconciling the object store account after adding finalizer")
		return reconcile.Result{}, *cephObjectStoreAccount, nil
	}

	// The CR was just created, initializing status fields
	if cephObjectStoreAccount.Status == nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.EmptyStatus)
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		// We skip the deleteAccount() function since everything is gone already
		//
		// Also, only remove the finalizer if the CephCluster is gone
		// If not, we should wait for it to be ready
		// This handles the case where the operator is not ready to accept Ceph command but the cluster exists
		if !cephObjectStoreAccount.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Remove finalizer
			err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephObjectStoreAccount)
			if err != nil {
				return reconcile.Result{}, *cephObjectStoreAccount, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, *cephObjectStoreAccount, nil
		}
		return reconcileResponse, *cephObjectStoreAccount, nil
	}
	r.cephClusterSpec = &cephCluster.Spec

	// Populate clusterInfo during each reconcile
	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace, r.cephClusterSpec)
	if err != nil {
		return reconcile.Result{}, *cephObjectStoreAccount, errors.Wrap(err, "failed to populate cluster info")
	}

	// Validate the object store has been initialized
	opsCtx, objectStore, err := object.InitializeObjectStoreContext(r.context, r.clusterInfo, r.client, r.opManagerContext, cephObjectStoreAccount.Spec.Store, newMultisiteAdminOpsCtxFunc)
	if err != nil {
		if !cephObjectStoreAccount.GetDeletionTimestamp().IsZero() {
			// Remove finalizer
			err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephObjectStoreAccount)
			if err != nil {
				return reconcile.Result{}, *cephObjectStoreAccount, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, *cephObjectStoreAccount, nil
		}
		log.NamedDebug(request.NamespacedName, logger, "ObjectStore resource not ready, retrying in %q. %v",
			opcontroller.WaitForRequeueIfCephClusterNotReady.RequeueAfter.String(), err)
		return opcontroller.WaitForRequeueIfCephClusterNotReady, *cephObjectStoreAccount, err
	}
	r.objContext = opsCtx

	// DELETE: the CR was deleted
	if !cephObjectStoreAccount.GetDeletionTimestamp().IsZero() {
		log.NamedDebug(request.NamespacedName, logger, "deleting object store account")
		r.recorder.Eventf(cephObjectStoreAccount, nil, corev1.EventTypeNormal, string(cephv1.ReconcileStarted), string(cephv1.ReconcileStarted), "deleting CephObjectStoreAccount %q", cephObjectStoreAccount.Name)

		err := r.deleteAccount(cephObjectStoreAccount)
		if err != nil {
			return reconcile.Result{}, *cephObjectStoreAccount, errors.Wrapf(err, "failed to delete ceph object store account %q", cephObjectStoreAccount.Name)
		}

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephObjectStoreAccount)
		if err != nil {
			return reconcile.Result{}, *cephObjectStoreAccount, errors.Wrap(err, "failed to remove finalizer")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, *cephObjectStoreAccount, nil
	}

	// CR is not deleted, continue reconciling

	// Reconcile the account
	accountID, err := r.reconcileAccount(cephObjectStoreAccount)
	if err != nil {
		return reconcile.Result{}, *cephObjectStoreAccount, errors.Wrapf(err, "failed to reconcile account %q", cephObjectStoreAccount.Name)
	}

	// Reconcile the root user
	secretName, err := r.reconcileRootUser(cephObjectStoreAccount, accountID, objectStore)
	if err != nil {
		return reconcile.Result{}, *cephObjectStoreAccount, errors.Wrapf(err, "failed to reconcile root user")
	}

	// Update the status with the account ID and root user secret name
	r.updateStatusWithAccountID(observedGeneration, request.NamespacedName, accountID, secretName)

	return reconcile.Result{}, *cephObjectStoreAccount, nil
}

// getAccountName returns the effective account name from the CR spec,
// falling back to the CR name if spec.Name is not set.
func getAccountName(cephObjectStoreAccount *cephv1.CephObjectStoreAccount) string {
	if cephObjectStoreAccount.Spec.Name != "" {
		return cephObjectStoreAccount.Spec.Name
	}
	return cephObjectStoreAccount.Name
}

// getAccountID looks up an already-persisted account ID from the CR,
// checking spec and status in priority order. It never generates
// a new ID. Returns "" when no ID has been recorded yet.
func getAccountID(cephObjectStoreAccount *cephv1.CephObjectStoreAccount) string {
	if cephObjectStoreAccount.Spec.AccountID != "" {
		return cephObjectStoreAccount.Spec.AccountID
	}
	if cephObjectStoreAccount.Status != nil && cephObjectStoreAccount.Status.AccountID != "" {
		return cephObjectStoreAccount.Status.AccountID
	}
	return ""
}

// getOrGenerateAccountID returns the existing account ID if one is persisted,
// otherwise derives a deterministic ID from the CR's UID. This is used during
// reconciliation to ensure the same ID is reused across retries, preventing
// orphaned accounts if the operator crashes after creating an account but
// before persisting the account ID to the CR status.
func getOrGenerateAccountID(cephObjectStoreAccount *cephv1.CephObjectStoreAccount) (string, error) {
	if id := getAccountID(cephObjectStoreAccount); id != "" {
		return id, nil
	}
	if cephObjectStoreAccount.UID != "" {
		nsName := types.NamespacedName{Namespace: cephObjectStoreAccount.Namespace, Name: cephObjectStoreAccount.Name}
		log.NamedInfo(nsName, logger, "generating RGW account ID from the CR's UID")
		id, err := generateDeterministicAccountID(cephObjectStoreAccount.UID)
		if err != nil {
			return "", errors.Wrap(err, "failed to generate deterministic account ID")
		}
		return id, nil
	}
	return "", nil
}

// generateDeterministicAccountID derives a stable RGW account ID from the
// CR's Kubernetes UID. It extracts 14 hex digits from the UUID, converts
// them to a 17-digit decimal number, and prefixes it with "RGW". The same
// UID always produces the same account ID.
func generateDeterministicAccountID(uid types.UID) (string, error) {
	uidStr := string(uid)
	l := len(uidStr)
	if l < 14 {
		return "", fmt.Errorf("UID too short to generate account ID: %q", uidStr)
	}
	last12 := uidStr[l-12:]
	first2 := uidStr[:2]
	value, err := strconv.ParseUint(last12+first2, 16, 64)
	if err != nil {
		return "", fmt.Errorf("failed to parse UID hex digits: %w", err)
	}
	return fmt.Sprintf("RGW%017d", value), nil
}

func (r *ReconcileObjectStoreAccount) reconcileAccount(cephObjectStoreAccount *cephv1.CephObjectStoreAccount) (string, error) {
	accountID, err := getOrGenerateAccountID(cephObjectStoreAccount)
	if err != nil {
		return "", errors.Wrap(err, "failed to get or generate account ID")
	}
	desiredAccount := admin.Account{
		ID:   accountID,
		Name: getAccountName(cephObjectStoreAccount),
	}
	nsName := types.NamespacedName{Namespace: cephObjectStoreAccount.Namespace, Name: cephObjectStoreAccount.Name}

	// Try to fetch the existing account
	liveAccountExists := true
	_, err = object.GetAccount(r.opManagerContext, r.objContext, desiredAccount.ID)
	if err != nil {
		if errors.Is(err, admin.ErrNoSuchKey) {
			log.NamedInfo(nsName, logger, "account %q does not exist", desiredAccount.ID)
			liveAccountExists = false
		} else {
			return "", errors.Wrapf(err, "failed to check if account exists")
		}
	}

	// If Account exists then validate ownership and check if it needs to be updated
	if liveAccountExists {
		// Verify ownership before modifying an existing account. Ownership is confirmed if
		// the status already records the account ID (from a previous reconcile or from
		// the pre-creation bookmark persisted to status before account creation).
		hasOwnershipProof := cephObjectStoreAccount.Status != nil && cephObjectStoreAccount.Status.AccountID == desiredAccount.ID
		if !hasOwnershipProof {
			return "", fmt.Errorf("account ID %q already exists in RGW but is not managed by this CR; refusing to adopt a foreign account", desiredAccount.ID)
		}

		log.NamedInfo(nsName, logger, "ensuring account %q is in sync", desiredAccount.ID)
		updatedAccount, err := object.ModifyAccount(r.opManagerContext, r.objContext, desiredAccount)
		if err != nil {
			return "", errors.Wrapf(err, "failed to modify account %q", desiredAccount.ID)
		}
		log.NamedInfo(nsName, logger, "successfully modified account %q", desiredAccount.ID)
		return updatedAccount.ID, nil
	}

	// Persist the account ID to status before creating the account. This serves as a
	// creation bookmark so that if the operator crashes after creating the account but
	// before the final status update, the next reconcile can identify the account as
	// owned by this CR. The phase is not set to Ready here — that happens after
	// successful creation.
	if err := r.persistAccountIDToStatus(cephObjectStoreAccount, desiredAccount.ID); err != nil {
		return "", errors.Wrapf(err, "failed to persist account ID to status before creating account %q", desiredAccount.Name)
	}

	// Account doesn't exist, create it
	log.NamedInfo(nsName, logger, "creating account %q", desiredAccount.Name)
	createdAccount, err := object.CreateAccount(r.opManagerContext, r.objContext, desiredAccount)
	if err != nil {
		if errors.Is(err, admin.ErrAccountAlreadyExists) {
			return "", fmt.Errorf("failed to create account with ID %q and name %q: an account with this ID or name already exists in RGW; ensure no conflicting account exists", desiredAccount.ID, desiredAccount.Name)
		}
		return "", errors.Wrapf(err, "failed to create account %q", desiredAccount.Name)
	}

	log.NamedInfo(nsName, logger, "successfully created account %q with ID %q", desiredAccount.Name, createdAccount.ID)
	return createdAccount.ID, nil
}

// persistAccountIDToStatus persists the account ID to the CR status before
// creating the account in the backend. This is a no-op if the status already
// contains the desired account ID. The phase is not modified here; it will be
// set to Ready after the account is successfully created.
func (r *ReconcileObjectStoreAccount) persistAccountIDToStatus(cephObjectStoreAccount *cephv1.CephObjectStoreAccount, accountID string) error {
	if cephObjectStoreAccount.Status != nil && cephObjectStoreAccount.Status.AccountID == accountID {
		return nil
	}
	// Re-fetch the latest version to avoid resourceVersion conflicts, since earlier
	// reconcile steps (e.g. updateStatus for initial empty status) may have updated
	// the object on the server, making our in-memory copy stale.
	nsName := types.NamespacedName{Namespace: cephObjectStoreAccount.Namespace, Name: cephObjectStoreAccount.Name}
	latest := &cephv1.CephObjectStoreAccount{}
	if err := r.client.Get(r.opManagerContext, nsName, latest); err != nil {
		return errors.Wrapf(err, "failed to get latest version of object %q", nsName)
	}
	if latest.Status == nil {
		latest.Status = &cephv1.ObjectStoreAccountStatus{}
	}
	latest.Status.AccountID = accountID
	if err := reporting.UpdateStatus(r.client, latest); err != nil {
		return errors.Wrapf(err, "failed to update object %q status", nsName)
	}
	// Update the caller's copy so subsequent checks see the persisted account ID
	cephObjectStoreAccount.Status = latest.Status
	return nil
}

func (r *ReconcileObjectStoreAccount) deleteAccount(cephObjectStoreAccount *cephv1.CephObjectStoreAccount) error {
	nsName := types.NamespacedName{Namespace: cephObjectStoreAccount.Namespace, Name: cephObjectStoreAccount.Name}
	accountID := getAccountID(cephObjectStoreAccount)
	if accountID == "" {
		log.NamedInfo(nsName, logger, "no account ID found, skipping deletion")
		return nil
	}

	// Only delete the account if we have ownership proof (status contains the account ID).
	// If the account ID is only in spec but was never successfully reconciled,
	// it means the controller never created/adopted this account, so we must
	// not delete what could be a foreign account.
	hasOwnershipProof := cephObjectStoreAccount.Status != nil && cephObjectStoreAccount.Status.AccountID == accountID
	if !hasOwnershipProof {
		log.NamedInfo(nsName, logger, "account %q was never successfully managed by this CR, skipping deletion to avoid removing a foreign account", accountID)
		return nil
	}

	// Always attempt to delete the root user to ensure cleanup
	rootUserID := getRootUserID(cephObjectStoreAccount)
	log.NamedInfo(nsName, logger, "deleting root user %q for account %q", rootUserID, accountID)
	err := object.DeleteAccountRootUser(r.opManagerContext, r.objContext, rootUserID)
	if err != nil {
		if !errors.Is(err, admin.ErrNoSuchUser) {
			return errors.Wrapf(err, "failed to delete root user %q for account %q", rootUserID, accountID)
		}
		log.NamedInfo(nsName, logger, "root user %q not found, considering deletion successful", rootUserID)
	} else {
		log.NamedInfo(nsName, logger, "successfully deleted root user %q", rootUserID)
	}

	log.NamedInfo(nsName, logger, "deleting account %q", accountID)

	err = object.DeleteAccount(r.opManagerContext, r.objContext, accountID)
	if err != nil {
		// If account doesn't exist, consider it successful (idempotent)
		if errors.Is(err, admin.ErrNoSuchKey) {
			log.NamedInfo(nsName, logger, "account %q not found, considering deletion successful", accountID)
			return nil
		}
		return errors.Wrapf(err, "failed to delete account %q", accountID)
	}

	log.NamedInfo(nsName, logger, "successfully deleted account %q", accountID)
	return nil
}

// deleteRootUserSecret deletes the Kubernetes secret for root user credentials, treating "not found" as success.
func (r *ReconcileObjectStoreAccount) deleteRootUserSecret(cephObjectStoreAccount *cephv1.CephObjectStoreAccount) error {
	nsName := types.NamespacedName{Namespace: cephObjectStoreAccount.Namespace, Name: cephObjectStoreAccount.Name}
	secretName := generateRootUserSecretName(cephObjectStoreAccount)
	log.NamedInfo(nsName, logger, "deleting root user secret %q", secretName)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cephObjectStoreAccount.Namespace,
		},
	}
	if err := r.client.Delete(r.opManagerContext, secret); err != nil {
		if kerrors.IsNotFound(err) {
			log.NamedInfo(nsName, logger, "root user secret %q not found, considering deletion successful", secretName)
		} else {
			return errors.Wrapf(err, "failed to delete root user secret %q", secretName)
		}
	} else {
		log.NamedInfo(nsName, logger, "successfully deleted root user secret %q", secretName)
	}
	return nil
}

// skipRootUserCreation returns true if root user creation should be skipped.
func skipRootUserCreation(cephObjectStoreAccount *cephv1.CephObjectStoreAccount) bool {
	return cephObjectStoreAccount.Spec.RootUser != nil && cephObjectStoreAccount.Spec.RootUser.SkipCreate != nil && *cephObjectStoreAccount.Spec.RootUser.SkipCreate
}

// getRootUserID returns the root user UID derived from the CR's metadata.uid.
// The root user's UID is the metadata.uid of the CephObjectStoreAccount CR,
// ensuring global uniqueness across multi-cluster and multisite environments.
func getRootUserID(cephObjectStoreAccount *cephv1.CephObjectStoreAccount) string {
	return string(cephObjectStoreAccount.UID)
}

// getRootUserDisplayName returns the display name for the root user.
func getRootUserDisplayName(cephObjectStoreAccount *cephv1.CephObjectStoreAccount) string {
	if cephObjectStoreAccount.Spec.RootUser != nil && cephObjectStoreAccount.Spec.RootUser.DisplayName != "" {
		return cephObjectStoreAccount.Spec.RootUser.DisplayName
	}
	// RGW enforces a maximum display name length of 64 characters
	return truncate(fmt.Sprintf("root-%s", cephObjectStoreAccount.Name), rgwAccountNameMaxLength)
}

// accountResourceLabels returns the standard labels for resources related to a CephObjectStoreAccount.
func accountResourceLabels(cephObjectStoreAccount *cephv1.CephObjectStoreAccount) map[string]string {
	return map[string]string{
		"cephObjectStoreAccountName": truncate(cephObjectStoreAccount.Name, validation.DNS1123LabelMaxLength),
		"cephObjectStoreName":        truncate(cephObjectStoreAccount.Spec.Store, validation.DNS1123LabelMaxLength),
		"cephObjectStoreNamespace":   truncate(cephObjectStoreAccount.Namespace, validation.DNS1123LabelMaxLength),
	}
}

// truncate truncates a string to the specified maximum length.
func truncate(value string, maxLen int) string {
	if len(value) > maxLen {
		return value[:maxLen]
	}
	return value
}

// generateRootUserSecretName returns the name of the Kubernetes secret for root user credentials.
func generateRootUserSecretName(cephObjectStoreAccount *cephv1.CephObjectStoreAccount) string {
	return fmt.Sprintf("rook-ceph-object-root-user-%s", cephObjectStoreAccount.Name)
}

// reconcileRootUser creates or updates the root user for the account and manages its credentials secret.
// Returns the secret name if a root user was created, or empty string if skipped.
func (r *ReconcileObjectStoreAccount) reconcileRootUser(cephObjectStoreAccount *cephv1.CephObjectStoreAccount, accountID string, objectStore *cephv1.CephObjectStore) (string, error) {
	nsName := types.NamespacedName{Namespace: cephObjectStoreAccount.Namespace, Name: cephObjectStoreAccount.Name}
	if skipRootUserCreation(cephObjectStoreAccount) {
		// If skipCreate is set, delete any previously created root user
		rootUserID := getRootUserID(cephObjectStoreAccount)
		log.NamedInfo(nsName, logger, "skipCreate is true, deleting root user %q if it exists", rootUserID)
		err := object.DeleteAccountRootUser(r.opManagerContext, r.objContext, rootUserID)
		if err != nil && !errors.Is(err, admin.ErrNoSuchUser) {
			return "", errors.Wrapf(err, "failed to delete root user %q for account %q", rootUserID, accountID)
		}
		log.NamedInfo(nsName, logger, "successfully deleted root user %q", rootUserID)

		// Delete the stale credentials secret since the root user no longer exists
		if err := r.deleteRootUserSecret(cephObjectStoreAccount); err != nil {
			return "", err
		}

		return "", nil
	}
	rootUserID := getRootUserID(cephObjectStoreAccount)
	displayName := getRootUserDisplayName(cephObjectStoreAccount)
	accountRoot := true
	generateKey := true

	// Check if the root user already exists
	user, err := object.GetAccountRootUser(r.opManagerContext, r.objContext, rootUserID)
	if err != nil {
		if !errors.Is(err, admin.ErrNoSuchUser) {
			return "", errors.Wrapf(err, "failed to check if root user %q exists", rootUserID)
		}

		// Root user does not exist, create it
		log.NamedInfo(nsName, logger, "creating root user %q for account %q", rootUserID, accountID)
		user, err = object.CreateAccountRootUser(r.opManagerContext, r.objContext, admin.User{
			ID:          rootUserID,
			DisplayName: displayName,
			AccountID:   accountID,
			AccountRoot: &accountRoot,
			GenerateKey: &generateKey,
		})
		if err != nil {
			return "", errors.Wrapf(err, "failed to create root user %q for account %q", rootUserID, accountID)
		}
		log.NamedInfo(nsName, logger, "successfully created root user %q for account %q", rootUserID, accountID)
	} else {
		// Root user exists, always update to ensure desired state
		log.NamedInfo(nsName, logger, "updating root user %q", rootUserID)
		user, err = object.ModifyAccountRootUser(r.opManagerContext, r.objContext, admin.User{
			ID:          rootUserID,
			DisplayName: displayName,
		})
		if err != nil {
			return "", errors.Wrapf(err, "failed to update root user %q", rootUserID)
		}
		log.NamedInfo(nsName, logger, "successfully updated root user %q", rootUserID)
	}

	// Reconcile the credentials secret
	secretName, err := r.reconcileRootUserSecret(cephObjectStoreAccount, &user, objectStore)
	if err != nil {
		return "", errors.Wrapf(err, "failed to reconcile root user secret")
	}

	return secretName, nil
}

// reconcileRootUserSecret creates or updates the Kubernetes secret containing root user credentials.
func (r *ReconcileObjectStoreAccount) reconcileRootUserSecret(cephObjectStoreAccount *cephv1.CephObjectStoreAccount, user *admin.User, objectStore *cephv1.CephObjectStore) (string, error) {
	secretName := generateRootUserSecretName(cephObjectStoreAccount)
	secrets := map[string]string{
		"AccessKey": user.Keys[0].AccessKey,
		"SecretKey": user.Keys[0].SecretKey,
		"Endpoint":  r.objContext.Endpoint,
	}
	if objectStore.Spec.Gateway.SSLCertificateRef != "" {
		secrets["SSLCertSecretName"] = objectStore.Spec.Gateway.SSLCertificateRef
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cephObjectStoreAccount.Namespace,
			Labels:    accountResourceLabels(cephObjectStoreAccount),
		},
		StringData: secrets,
		Type:       k8sutil.RookType,
	}

	// Set owner reference to the CephObjectStoreAccount CR
	if err := controllerutil.SetControllerReference(cephObjectStoreAccount, secret, r.scheme); err != nil {
		return "", errors.Wrapf(err, "failed to set owner reference of root user secret %q", secretName)
	}

	// Create or update the secret
	if err := opcontroller.CreateOrUpdateObject(r.opManagerContext, r.client, secret); err != nil {
		return "", errors.Wrapf(err, "failed to create or update root user secret %q", secretName)
	}

	return secretName, nil
}

func (r *ReconcileObjectStoreAccount) updateStatus(observedGeneration int64, name types.NamespacedName, status string) {
	account := &cephv1.CephObjectStoreAccount{}
	if err := r.client.Get(r.opManagerContext, name, account); err != nil {
		if kerrors.IsNotFound(err) {
			log.NamedDebug(name, logger, "CephObjectStoreAccount resource not found. Ignoring since object must be deleted.")
			return
		}
		log.NamedWarning(name, logger, "failed to retrieve object store account %q to update status. %v", name, err)
		return
	}
	if account.Status == nil {
		account.Status = &cephv1.ObjectStoreAccountStatus{}
	}

	account.Status.Phase = status
	if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
		account.Status.ObservedGeneration = &observedGeneration
	}
	if err := reporting.UpdateStatus(r.client, account); err != nil {
		log.NamedError(name, logger, "failed to set object store account %q status to %q. %v", name, status, err)
		return
	}
	log.NamedDebug(name, logger, "object store account %q status updated to %q", name, status)
}

func (r *ReconcileObjectStoreAccount) updateStatusWithAccountID(observedGeneration int64, name types.NamespacedName, accountID, rootAccountSecretName string) {
	account := &cephv1.CephObjectStoreAccount{}
	if err := r.client.Get(r.opManagerContext, name, account); err != nil {
		if kerrors.IsNotFound(err) {
			log.NamedDebug(name, logger, "CephObjectStoreAccount resource not found. Ignoring since object must be deleted.")
			return
		}
		log.NamedWarning(name, logger, "failed to retrieve object store account %q to update status. %v", name, err)
		return
	}
	if account.Status == nil {
		account.Status = &cephv1.ObjectStoreAccountStatus{}
	}

	account.Status.Phase = k8sutil.ReadyStatus
	account.Status.AccountID = accountID
	account.Status.RootAccountSecretName = rootAccountSecretName
	if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
		account.Status.ObservedGeneration = &observedGeneration
	}
	if err := reporting.UpdateStatus(r.client, account); err != nil {
		log.NamedError(name, logger, "failed to update object store account %q status. %v", name, err)
		return
	}
	log.NamedDebug(name, logger, "object store account %q status updated with account ID %q", name, accountID)
}
