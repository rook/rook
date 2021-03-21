/*
Copyright 2019 The Rook Authors. All rights reserved.

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

// Package client to manage a rook client.
package client

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	ceph "github.com/rook/rook/pkg/daemon/ceph/client"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

const (
	controllerName = "ceph-client-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

var cephClientKind = reflect.TypeOf(cephv1.CephClient{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephClientKind,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

// ReconcileCephClient reconciles a CephClient object
type ReconcileCephClient struct {
	client      client.Client
	scheme      *runtime.Scheme
	context     *clusterd.Context
	clusterInfo *cephclient.ClusterInfo
}

// Add creates a new CephClient Controller and adds it to the Manager. The Manager will set fields on the Controller
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

	return &ReconcileCephClient{
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
	logger.Info("successfully started")

	// Watch for changes on the CephClient CRD object
	err = c.Watch(&source.Kind{Type: &cephv1.CephClient{TypeMeta: controllerTypeMeta}}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate())
	if err != nil {
		return err
	}

	// Watch secrets
	err = c.Watch(&source.Kind{Type: &v1.Secret{TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: v1.SchemeGroupVersion.String()}}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &cephv1.CephClient{},
	}, opcontroller.WatchPredicateForNonCRDObject(&cephv1.CephClient{TypeMeta: controllerTypeMeta}, mgr.GetScheme()))
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephClient object and makes changes based on the state read
// and what is in the CephClient.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephClient) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileCephClient) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the CephClient instance
	cephClient := &cephv1.CephClient{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cephClient)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("cephClient resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "failed to get cephClient")
	}

	// Set a finalizer so we can do cleanup before the object goes away
	err = opcontroller.AddFinalizerIfNotPresent(r.client, cephClient)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer")
	}

	// The CR was just created, initializing status fields
	if cephClient.Status == nil {
		updateStatus(r.client, request.NamespacedName, cephv1.ConditionProgressing)
	}

	// Make sure a CephCluster is present otherwise do nothing
	_, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.client, r.context, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		// We skip the deletePool() function since everything is gone already
		//
		// Also, only remove the finalizer if the CephCluster is gone
		// If not, we should wait for it to be ready
		// This handles the case where the operator is not ready to accept Ceph command but the cluster exists
		if !cephClient.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Remove finalizer
			err = opcontroller.RemoveFinalizer(r.client, cephClient)
			if err != nil {
				return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, nil
		}
		return reconcileResponse, nil
	}

	// Populate clusterInfo during each reconcile
	r.clusterInfo, _, _, err = mon.LoadClusterInfo(r.context, request.NamespacedName.Namespace)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to populate cluster info")
	}

	// DELETE: the CR was deleted
	if !cephClient.GetDeletionTimestamp().IsZero() {
		logger.Debugf("deleting pool %q", cephClient.Name)
		err := r.deleteClient(cephClient)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to delete ceph client %q", cephClient.Name)
		}

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.client, cephClient)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	// validate the client settings
	err = ValidateClient(r.context, cephClient)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to validate client %q arguments", cephClient.Name)
	}

	// Create or Update client
	err = r.createOrUpdateClient(cephClient)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info("skipping reconcile since operator is still initializing")
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, nil
		}
		updateStatus(r.client, request.NamespacedName, cephv1.ConditionFailure)
		return reconcile.Result{}, errors.Wrapf(err, "failed to create or update client %q", cephClient.Name)
	}

	// Success! Let's update the status
	updateStatus(r.client, request.NamespacedName, cephv1.ConditionReady)

	// Return and do not requeue
	logger.Debug("done reconciling")
	return reconcile.Result{}, nil
}

// Create the client
func (r *ReconcileCephClient) createOrUpdateClient(cephClient *cephv1.CephClient) error {
	ctx := context.TODO()
	logger.Infof("creating client %s in namespace %s", cephClient.Name, cephClient.Namespace)

	// Generate the CephX details
	clientEntity, caps := genClientEntity(cephClient)

	// Check if client was created manually, create if necessary or update caps and create secret
	key, err := ceph.AuthGetKey(r.context, r.clusterInfo, clientEntity)
	if err != nil {
		key, err = ceph.AuthGetOrCreateKey(r.context, r.clusterInfo, clientEntity, caps)
		if err != nil {
			return errors.Wrapf(err, "failed to create client %q", cephClient.Name)
		}
	} else {
		err = ceph.AuthUpdateCaps(r.context, r.clusterInfo, clientEntity, caps)
		if err != nil {
			return errors.Wrapf(err, "client %q exists, failed to update client caps", cephClient.Name)
		}
	}

	// Generate Kubernetes Secret
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateCephUserSecretName(cephClient),
			Namespace: cephClient.Namespace,
		},
		StringData: map[string]string{
			cephClient.Name: key,
		},
		Type: k8sutil.RookType,
	}

	// Set CephClient owner ref to the Secret
	err = controllerutil.SetControllerReference(cephClient, secret, r.scheme)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference to ceph client secret %q", secret.Name)
	}

	// Create or Update Kubernetes Secret
	_, err = r.context.Clientset.CoreV1().Secrets(cephClient.Namespace).Get(ctx, secret.Name, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("creating secret for %q", secret.Name)
			if _, err := r.context.Clientset.CoreV1().Secrets(cephClient.Namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
				return errors.Wrapf(err, "failed to create secret for %q", secret.Name)
			}
			logger.Infof("created client %q", cephClient.Name)
			return nil
		}
		return errors.Wrapf(err, "failed to get secret for %q", secret.Name)
	}
	logger.Debugf("updating secret for %s", secret.Name)
	_, err = r.context.Clientset.CoreV1().Secrets(cephClient.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to update secret for %q", secret.Name)
	}

	logger.Infof("updated client %q", cephClient.Name)
	return nil
}

// Delete the client
func (r *ReconcileCephClient) deleteClient(cephClient *cephv1.CephClient) error {
	logger.Infof("deleting client object %q", cephClient.Name)
	if err := ceph.AuthDelete(r.context, r.clusterInfo, generateClientName(cephClient.Name)); err != nil {
		return errors.Wrapf(err, "failed to delete client %q", cephClient.Name)
	}

	logger.Infof("deleted client %q", cephClient.Name)
	return nil
}

// ValidateClient the client arguments
func ValidateClient(context *clusterd.Context, cephClient *cephv1.CephClient) error {
	// Validate name
	if cephClient.Name == "" {
		return errors.New("missing name")
	}
	reservedNames := regexp.MustCompile("^admin$|^rgw.*$|^rbd-mirror$|^osd.[0-9]*$|^bootstrap-(mds|mgr|mon|osd|rgw|^rbd-mirror)$")
	if reservedNames.Match([]byte(cephClient.Name)) {
		return errors.Errorf("ignoring reserved name %q", cephClient.Name)
	}

	// Validate Spec
	if cephClient.Spec.Caps == nil {
		return errors.New("no caps specified")
	}
	for _, cap := range cephClient.Spec.Caps {
		if cap == "" {
			return errors.New("no caps specified")
		}
	}

	return nil
}

func genClientEntity(cephClient *cephv1.CephClient) (string, []string) {
	caps := []string{}
	for name, cap := range cephClient.Spec.Caps {
		caps = append(caps, name, cap)
	}

	return generateClientName(cephClient.Name), caps
}

func generateClientName(name string) string {
	return fmt.Sprintf("client.%s", name)
}

// updateStatus updates an object with a given status
func updateStatus(client client.Client, name types.NamespacedName, status cephv1.ConditionType) {
	cephClient := &cephv1.CephClient{}
	if err := client.Get(context.TODO(), name, cephClient); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephClient resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve ceph client %q to update status to %q. %v", name, status, err)
		return
	}
	if cephClient.Status == nil {
		cephClient.Status = &cephv1.CephClientStatus{}
	}

	cephClient.Status.Phase = status
	if cephClient.Status.Phase == cephv1.ConditionReady {
		cephClient.Status.Info = generateStatusInfo(cephClient)
	}
	if err := opcontroller.UpdateStatus(client, cephClient); err != nil {
		logger.Errorf("failed to set ceph client %q status to %q. %v", name, status, err)
		return
	}
	logger.Debugf("ceph client %q status updated to %q", name, status)
}

func generateStatusInfo(client *cephv1.CephClient) map[string]string {
	m := make(map[string]string)
	m["secretName"] = generateCephUserSecretName(client)
	return m
}

func generateCephUserSecretName(client *cephv1.CephClient) string {
	return fmt.Sprintf("rook-ceph-client-%s", client.Name)
}
