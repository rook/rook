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
)

const (
	controllerName = "ceph-object-store-account-controller"
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
	opsCtx, _, err := object.InitializeObjectStoreContext(r.context, r.clusterInfo, r.client, r.opManagerContext, cephObjectStoreAccount.Spec.Store, newMultisiteAdminOpsCtxFunc)
	if err != nil {
		if !cephObjectStoreAccount.GetDeletionTimestamp().IsZero() {
			// Remove finalizer
			err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephObjectStoreAccount)
			if err != nil {
				return reconcile.Result{}, *cephObjectStoreAccount, errors.Wrap(err, "failed to remove finalizer")
			}
			r.recorder.Eventf(cephObjectStoreAccount, nil, corev1.EventTypeNormal, string(cephv1.ReconcileSucceeded), string(cephv1.ReconcileSucceeded), "successfully removed finalizer")

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
		r.recorder.Eventf(cephObjectStoreAccount, nil, corev1.EventTypeNormal, string(cephv1.ReconcileSucceeded), string(cephv1.ReconcileSucceeded), "successfully removed finalizer")

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, *cephObjectStoreAccount, nil
	}

	// CR is not deleted, continue reconciling

	// Reconcile the account
	accountID, err := r.reconcileAccount(cephObjectStoreAccount)
	if err != nil {
		return reconcile.Result{}, *cephObjectStoreAccount, errors.Wrapf(err, "failed to reconcile account %q", cephObjectStoreAccount.Name)
	}

	// Update the status with the account ID
	r.updateStatusWithAccountID(observedGeneration, request.NamespacedName, accountID)

	r.recorder.Eventf(cephObjectStoreAccount, nil, corev1.EventTypeNormal, string(cephv1.ReconcileSucceeded), string(cephv1.ReconcileSucceeded), "CephObjectStoreAccount %q reconciled successfully", cephObjectStoreAccount.Name)

	return reconcile.Result{}, *cephObjectStoreAccount, nil
}

func (r *ReconcileObjectStoreAccount) reconcileAccount(cephObjectStoreAccount *cephv1.CephObjectStoreAccount) (string, error) {
	// Determine the account name
	accountName := cephObjectStoreAccount.Spec.Name
	if accountName == "" {
		accountName = cephObjectStoreAccount.Name
	}

	// Determine the account ID (if specified)
	var accountID string
	if cephObjectStoreAccount.Spec.AccountID != "" {
		accountID = cephObjectStoreAccount.Spec.AccountID
	} else if cephObjectStoreAccount.Status != nil && cephObjectStoreAccount.Status.AccountID != "" {
		// if the account is already created, then use the Account ID from the the resource status
		accountID = cephObjectStoreAccount.Status.AccountID
	}

	nsName := types.NamespacedName{Namespace: cephObjectStoreAccount.Namespace, Name: cephObjectStoreAccount.Name}

	// Check if account exists
	var existingAccount admin.Account
	var accountExists bool
	var err error
	if accountID != "" {
		// Try to get account by ID
		existingAccount, err = object.GetAccount(r.opManagerContext, r.objContext, accountID)
		if err != nil {
			if errors.Is(err, admin.ErrNoSuchKey) {
				accountExists = false
			} else {
				return "", errors.Wrapf(err, "failed to check if account exists")
			}
		} else {
			accountExists = true
		}

		if accountExists {
			// Account exists, check if we need to update it
			if existingAccount.Name != accountName {
				log.NamedInfo(nsName, logger, "updating account %q name to %q", accountID, accountName)
				modifiedAccount := admin.Account{
					ID:   accountID,
					Name: accountName,
				}
				updatedAccount, err := object.ModifyAccount(r.opManagerContext, r.objContext, modifiedAccount)
				if err != nil {
					return "", errors.Wrapf(err, "failed to modify account %q", accountID)
				}
				log.NamedInfo(nsName, logger, "successfully updated account %q", accountID)
				return updatedAccount.ID, nil
			}
			log.NamedDebug(nsName, logger, "account %q already exists with correct configuration", accountID)
			return existingAccount.ID, nil
		}
	}

	// Account doesn't exist or no ID specified, create it
	log.NamedInfo(nsName, logger, "creating account %q", accountName)
	newAccount := admin.Account{
		Name: accountName,
	}
	if accountID != "" {
		newAccount.ID = accountID
	}

	createdAccount, err := object.CreateAccount(r.opManagerContext, r.objContext, newAccount)
	if err != nil {
		// Check if it already exists
		if errors.Is(err, admin.ErrAccountAlreadyExists) {
			log.NamedInfo(nsName, logger, "account %q already exists", accountName)
			// Try to get the account to return its ID
			if accountID != "" {
				existingAccount, getErr := object.GetAccount(r.opManagerContext, r.objContext, accountID)
				if getErr == nil {
					return existingAccount.ID, nil
				}
			}
			// If we can't get it, return error
			return "", errors.Wrapf(err, "account already exists but failed to retrieve it")
		}
		return "", errors.Wrapf(err, "failed to create account %q", accountName)
	}

	log.NamedInfo(nsName, logger, "successfully created account %q with ID %q", accountName, createdAccount.ID)
	return createdAccount.ID, nil
}

func (r *ReconcileObjectStoreAccount) deleteAccount(cephObjectStoreAccount *cephv1.CephObjectStoreAccount) error {
	// Get the account ID from status or spec
	accountID := ""
	if cephObjectStoreAccount.Status != nil && cephObjectStoreAccount.Status.AccountID != "" {
		accountID = cephObjectStoreAccount.Status.AccountID
	} else if cephObjectStoreAccount.Spec.AccountID != "" {
		accountID = cephObjectStoreAccount.Spec.AccountID
	}

	if accountID == "" {
		log.NamedInfo(types.NamespacedName{Namespace: cephObjectStoreAccount.Namespace, Name: cephObjectStoreAccount.Name},
			logger, "no account ID found, skipping deletion")
		return nil
	}

	log.NamedInfo(types.NamespacedName{Namespace: cephObjectStoreAccount.Namespace, Name: cephObjectStoreAccount.Name},
		logger, "deleting account %q", accountID)

	err := object.DeleteAccount(r.opManagerContext, r.objContext, accountID)
	if err != nil {
		// If account doesn't exist, consider it successful (idempotent)
		if errors.Is(err, admin.ErrNoSuchKey) {
			log.NamedInfo(types.NamespacedName{Namespace: cephObjectStoreAccount.Namespace, Name: cephObjectStoreAccount.Name},
				logger, "account %q not found, considering deletion successful", accountID)
			return nil
		}
		return errors.Wrapf(err, "failed to delete account %q", accountID)
	}

	log.NamedInfo(types.NamespacedName{Namespace: cephObjectStoreAccount.Namespace, Name: cephObjectStoreAccount.Name},
		logger, "successfully deleted account %q", accountID)
	return nil
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
		account.Status.ObservedGeneration = observedGeneration
	}
	if err := reporting.UpdateStatus(r.client, account); err != nil {
		log.NamedError(name, logger, "failed to set object store account %q status to %q. %v", name, status, err)
		return
	}
	log.NamedDebug(name, logger, "object store account %q status updated to %q", name, status)
}

func (r *ReconcileObjectStoreAccount) updateStatusWithAccountID(observedGeneration int64, name types.NamespacedName, accountID string) {
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
	if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
		account.Status.ObservedGeneration = observedGeneration
	}
	if err := reporting.UpdateStatus(r.client, account); err != nil {
		log.NamedError(name, logger, "failed to update object store account %q status. %v", name, err)
		return
	}
	log.NamedDebug(name, logger, "object store account %q status updated with account ID %q", name, accountID)
}
