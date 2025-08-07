/*
Copyright 2020 The Rook Authors. All rights reserved.

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

// Package objectrealm to manage a rook object realm.
package realm

import (
	"context"
	"encoding/base64"
	"fmt"
	"reflect"
	"syscall"
	"time"

	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"k8s.io/apimachinery/pkg/runtime"
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
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	controllerName  = "ceph-object-realm-controller"
	accessKeyLength = 14
	secretKeyLength = 28
)

var waitForRequeueIfRealmNotReady = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

var cephObjectRealmKind = reflect.TypeOf(cephv1.CephObjectRealm{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephObjectRealmKind,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

// ReconcileObjectRealm reconciles a ObjectRealm object
type ReconcileObjectRealm struct {
	client           client.Client
	scheme           *runtime.Scheme
	context          *clusterd.Context
	clusterInfo      *cephclient.ClusterInfo
	opManagerContext context.Context
	recorder         record.EventRecorder
}

// Add creates a new CephObjectRealm Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context) reconcile.Reconciler {
	return &ReconcileObjectRealm{
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

	// Watch for changes on the CephObjectRealm CRD object
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephObjectRealm{TypeMeta: controllerTypeMeta},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephObjectRealm]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephObjectRealm](mgr.GetScheme()),
		),
	)
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephObjectRealm object and makes changes based on the state read
// and what is in the CephObjectRealm.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileObjectRealm) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	defer opcontroller.RecoverAndLogException()
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, cephObjectRealm, err := r.reconcile(request)

	return reporting.ReportReconcileResult(logger, r.recorder, request, &cephObjectRealm, reconcileResponse, err)
}

func (r *ReconcileObjectRealm) reconcile(request reconcile.Request) (reconcile.Result, cephv1.CephObjectRealm, error) {
	// Fetch the CephObjectRealm instance
	cephObjectRealm := &cephv1.CephObjectRealm{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephObjectRealm)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("CephObjectRealm %q resource not found. Ignoring since object must be deleted", request.NamespacedName.String())
			return reconcile.Result{}, *cephObjectRealm, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, *cephObjectRealm, errors.Wrap(err, "failed to get CephObjectRealm")
	}
	// update observedGeneration local variable with current generation value,
	// because generation can be changed before reconcile got completed
	// CR status will be updated at end of reconcile, so to reflect the reconcile has finished
	observedGeneration := cephObjectRealm.ObjectMeta.Generation

	// The CR was just created, initializing status fields
	if cephObjectRealm.Status == nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.EmptyStatus)
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		if !cephObjectRealm.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, *cephObjectRealm, nil
		}
		return reconcileResponse, *cephObjectRealm, nil
	}

	// DELETE: the CR was deleted
	if !cephObjectRealm.GetDeletionTimestamp().IsZero() {
		logger.Debugf("deleting realm CR %q", cephObjectRealm.Name)
		r.recorder.Eventf(cephObjectRealm, v1.EventTypeNormal, string(cephv1.ReconcileStarted), "deleting CephObjectRealm %q", cephObjectRealm.Name)

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, *cephObjectRealm, nil
	}

	// Populate clusterInfo during each reconcile
	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace, &cephCluster.Spec)
	if err != nil {
		return reconcile.Result{}, *cephObjectRealm, errors.Wrap(err, "failed to populate cluster info")
	}

	// validate the realm settings
	err = validateRealmCR(cephObjectRealm)
	if err != nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcileFailedStatus)
		return reconcile.Result{}, *cephObjectRealm, errors.Wrapf(err, "invalid CephObjectRealm CR %q", cephObjectRealm.Name)
	}

	// Start object reconciliation, updating status for this
	r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.ReconcilingStatus)

	// Create/Pull Ceph Realm
	if cephObjectRealm.Spec.IsPullRealm() {
		logger.Debugf("pull section in realm %q spec found", request.NamespacedName)
		_, err = r.pullCephRealm(cephObjectRealm)
		if err != nil {
			return reconcile.Result{}, *cephObjectRealm, err
		}
	} else {
		_, err = r.createRealmKeys(cephObjectRealm)
		if err != nil {
			return r.setFailedStatus(k8sutil.ObservedGenerationNotAvailable, cephObjectRealm, request.NamespacedName, "failed to create keys for realm", err)
		}

		_, err = r.createCephRealm(cephObjectRealm)
		if err != nil {
			return r.setFailedStatus(k8sutil.ObservedGenerationNotAvailable, cephObjectRealm, request.NamespacedName, "failed to create ceph realm", err)
		}
	}

	// Set the realm as default if specified and supported
	if cephObjectRealm.Spec.DefaultRealm {
		objCtx := object.NewContext(r.context, r.clusterInfo, cephObjectRealm.Namespace)
		if err := object.SetDefaultRealm(objCtx, cephObjectRealm.Name); err != nil {
			return reconcile.Result{}, *cephObjectRealm, errors.Wrapf(err,
				"failed to set realm %q as default", cephObjectRealm.Name)
		}
	}

	// update ObservedGeneration in status at the end of reconcile
	// Set Ready status, we are done reconciling
	r.updateStatus(observedGeneration, request.NamespacedName, k8sutil.ReadyStatus)

	// Return and do not requeue
	logger.Debugf("realm %q done reconciling", request.NamespacedName)
	return reconcile.Result{}, *cephObjectRealm, nil
}

func (r *ReconcileObjectRealm) pullCephRealm(realm *cephv1.CephObjectRealm) (reconcile.Result, error) {
	realmArg := fmt.Sprintf("--rgw-realm=%s", realm.Name)
	urlArg := fmt.Sprintf("--url=%s", realm.Spec.Pull.Endpoint)
	logger.Debugf("getting keys to pull realm for CephObjectRealm %q", realm.Name)
	accessKeyArg, secretKeyArg, err := object.GetRealmKeyArgs(r.opManagerContext, r.context, realm.Name, realm.Namespace)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return waitForRequeueIfRealmNotReady, err
		}
		return waitForRequeueIfRealmNotReady, errors.Wrap(err, "failed to get keys for realm")
	}
	logger.Debugf("keys found to pull realm for CephObjectRealm %q, getting ready to pull from endpoint %q", realm.Name, realm.Spec.Pull.Endpoint)

	objContext := object.NewContext(r.context, r.clusterInfo, realm.Name)
	output, err := object.RunAdminCommandNoMultisite(objContext, false, "realm", "pull", realmArg, urlArg, accessKeyArg, secretKeyArg)
	if err != nil {
		return waitForRequeueIfRealmNotReady, errors.Wrapf(err, "realm pull failed for reason: %v", output)
	}
	logger.Debugf("realm pull for %q from endpoint %q succeeded", realm.Name, realm.Spec.Pull.Endpoint)

	return reconcile.Result{}, nil
}

func (r *ReconcileObjectRealm) createCephRealm(realm *cephv1.CephObjectRealm) (reconcile.Result, error) {
	realmArg := fmt.Sprintf("--rgw-realm=%s", realm.Name)
	objContext := object.NewContext(r.context, r.clusterInfo, realm.Namespace)

	_, err := object.RunAdminCommandNoMultisite(objContext, true, "realm", "get", realmArg)
	if err != nil {
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
			logger.Debugf("ceph realm %q not found, running `radosgw-admin realm create`", realm.Name)
			_, err := object.RunAdminCommandNoMultisite(objContext, false, "realm", "create", realmArg)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "failed to create ceph realm %s", realm.Name)
			}
			logger.Debugf("created ceph realm %q", realm.Name)
		} else {
			return reconcile.Result{}, errors.Wrapf(err, "radosgw-admin realm get failed with code %d", code)
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileObjectRealm) createRealmKeys(realm *cephv1.CephObjectRealm) (reconcile.Result, error) {
	realmName := types.NamespacedName{Namespace: realm.Namespace, Name: realm.Name}
	logger.Debugf("generating access and secret keys for new realm %q", realmName.String())
	secretName := realm.Name + "-keys"

	// Check if the secret exists first, and check that it has the access information needed.
	secret, err := object.GetRealmKeySecret(r.opManagerContext, r.context, realmName)
	if err == nil {
		// secret exists, now verify access info. We don't need the args, but we do want to get the
		// error if the args can't be built
		_, _, err := object.GetRealmKeyArgsFromSecret(secret, realmName)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to ensure secret keys for CephObjectRealm %q", realmName.String())
		}
		return reconcile.Result{}, nil
	}

	// the realm's secret key and access key are randomly generated and then encoded to base64
	accessKey, err := mgr.GeneratePassword(accessKeyLength, mgr.AccessKey)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "access key failed to generate")
	}
	accessKey = base64.StdEncoding.EncodeToString([]byte(accessKey))

	secretKey, err := mgr.GeneratePassword(secretKeyLength, mgr.DefaultKey)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to generate secret key")
	}
	secretKey = base64.StdEncoding.EncodeToString([]byte(secretKey))

	logger.Debugf("creating secrets for new realm %q", realm.Name)

	secrets := map[string][]byte{
		object.AccessKeyName: []byte(accessKey),
		object.SecretKeyName: []byte(secretKey),
	}

	secret = &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: realm.Namespace,
		},
		Data: secrets,
		Type: k8sutil.RookType,
	}
	err = controllerutil.SetControllerReference(realm, secret, r.scheme)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to set owner reference of rgw secret %q", secret.Name)
	}

	if _, err = r.context.Clientset.CoreV1().Secrets(realm.Namespace).Create(r.opManagerContext, secret, metav1.CreateOptions{}); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to save rgw secrets")
	}
	logger.Infof("secret keys have been created for realm %q in secret %q", realm.Name, secret.Name)

	return reconcile.Result{}, nil
}

// validateRealmCR validates the realm arguments
func validateRealmCR(u *cephv1.CephObjectRealm) error {
	if u.Name == "" {
		return errors.New("missing name")
	}
	if u.Namespace == "" {
		return errors.New("missing namespace")
	}
	return nil
}

func (r *ReconcileObjectRealm) setFailedStatus(observedGeneration int64, cephObjectRealm *cephv1.CephObjectRealm, name types.NamespacedName, errMessage string, err error) (reconcile.Result, cephv1.CephObjectRealm, error) {
	r.updateStatus(observedGeneration, name, k8sutil.ReconcileFailedStatus)
	return reconcile.Result{}, *cephObjectRealm, errors.Wrapf(err, "%s", errMessage)
}

// updateStatus updates an realm with a given status
func (r *ReconcileObjectRealm) updateStatus(observedGeneration int64, name types.NamespacedName, status string) {
	objectRealm := &cephv1.CephObjectRealm{}
	if err := r.client.Get(r.opManagerContext, name, objectRealm); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("CephObjectRealm %q resource not found. Ignoring since object must be deleted", name)
			return
		}
		logger.Warningf("failed to retrieve object realm %q to update status to %q. %v", name, status, err)
		return
	}
	if objectRealm.Status == nil {
		objectRealm.Status = &cephv1.Status{}
	}

	objectRealm.Status.Phase = status
	if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
		objectRealm.Status.ObservedGeneration = observedGeneration
	}
	if err := reporting.UpdateStatus(r.client, objectRealm); err != nil {
		logger.Errorf("failed to set object realm %q status to %q. %v", name, status, err)
		return
	}
	logger.Debugf("object realm %q status updated to %q", name, status)
}
