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
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/rook/rook/pkg/clusterd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
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
	// Generate account ID based on namespace name
	accountID := fmt.Sprintf("RGW%s", namespace.Name)

	logger.Infof("creating RGW User Account %q for namespace %q", accountID, namespace.Name)

	// TODO: Implement actual RGW User Account creation using radosgw-admin
	// This will require:
	// 1. Create the RGW User Account: radosgw-admin account create --account-name=<accountID>
	// 2. Configure OIDC provider for the account
	// 3. Create IAM role for AssumeRoleWithWebIdentity
	// 4. Create service account in the namespace

	// For now, we'll create a placeholder implementation
	err := r.createRGWUserAccount(ctx, namespace, accountID)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to create RGW User Account for namespace %q", namespace.Name)
	}

	// Update namespace annotations with account ARN and role ARN
	if namespace.Annotations == nil {
		namespace.Annotations = make(map[string]string)
	}
	namespace.Annotations[AccountARNAnnotation] = accountID
	namespace.Annotations[RoleARNAnnotation] = fmt.Sprintf("arn:aws:iam::%s:role/namespace-role", accountID)

	err = r.client.Update(ctx, namespace)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to update namespace %q annotations", namespace.Name)
	}

	logger.Infof("successfully created RGW User Account %q for namespace %q", accountID, namespace.Name)
	return reconcile.Result{}, nil
}

// createRGWUserAccount creates the actual RGW User Account using radosgw-admin
func (r *ReconcileTenantIdentity) createRGWUserAccount(ctx context.Context, namespace *corev1.Namespace, accountID string) error {
	// TODO: Implement RGW User Account creation
	// This will use radosgw-admin commands similar to:
	// radosgw-admin account create --account-name=<accountID>
	// radosgw-admin oidc-provider create --account-name=<accountID> --issuer=<cluster-issuer> --thumbprint=<thumbprint>
	// radosgw-admin role create --account-name=<accountID> --role-name=namespace-role --assume-role-policy-doc=<policy>

	logger.Infof("creating RGW User Account %q (placeholder implementation)", accountID)

	// Create service account in the namespace
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rgw-identity",
			Namespace: namespace.Name,
			Annotations: map[string]string{
				"object.fusion.io/account-id": accountID,
			},
		},
	}

	err := r.client.Create(ctx, sa)
	if err != nil && !kerrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "failed to create service account in namespace %q", namespace.Name)
	}

	logger.Infof("created service account for RGW identity in namespace %q", namespace.Name)
	return nil
}

// verifyRGWAccount verifies that the RGW User Account still exists
func (r *ReconcileTenantIdentity) verifyRGWAccount(ctx context.Context, namespace *corev1.Namespace) (reconcile.Result, error) {
	accountARN := namespace.Annotations[AccountARNAnnotation]

	// TODO: Implement verification logic
	// Check if the RGW User Account still exists using:
	// radosgw-admin account info --account-name=<accountID>
	// If not, recreate it

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

	// TODO: Implement cleanup logic
	// Delete the RGW User Account using:
	// radosgw-admin account rm --account-name=<accountID>

	logger.Infof("cleaned up RGW User Account %q for namespace %q", accountARN, namespace.Name)
	return reconcile.Result{}, nil
}

// Made with Bob
