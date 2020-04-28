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

	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/k8sutil"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	appName             = object.AppName
	controllerName      = "ceph-object-store-user-controller"
	cephObjectStoreKind = "CephObjectStoreUser"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

var cephObjectStoreUserKind = reflect.TypeOf(cephv1.CephObjectStoreUser{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephObjectStoreUserKind,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

// ReconcileObjectStoreUser reconciles a ObjectStoreUser object
type ReconcileObjectStoreUser struct {
	client     client.Client
	scheme     *runtime.Scheme
	context    *clusterd.Context
	objContext *object.Context
	userConfig object.ObjectUser
}

// Add creates a new CephObjectStoreUser Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context) error {
	return add(mgr, newReconciler(mgr, context))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context) reconcile.Reconciler {
	// Add the cephv1 scheme to the manager scheme so that the controller knows about it
	mgrScheme := mgr.GetScheme()
	cephv1.AddToScheme(mgr.GetScheme())

	return &ReconcileObjectStoreUser{
		client:  mgr.GetClient(),
		scheme:  mgrScheme,
		context: context,
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes on the CephObjectStoreUser CRD object
	err = c.Watch(&source.Kind{Type: &cephv1.CephObjectStoreUser{TypeMeta: controllerTypeMeta}}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate())
	if err != nil {
		return err
	}

	// Watch secrets
	err = c.Watch(&source.Kind{Type: &corev1.Secret{TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: corev1.SchemeGroupVersion.String()}}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &cephv1.CephObjectStoreUser{},
	}, opcontroller.WatchPredicateForNonCRDObject(&cephv1.CephObjectStoreUser{TypeMeta: controllerTypeMeta}, mgr.GetScheme()))
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephObjectStoreUser object and makes changes based on the state read
// and what is in the CephObjectStoreUser.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileObjectStoreUser) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime loggin interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileObjectStoreUser) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the CephObjectStoreUser instance
	cephObjectStoreUser := &cephv1.CephObjectStoreUser{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cephObjectStoreUser)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephObjectStoreUser resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "failed to get CephObjectStoreUser")
	}

	// The CR was just created, initializing status fields
	if cephObjectStoreUser.Status == nil {
		updateStatus(r.client, request.NamespacedName, k8sutil.Created)
	}

	// Make sure a CephCluster is present otherwise do nothing
	_, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.client, r.context, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		// We skip the deleteUser() function since everything is gone already
		//
		// Also, only remove the finalizer if the CephCluster is gone
		// If not, we should wait for it to be ready
		// This handles the case where the operator is not ready to accept Ceph command but the cluster exists
		if !cephObjectStoreUser.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Remove finalizer
			err = opcontroller.RemoveFinalizer(r.client, cephObjectStoreUser)
			if err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, nil
		}
		return reconcileResponse, nil
	}

	// Set a finalizer so we can do cleanup before the object goes away
	err = opcontroller.AddFinalizerIfNotPresent(r.client, cephObjectStoreUser)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer")
	}

	// validate isObjectStoreInitialized
	objContext, err := r.isObjectStoreInitialized(cephObjectStoreUser)
	if err != nil {
		if !cephObjectStoreUser.GetDeletionTimestamp().IsZero() {
			// Remove finalizer
			err = opcontroller.RemoveFinalizer(r.client, cephObjectStoreUser)
			if err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, nil
		}
		logger.Debugf("ObjectStore resource not ready in namespace %q, retrying in %q. %v", request.NamespacedName.Namespace, opcontroller.WaitForRequeueIfCephClusterNotReadyAfter.String(), err)
		updateStatus(r.client, request.NamespacedName, k8sutil.ReconcileFailedStatus)
		return opcontroller.WaitForRequeueIfCephClusterNotReady, nil
	}
	// Set the object store context
	r.objContext = objContext

	// Generate user config
	userConfig := generateUserConfig(cephObjectStoreUser)
	r.userConfig = userConfig

	// DELETE: the CR was deleted
	if !cephObjectStoreUser.GetDeletionTimestamp().IsZero() {
		logger.Debugf("deleting pool %q", cephObjectStoreUser.Name)
		err := deleteUser(r.context, cephObjectStoreUser)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to delete ceph object user %q", cephObjectStoreUser.Name)
		}

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.client, cephObjectStoreUser)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	// validate the user settings
	err = ValidateUser(cephObjectStoreUser)
	if err != nil {
		updateStatus(r.client, request.NamespacedName, k8sutil.ReconcileFailedStatus)
		return reconcile.Result{}, errors.Wrapf(err, "invalid pool CR %q spec", cephObjectStoreUser.Name)
	}

	// CREATE/UPDATE CEPH USER
	reconcileResponse, err = r.reconcileCephUser(cephObjectStoreUser)
	if err != nil {
		updateStatus(r.client, request.NamespacedName, k8sutil.ReconcileFailedStatus)
		return reconcileResponse, err
	}

	// CREATE/UPDATE KUBERNETES SECRET
	reconcileResponse, err = r.reconcileCephUserSecret(cephObjectStoreUser)
	if err != nil {
		updateStatus(r.client, request.NamespacedName, k8sutil.ReconcileFailedStatus)
		return reconcileResponse, err
	}

	// Set Ready status, we are done reconciling
	updateStatus(r.client, request.NamespacedName, k8sutil.ReadyStatus)

	// Return and do not requeue
	logger.Debug("done reconciling")
	return reconcile.Result{}, nil
}

func (r *ReconcileObjectStoreUser) reconcileCephUser(cephObjectStoreUser *cephv1.CephObjectStoreUser) (reconcile.Result, error) {
	err := r.createCephUser(cephObjectStoreUser)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to create object store user %q", cephObjectStoreUser.Name)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileObjectStoreUser) createCephUser(u *cephv1.CephObjectStoreUser) error {
	logger.Infof("creating ceph object user %q in namespace %q", u.Name, u.Namespace)
	user, rgwerr, err := object.CreateUser(r.objContext, r.userConfig)
	if err != nil {
		if rgwerr == object.ErrorCodeFileExists {
			objectUser, _, err := object.GetUser(r.objContext, r.userConfig.UserID)
			if err != nil {
				return errors.Wrapf(err, "failed to get details from ceph object user %q", objectUser.UserID)
			}

			// Set access and secret key
			r.userConfig.AccessKey = objectUser.AccessKey
			r.userConfig.SecretKey = objectUser.SecretKey

			return nil
		}
		return errors.Wrapf(err, "failed to create ceph object user %q. error code %d", u.Name, rgwerr)
	}

	// Set access and secret key
	r.userConfig.AccessKey = user.AccessKey
	r.userConfig.SecretKey = user.SecretKey

	logger.Infof("created ceph object user %q", u.Name)
	return nil
}

func (r *ReconcileObjectStoreUser) isObjectStoreInitialized(u *cephv1.CephObjectStoreUser) (*object.Context, error) {
	objContext := object.NewContext(r.context, u.Spec.Store, u.Namespace)
	err := r.objectStoreInitialized(u)
	if err != nil {
		return objContext, errors.Wrap(err, "failed to detect if object store is initialized")
	}

	return objContext, nil
}

func generateUserConfig(user *cephv1.CephObjectStoreUser) object.ObjectUser {
	// Set DisplayName to match Name if DisplayName is not set
	displayName := user.Spec.DisplayName
	if len(displayName) == 0 {
		displayName = user.Name
	}

	// create the user
	userConfig := object.ObjectUser{
		UserID:      user.Name,
		DisplayName: &displayName,
	}

	return userConfig
}

func (r *ReconcileObjectStoreUser) generateCephUserSecret(u *cephv1.CephObjectStoreUser) *v1.Secret {
	// Store the keys in a secret
	secrets := map[string]string{
		"AccessKey": *r.userConfig.AccessKey,
		"SecretKey": *r.userConfig.SecretKey,
	}

	secretName := fmt.Sprintf("rook-ceph-object-user-%s-%s", u.Spec.Store, u.Name)
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
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

func (r *ReconcileObjectStoreUser) reconcileCephUserSecret(cephObjectStoreUser *cephv1.CephObjectStoreUser) (reconcile.Result, error) {
	// Generate Kubernetes Secret
	secret := r.generateCephUserSecret(cephObjectStoreUser)

	// Set owner ref to the object store user object
	err := controllerutil.SetControllerReference(cephObjectStoreUser, secret, r.scheme)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to set owner reference for ceph object user %q secret", secret.Name)
	}

	// Create Kubernetes Secret
	err = opcontroller.CreateOrUpdateObject(r.client, secret)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to create or update ceph object user %q secret", secret.Name)
	}

	logger.Infof("created ceph object user secret %q", secret.Name)
	return reconcile.Result{}, nil
}

func (r *ReconcileObjectStoreUser) objectStoreInitialized(cephObjectStoreUser *cephv1.CephObjectStoreUser) error {
	err := r.getObjectStore(cephObjectStoreUser)
	if err != nil {
		return err
	}
	logger.Debug("CephObjectStore exists")

	pods, err := r.getRgwPodList(cephObjectStoreUser)
	if err != nil {
		return err
	}

	// check if at least one pod is running
	if len(pods.Items) > 0 {
		logger.Debugf("CephObjectStore %q is running with %d pods", cephObjectStoreUser.Name, len(pods.Items))
		return nil
	}

	return errors.New("no rgw pod found")
}

func (r *ReconcileObjectStoreUser) getObjectStore(cephObjectStoreUser *cephv1.CephObjectStoreUser) error {
	// check if CephObjectStore CR is created
	objectStores := &cephv1.CephObjectStoreList{}
	err := r.client.List(context.TODO(), objectStores)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return errors.Wrapf(err, "CephObjectStore %q could not be found", cephObjectStoreUser.Name)
		}
		return errors.Wrap(err, "failed to get CephObjectStore")
	}

	return nil
}

func (r *ReconcileObjectStoreUser) getRgwPodList(cephObjectStoreUser *cephv1.CephObjectStoreUser) (*corev1.PodList, error) {
	pods := &corev1.PodList{}

	// check if ObjectStore is initialized
	// rook does this by starting the RGW pod(s)
	listOpts := []client.ListOption{
		client.InNamespace(cephObjectStoreUser.Namespace),
		client.MatchingLabels(labelsForRgw(cephObjectStoreUser.Spec.Store)),
	}

	err := r.client.List(context.TODO(), pods, listOpts...)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return pods, errors.Wrap(err, "no rgw pod could not be found")
		}
		return pods, errors.Wrap(err, "failed to list rgw pods")
	}

	return pods, nil
}

// Delete the user
func deleteUser(context *clusterd.Context, u *cephv1.CephObjectStoreUser) error {
	objContext := object.NewContext(context, u.Spec.Store, u.Namespace)
	_, rgwerr, err := object.DeleteUser(objContext, u.Name)
	if err != nil {
		if rgwerr == 3 {
			logger.Infof("ceph object user %q does not exist in store %q", u.Name, u.Spec.Store)
		} else {
			return errors.Wrapf(err, "failed to delete ceph object user %q", u.Name)
		}
	}

	logger.Infof("ceph object user %q deleted successfully", u.Name)
	return nil
}

// ValidateUser validates the user arguments
func ValidateUser(u *cephv1.CephObjectStoreUser) error {
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
func updateStatus(client client.Client, name types.NamespacedName, status string) {
	user := &cephv1.CephObjectStoreUser{}
	if err := client.Get(context.TODO(), name, user); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephObjectStoreUser resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve object store user %q to update status to %q. %v", name, status, err)
		return
	}
	if user.Status == nil {
		user.Status = &cephv1.Status{}
	}

	user.Status.Phase = status
	if err := opcontroller.UpdateStatus(client, user); err != nil {
		logger.Errorf("failed to set object store user %q status to %q. %v", name, status, err)
		return
	}
	logger.Debugf("object store user %q status updated to %q", name, status)
}
