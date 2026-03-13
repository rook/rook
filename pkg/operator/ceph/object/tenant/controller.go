/*
Copyright 2024 The Rook Authors. All rights reserved.

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

package tenant

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/object"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName = "rook-ceph-tenant-identity-controller"

	// Annotations for tenant identity binding
	IdentityBindingAnnotation = "object.fusion.io/identity-binding"
	AccountARNAnnotation      = "object.fusion.io/account-arn"
	RoleARNAnnotation         = "object.fusion.io/role-arn"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "tenant-identity-controller")

// ReconcileTenantIdentity reconciles Kubernetes Namespaces with identity binding annotations
type ReconcileTenantIdentity struct {
	client           client.Client
	scheme           *runtime.Scheme
	context          *clusterd.Context
	clusterInfo      *cephclient.ClusterInfo
	opManagerContext context.Context
}

// Add creates a new tenant identity controller and adds it to the Manager
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext))
}

func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context) reconcile.Reconciler {
	return &ReconcileTenantIdentity{
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
		return errors.Wrapf(err, "failed to create a new %q", controllerName)
	}
	logger.Info("successfully started")

	// Watch for changes to Namespaces - only watch annotation changes
	annotationChangePredicate := predicate.TypedFuncs[*corev1.Namespace]{
		UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Namespace]) bool {
			oldNS := e.ObjectOld
			newNS := e.ObjectNew

			// Check if identity binding annotation changed
			oldBinding := oldNS.Annotations[IdentityBindingAnnotation]
			newBinding := newNS.Annotations[IdentityBindingAnnotation]

			if oldBinding != newBinding {
				logger.Infof("namespace %q identity binding annotation changed from %q to %q", newNS.GetName(), oldBinding, newBinding)
				return true
			}

			return false
		},
	}

	logger.Info("watching for namespace annotation changes")
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.Namespace{},
			&handler.TypedEnqueueRequestForObject[*corev1.Namespace]{},
			annotationChangePredicate,
		),
	)
	if err != nil {
		return errors.Wrap(err, "failed to watch for namespace changes")
	}

	logger.Info("tenant identity controller started watching namespaces")
	return nil
}

// Reconcile reads the state of Kubernetes Namespaces and creates RGW User Accounts for those with identity binding annotations
func (r *ReconcileTenantIdentity) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger.Infof("reconciling tenant identity for namespace %q", request.NamespacedName)

	// Fetch the Namespace instance
	namespace := &corev1.Namespace{}
	err := r.client.Get(ctx, request.NamespacedName, namespace)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("namespace %q not found, ignoring", request.NamespacedName)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, errors.Wrapf(err, "failed to get namespace %q", request.NamespacedName)
	}

	// Check if the namespace is being deleted
	if !namespace.DeletionTimestamp.IsZero() {
		logger.Infof("namespace %q is being deleted, cleaning up RGW account", namespace.Name)
		return r.cleanupRGWAccount(ctx, namespace)
	}

	// Check if identity binding is enabled
	if namespace.Annotations[IdentityBindingAnnotation] != "true" {
		logger.Debugf("namespace %q does not have identity binding enabled, skipping", namespace.Name)
		return reconcile.Result{}, nil
	}

	// Load cluster info - find the CephCluster dynamically
	// List all CephClusters in common namespaces
	cephClusterNamespace := "openshift-storage"
	cephClusterList := &cephv1.CephClusterList{}
	err = r.client.List(ctx, cephClusterList, client.InNamespace(cephClusterNamespace))
	if err != nil {
		// Try rook-ceph namespace as fallback
		cephClusterNamespace = "rook-ceph"
		err = r.client.List(ctx, cephClusterList, client.InNamespace(cephClusterNamespace))
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to list CephClusters in namespace %q", cephClusterNamespace)
		}
	}

	if len(cephClusterList.Items) == 0 {
		return reconcile.Result{}, errors.Errorf("no CephCluster found in namespace %q", cephClusterNamespace)
	}

	// Use the first CephCluster found
	cephCluster := &cephClusterList.Items[0]
	logger.Infof("using CephCluster %q in namespace %q for tenant identity", cephCluster.Name, cephCluster.Namespace)

	// Populate clusterInfo
	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, cephClusterNamespace, &cephCluster.Spec)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to load cluster info")
	}

	// Check if account already exists (annotation is already set)
	if accountARN, exists := namespace.Annotations[AccountARNAnnotation]; exists && accountARN != "" {
		logger.Debugf("namespace %q already has RGW account %q, verifying", namespace.Name, accountARN)
		return r.verifyRGWAccount(ctx, namespace)
	}

	// Create new RGW User Account
	logger.Infof("creating RGW User Account for namespace %q", namespace.Name)
	return r.createRGWAccount(ctx, namespace)
}

// createRGWAccount creates a new RGW User Account for the namespace
func (r *ReconcileTenantIdentity) createRGWAccount(ctx context.Context, namespace *corev1.Namespace) (reconcile.Result, error) {
	// Generate account ID based on namespace name (without RGW prefix)
	accountID := namespace.Name

	logger.Infof("creating RGW User Account %q for namespace %q", accountID, namespace.Name)

	// TODO: Implement actual RGW User Account creation using radosgw-admin
	// This will require:
	// 1. Create the RGW User Account: radosgw-admin account create --account-name=<accountID>
	// 2. Configure OIDC provider for the account
	// 3. Create IAM role for AssumeRoleWithWebIdentity
	// 4. Create service account in the namespace

	// Create the actual RGW User Account first to get the real account ID
	account, err := r.createRGWUserAccount(ctx, namespace, accountID)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to create RGW User Account for namespace %q", namespace.Name)
	}

	// Now update namespace annotations with the actual account ID
	// Re-fetch namespace first to get latest version
	freshNamespace := &corev1.Namespace{}
	err = r.client.Get(ctx, types.NamespacedName{Name: namespace.Name}, freshNamespace)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to fetch namespace %q for annotation update", namespace.Name)
	}

	if freshNamespace.Annotations == nil {
		freshNamespace.Annotations = make(map[string]string)
	}
	freshNamespace.Annotations[AccountARNAnnotation] = account.AccountID
	freshNamespace.Annotations[RoleARNAnnotation] = fmt.Sprintf("arn:aws:iam::%s:role/namespace-role", account.AccountID)

	err = r.client.Update(ctx, freshNamespace)
	if err != nil {
		logger.Warningf("failed to update namespace %q annotations after account creation: %v", namespace.Name, err)
		// Don't fail the reconciliation if annotation update fails - the account was created successfully
	} else {
		logger.Infof("updated namespace %q annotations: account-arn=%s, role-arn=%s", namespace.Name, account.AccountID, freshNamespace.Annotations[RoleARNAnnotation])
	}

	logger.Infof("successfully created RGW User Account %q (ID: %s) for namespace %q", accountID, account.AccountID, namespace.Name)
	// Return success - next reconcile will see the annotations and call verifyRGWAccount instead
	return reconcile.Result{}, nil
}

// createRGWUserAccount creates the actual RGW User Account using radosgw-admin and IAM API
func (r *ReconcileTenantIdentity) createRGWUserAccount(ctx context.Context, namespace *corev1.Namespace, accountID string) (*RGWAccount, error) {
	logger.Infof("creating RGW User Account %q", accountID)

	// Create object context for radosgw-admin commands
	// Find the CephObjectStore in the cluster namespace
	objectStoreList := &cephv1.CephObjectStoreList{}
	err := r.client.List(ctx, objectStoreList, client.InNamespace(r.clusterInfo.Namespace))
	if err != nil {
		logger.Errorf("failed to list CephObjectStores in namespace %q: %v", r.clusterInfo.Namespace, err)
		return nil, errors.Wrapf(err, "failed to list CephObjectStores in namespace %q", r.clusterInfo.Namespace)
	}

	if len(objectStoreList.Items) == 0 {
		logger.Errorf("no CephObjectStore found in namespace %q", r.clusterInfo.Namespace)
		return nil, errors.Errorf("no CephObjectStore found in namespace %q", r.clusterInfo.Namespace)
	}

	objectStoreName := objectStoreList.Items[0].Name
	logger.Infof("using CephObjectStore %q for account %q", objectStoreName, accountID)
	objContext := object.NewContext(r.context, r.clusterInfo, objectStoreName)

	// Step 1: Create the RGW User Account
	logger.Infof("attempting to create account %q using radosgw-admin", accountID)
	account, err := CreateAccount(objContext, accountID)
	if err != nil {
		// Check if account already exists
		if strings.Contains(err.Error(), "already exists") {
			logger.Infof("RGW User Account %q already exists, retrieving info", accountID)
			account, err = GetAccount(objContext, accountID)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get existing RGW User Account %q", accountID)
			}
			logger.Infof("Retrieved existing RGW User Account: %+v", account)
		} else {
			return nil, errors.Wrapf(err, "failed to create RGW User Account %q", accountID)
		}
	} else {
		logger.Infof("RGW User Account created: %+v", account)
	}

	// Step 2: Get admin ops context for IAM API calls
	// We need to find the object store and get admin credentials
	adminOpsContext, err := r.getAdminOpsContext(ctx, objContext)
	if err != nil {
		logger.Warningf("failed to get admin ops context, skipping OIDC setup: %v", err)
		return account, r.createServiceAccount(ctx, namespace, accountID, "")
	}

	// Step 3: Create IAM client
	iamClient, err := CreateIAMClient(objContext, adminOpsContext)
	if err != nil {
		logger.Warningf("failed to create IAM client, skipping OIDC setup: %v", err)
		return account, r.createServiceAccount(ctx, namespace, accountID, "")
	}

	// Step 4: Get OIDC configuration from the cluster
	oidcConfig, err := GetClusterOIDCConfig(ctx, r.client)
	if err != nil {
		logger.Warningf("failed to get OIDC config, skipping OIDC provider creation: %v", err)
		return account, r.createServiceAccount(ctx, namespace, accountID, "")
	}

	// Step 5: Create OIDC provider using IAM API
	logger.Infof("creating OIDC provider for account %q with issuer %q and clientIDs %v", accountID, oidcConfig.IssuerURL, oidcConfig.ClientIDs)
	providerARN, err := CreateOIDCProviderViaAPI(iamClient, oidcConfig.IssuerURL, oidcConfig.Thumbprints, oidcConfig.ClientIDs)
	if err != nil {
		logger.Warningf("failed to create OIDC provider for account %q: %v", accountID, err)
		return account, r.createServiceAccount(ctx, namespace, accountID, "")
	}
	logger.Infof("OIDC provider created: %s", providerARN)

	// Step 6: Create IAM role for AssumeRoleWithWebIdentity
	roleName := "namespace-role"
	assumeRolePolicy := GenerateAssumeRolePolicyDocument(accountID, providerARN, namespace.Name)

	logger.Infof("creating IAM role %q for account %q", roleName, accountID)
	role, err := CreateRole(objContext, accountID, roleName, assumeRolePolicy)
	if err != nil {
		logger.Warningf("failed to create IAM role for account %q: %v", accountID, err)
		return account, r.createServiceAccount(ctx, namespace, accountID, "")
	}
	logger.Infof("IAM role created: %+v", role)

	// Step 7: Create and attach permissions policy
	policyName := "namespace-s3-policy"
	policyDoc := GeneratePermissionsPolicyDocument(accountID)

	logger.Infof("creating IAM policy %q for account %q", policyName, accountID)
	policy, err := CreatePolicy(objContext, accountID, policyName, policyDoc)
	if err != nil {
		logger.Warningf("failed to create IAM policy for account %q: %v", accountID, err)
	} else {
		logger.Infof("IAM policy created: %+v", policy)

		// Attach policy to role
		err = AttachRolePolicy(objContext, accountID, roleName, policy.PolicyARN)
		if err != nil {
			logger.Warningf("failed to attach policy to role: %v", err)
		} else {
			logger.Infof("policy attached to role successfully")
		}
	}

	// Step 8: Create service account in the namespace with role ARN
	return account, r.createServiceAccount(ctx, namespace, accountID, role.RoleARN)
}

// getAdminOpsContext retrieves the admin ops context for IAM API calls
func (r *ReconcileTenantIdentity) getAdminOpsContext(ctx context.Context, objContext *object.Context) (*object.AdminOpsContext, error) {
	// Find the CephObjectStore in the cluster - use the one we already found
	objectStoreName := objContext.Name
	objectStoreNamespace := r.clusterInfo.Namespace

	objectStore := &cephv1.CephObjectStore{}
	err := r.client.Get(ctx, types.NamespacedName{Name: objectStoreName, Namespace: objectStoreNamespace}, objectStore)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get CephObjectStore %s/%s", objectStoreNamespace, objectStoreName)
	}

	// Update object context with object store name
	objContext.Name = objectStoreName

	// Get admin ops context
	adminOpsContext, err := object.NewMultisiteAdminOpsContext(objContext, &objectStore.Spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create admin ops context")
	}

	return adminOpsContext, nil
}

// createServiceAccount creates a service account in the namespace
func (r *ReconcileTenantIdentity) createServiceAccount(ctx context.Context, namespace *corev1.Namespace, accountID, roleARN string) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rgw-identity",
			Namespace: namespace.Name,
			Annotations: map[string]string{
				"object.fusion.io/account-id": accountID,
			},
		},
	}

	if roleARN != "" {
		sa.Annotations["eks.amazonaws.com/role-arn"] = roleARN
	}

	err := r.client.Create(ctx, sa)
	if err != nil && !kerrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "failed to create service account in namespace %q", namespace.Name)
	}

	logger.Infof("successfully created RGW User Account %q and service account for namespace %q", accountID, namespace.Name)
	return nil
}

// verifyRGWAccount verifies that the RGW User Account still exists
func (r *ReconcileTenantIdentity) verifyRGWAccount(ctx context.Context, namespace *corev1.Namespace) (reconcile.Result, error) {
	accountARN := namespace.Annotations[AccountARNAnnotation]

	// Create object context for radosgw-admin commands
	objContext := object.NewContext(r.context, r.clusterInfo, "")

	// Check if the RGW User Account still exists
	_, err := GetAccount(objContext, accountARN)
	if err != nil {
		logger.Warningf("RGW User Account %q not found for namespace %q, recreating", accountARN, namespace.Name)
		// Account doesn't exist, recreate it
		return r.createRGWAccount(ctx, namespace)
	}

	logger.Debugf("verified RGW User Account %q for namespace %q", accountARN, namespace.Name)
	return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
}

// cleanupRGWAccount cleans up the RGW User Account when the namespace is deleted
func (r *ReconcileTenantIdentity) cleanupRGWAccount(ctx context.Context, namespace *corev1.Namespace) (reconcile.Result, error) {
	accountARN, exists := namespace.Annotations[AccountARNAnnotation]
	if !exists || accountARN == "" {
		logger.Debugf("no RGW account to clean up for namespace %q", namespace.Name)
		return reconcile.Result{}, nil
	}

	// Create object context for radosgw-admin commands
	objContext := object.NewContext(r.context, r.clusterInfo, "")

	// Delete the RGW User Account
	logger.Infof("deleting RGW User Account %q for namespace %q", accountARN, namespace.Name)
	err := DeleteAccount(objContext, accountARN)
	if err != nil {
		logger.Warningf("failed to delete RGW User Account %q: %v", accountARN, err)
		// Don't return error - allow namespace deletion to proceed
	} else {
		logger.Infof("successfully deleted RGW User Account %q for namespace %q", accountARN, namespace.Name)
	}

	return reconcile.Result{}, nil
}

// Made with Bob
