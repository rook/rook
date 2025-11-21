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
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
)

const (
	controllerName = "ceph-client-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "client-controller")

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       reflect.TypeFor[cephv1.CephClient]().Name(),
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

// ReconcileCephClient reconciles a CephClient object
type ReconcileCephClient struct {
	client           client.Client
	scheme           *runtime.Scheme
	context          *clusterd.Context
	clusterInfo      *cephclient.ClusterInfo
	opManagerContext context.Context
	recorder         record.EventRecorder
}

// Add creates a new CephClient Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context) reconcile.Reconciler {
	return &ReconcileCephClient{
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

	// Watch for changes on the CephClient CRD object
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephClient{TypeMeta: controllerTypeMeta},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephClient]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephClient](mgr.GetScheme()),
		),
	)
	if err != nil {
		return err
	}

	// Watch secrets
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&v1.Secret{TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: v1.SchemeGroupVersion.String()}},
			handler.TypedEnqueueRequestForOwner[*v1.Secret](
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&cephv1.CephClient{},
			),
			opcontroller.WatchPredicateForNonCRDObject[*v1.Secret](&cephv1.CephClient{TypeMeta: controllerTypeMeta}, mgr.GetScheme()),
		),
	)
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
	defer opcontroller.RecoverAndLogException()
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, cephClient, err := r.reconcile(request)
	return reporting.ReportReconcileResult(logger, r.recorder, request, &cephClient, reconcileResponse, err)
}

func (r *ReconcileCephClient) reconcile(request reconcile.Request) (reconcile.Result, cephv1.CephClient, error) {
	// Fetch the CephClient instance
	cephClient := &cephv1.CephClient{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephClient)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("cephClient resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, *cephClient, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, *cephClient, errors.Wrap(err, "failed to get cephClient")
	}
	// update observedGeneration local variable with current generation value,
	// because generation can be changed before reconcile got completed
	// CR status will be updated at end of reconcile, so to reflect the reconcile has finished
	observedGeneration := cephClient.ObjectMeta.Generation

	// Set a finalizer so we can do cleanup before the object goes away
	generationUpdated, err := opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephClient)
	if err != nil {
		return reconcile.Result{}, *cephClient, errors.Wrap(err, "failed to add finalizer")
	}
	if generationUpdated {
		logger.Infof("reconciling the cephclient %q after adding finalizer", cephClient.Name)
		return reconcile.Result{}, *cephClient, nil
	}

	// The CR was just created, initializing status fields
	if cephClient.Status == nil {
		cephxUninitialized := keyring.UninitializedCephxStatus()
		err := r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, cephv1.ConditionProgressing, &cephxUninitialized)
		if err != nil {
			return reconcile.Result{}, *cephClient, errors.Wrapf(err, "failed to initialize ceph client %q status", request.NamespacedName)
		}
		cephClient.Status = &cephv1.CephClientStatus{
			Cephx: cephxUninitialized,
		}
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		// We skip the deletePool() function since everything is gone already
		//
		// Also, only remove the finalizer if the CephCluster is gone
		// If not, we should wait for it to be ready
		// This handles the case where the operator is not ready to accept Ceph command but the cluster exists
		if !cephClient.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Remove finalizer
			err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephClient)
			if err != nil {
				return opcontroller.ImmediateRetryResult, *cephClient, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, *cephClient, nil
		}
		return reconcileResponse, *cephClient, nil
	}

	// Populate clusterInfo during each reconcile
	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace, &cephCluster.Spec)
	if err != nil {
		return reconcile.Result{}, *cephClient, errors.Wrap(err, "failed to populate cluster info")
	}
	r.clusterInfo.Context = r.opManagerContext

	// DELETE: the CR was deleted
	if !cephClient.GetDeletionTimestamp().IsZero() {
		logger.Debugf("deleting client %q", cephClient.Name)
		err := r.deleteClient(cephClient)
		if err != nil {
			return reconcile.Result{}, *cephClient, errors.Wrapf(err, "failed to delete ceph client %q", cephClient.Name)
		}
		r.recorder.Eventf(cephClient, v1.EventTypeNormal, string(cephv1.ReconcileStarted), "deleting CephClient %q", cephClient.Name)

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephClient)
		if err != nil {
			return reconcile.Result{}, *cephClient, errors.Wrap(err, "failed to remove finalizer")
		}
		r.recorder.Event(cephClient, v1.EventTypeNormal, string(cephv1.ReconcileSucceeded), "successfully removed finalizer")

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, *cephClient, nil
	}

	// validate the client settings
	err = ValidateClient(r.context, cephClient)
	if err != nil {
		return reconcile.Result{}, *cephClient, errors.Wrapf(err, "failed to validate client %q arguments", cephClient.Name)
	}

	// Check the ceph version of the running monitors
	runningCephVersion, err := cephclient.LeastUptodateDaemonVersion(r.context, r.clusterInfo, config.MonType)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, *cephClient, nil
		}
		return reconcile.Result{}, *cephClient, errors.Wrapf(err, "failed to retrieve current ceph %q version", config.MonType)
	}

	shouldRotateCephxKeys, err := keyring.ShouldRotateCephxKeys(
		cephClient.Spec.Security.CephX, runningCephVersion, runningCephVersion, cephClient.Status.Cephx)
	if err != nil {
		return reconcile.Result{}, *cephClient, errors.Wrap(err, "failed to determine if cephx keys should be rotated")
	}

	// Create or Update client
	err = r.createOrUpdateClient(cephClient, shouldRotateCephxKeys)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, *cephClient, nil
		}
		var nilCephxStatus *cephv1.CephxStatus = nil // leave cephx status as-is
		statusErr := r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, cephv1.ConditionFailure, nilCephxStatus)
		if statusErr != nil {
			return reconcile.Result{}, *cephClient, errors.Wrapf(statusErr, "failed to set failed status for client %q", request.NamespacedName)
		}
		return reconcile.Result{}, *cephClient, errors.Wrapf(err, "failed to create or update client %q", cephClient.Name)
	}

	// update status with latest ObservedGeneration value at the end of reconcile
	// Success! Let's update the status
	cephxStatus := keyring.UpdatedCephxStatus(shouldRotateCephxKeys, cephClient.Spec.Security.CephX, runningCephVersion, cephClient.Status.Cephx)
	err = r.updateStatus(observedGeneration, request.NamespacedName, cephv1.ConditionReady, &cephxStatus)
	if err != nil {
		return reconcile.Result{}, *cephClient, errors.Wrapf(err, "failed to set final status for client %q", request.NamespacedName)
	}

	// Return and do not requeue
	logger.Debug("done reconciling")
	return reconcile.Result{}, *cephClient, nil
}

// Create the client
func (r *ReconcileCephClient) createOrUpdateClient(cephClient *cephv1.CephClient, shouldRotateCephxKeys bool) error {
	clientName := getClientName(cephClient)
	logger.Infof("creating client %s in namespace %s", clientName, cephClient.Namespace)

	// Generate the CephX details
	clientEntity, caps := genClientEntity(cephClient)

	// Check if client was created manually, create if necessary or update caps and create secret
	key, err := cephclient.AuthGetKey(r.context, r.clusterInfo, clientEntity)
	if err != nil {
		key, err = cephclient.AuthGetOrCreateKey(r.context, r.clusterInfo, clientEntity, caps)
		if err != nil {
			return errors.Wrapf(err, "failed to create client %q", clientName)
		}
	} else {
		err = cephclient.AuthUpdateCaps(r.context, r.clusterInfo, clientEntity, caps)
		if err != nil {
			return errors.Wrapf(err, "client %q exists, failed to update client caps", clientName)
		}
	}

	if shouldRotateCephxKeys {
		// rotate the CephX key if the user requested it
		logger.Infof("rotating cephx key for CephClient %v", types.NamespacedName{Name: cephClient.Name, Namespace: cephClient.Namespace})

		rotatedKey, err := cephclient.AuthRotate(r.context, r.clusterInfo, clientEntity)
		if err != nil {
			return errors.Wrapf(err, "failed to rotate cephx key for client %q", cephClient.Name)
		} else {
			key = rotatedKey
		}
	}
	// Generate Kubernetes Secret
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateCephUserSecretName(cephClient),
			Namespace: cephClient.Namespace,
			Annotations: map[string]string{
				keyring.KeyringAnnotation: "",
			},
		},
		StringData: map[string]string{
			clientName: key,
			// CSI requires userID and userKey in secret
			"userID":  cephClient.Name,
			"userKey": key,
		},
		Type: k8sutil.RookType,
	}
	return r.reconcileCephClientSecret(cephClient, secret)
}

func (r *ReconcileCephClient) reconcileCephClientSecret(
	cephClient *cephv1.CephClient,
	secret *v1.Secret,
) error {
	// Fetch existing secret
	_, getSecretErr := r.context.Clientset.CoreV1().
		Secrets(secret.Namespace).
		Get(r.clusterInfo.Context, secret.Name, metav1.GetOptions{})
	if getSecretErr != nil && !kerrors.IsNotFound(getSecretErr) {
		return errors.Wrapf(getSecretErr, "error fetching secret %q", secret.Name)
	}

	if err := controllerutil.SetControllerReference(cephClient, secret, r.scheme); err != nil {
		return errors.Wrapf(err, "failed to set owner reference on secret %q", secret.Name)
	}

	// Delete the secret if required
	if cephClient.Spec.RemoveSecret {
		if getSecretErr == nil {
			return k8sutil.DeleteSecretIfOwnedBy(r.clusterInfo.Context, r.context.Clientset,
				secret.Name, secret.Namespace, *metav1.GetControllerOf(secret))
		}
		return nil
	}

	if kerrors.IsNotFound(getSecretErr) {
		logger.Debugf("creating secret %q", secret.Namespace+"/"+secret.Name)
		if _, err := r.context.Clientset.CoreV1().
			Secrets(secret.Namespace).Create(r.clusterInfo.Context, secret, metav1.CreateOptions{}); err != nil {
			return errors.Wrapf(err, "failed to create secret %q", secret.Name)
		}
		logger.Infof("created secret for CephClient %q", cephClient.Namespace+"/"+cephClient.Name)
		return nil
	}

	return k8sutil.UpdateSecretIfOwnedBy(r.clusterInfo.Context, r.context.Clientset, secret)
}

// Delete the client
func (r *ReconcileCephClient) deleteClient(cephClient *cephv1.CephClient) error {
	clientName := getClientName(cephClient)
	logger.Infof("deleting client object %q", clientName)

	if err := cephclient.AuthDelete(r.context, r.clusterInfo, generateClientName(clientName)); err != nil {
		return errors.Wrapf(err, "failed to delete client %q", clientName)
	}

	logger.Infof("deleted client %q", clientName)
	return nil
}

// ValidateClient the client arguments
func ValidateClient(context *clusterd.Context, cephClient *cephv1.CephClient) error {
	reservedNames := regexp.MustCompile("^admin$|^rgw.*$|^rbd-mirror$|^osd.[0-9]*$|^bootstrap-(mds|mgr|mon|osd|rgw|rbd-mirror)$|^rbd-mirror-peer$")
	clientName := getClientName(cephClient)
	// validate the Client name
	if reservedNames.Match([]byte(clientName)) {
		return errors.Errorf("ignoring reserved name %q", clientName)
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

	return generateClientName(getClientName(cephClient)), caps
}

func getClientName(cephClient *cephv1.CephClient) string {
	name := cephClient.Name
	if cephClient.Spec.Name != "" {
		name = cephClient.Spec.Name
	}
	return name
}

func generateClientName(name string) string {
	return fmt.Sprintf("client.%s", name)
}

// updateStatus updates an object with a given status
func (r *ReconcileCephClient) updateStatus(observedGeneration int64, name types.NamespacedName, status cephv1.ConditionType, cephx *cephv1.CephxStatus) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cephClient := &cephv1.CephClient{}
		if err := r.client.Get(r.opManagerContext, name, cephClient); err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debug("CephClient resource not found. Ignoring since object must be deleted.")
				return nil
			}
			logger.Warningf("failed to retrieve ceph client %q to update status to %q. %v", name, status, err)
			return errors.Wrapf(err, "failed to retrieve ceph client %q to update status to %q", name, status)
		}
		if cephClient.Status == nil {
			cephClient.Status = &cephv1.CephClientStatus{}
		}

		cephClient.Status.Phase = status
		if cephClient.Status.Phase == cephv1.ConditionReady {
			cephClient.Status.Info = generateStatusInfo(cephClient)
		}
		if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
			cephClient.Status.ObservedGeneration = observedGeneration
		}
		if cephx != nil {
			cephClient.Status.Cephx = *cephx
		}
		if err := reporting.UpdateStatus(r.client, cephClient); err != nil {
			logger.Errorf("failed to set ceph client %q status to %q. %v", name, status, err)
			return errors.Wrapf(err, "failed to set ceph client %q status to %q", name, status)
		}
		return nil
	})
	if err != nil {
		return err
	}

	logger.Debugf("ceph client %q status updated to %q", name, status)
	return nil
}

func generateStatusInfo(client *cephv1.CephClient) map[string]string {
	m := make(map[string]string)
	// Set only if the secret is managed by the client
	if !client.Spec.RemoveSecret {
		m["secretName"] = generateCephUserSecretName(client)
	}

	return m
}

func generateCephUserSecretName(client *cephv1.CephClient) string {
	if client.Spec.SecretName != "" {
		return client.Spec.SecretName // return the secret name as requested by user.
	}
	return fmt.Sprintf("rook-ceph-client-%s", client.Name)
}
