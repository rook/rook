/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package nvmeof

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	csiopv1 "github.com/ceph/ceph-csi-operator/api/v1"
	"github.com/coreos/pkg/capnslog"
	addonsv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/api/csiaddons/v1alpha1"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName                = "ceph-nvmeof-gateway-controller"
	CephNVMeOFGatewayNameLabelKey = "ceph_nvmeof_gateway"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

// List of object resources to watch by the controller
var objectsToWatch = []client.Object{
	&v1.Service{TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: v1.SchemeGroupVersion.String()}},
	&appsv1.Deployment{TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: appsv1.SchemeGroupVersion.String()}},
}

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       reflect.TypeFor[cephv1.CephNVMeOFGateway]().Name(),
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

var currentAndDesiredCephVersion = opcontroller.CurrentAndDesiredCephVersion

// ReconcileCephNVMeOFGateway reconciles a CephNVMeOFGateway object
type ReconcileCephNVMeOFGateway struct {
	client                client.Client
	scheme                *runtime.Scheme
	context               *clusterd.Context
	cephClusterSpec       *cephv1.ClusterSpec
	clusterInfo           *cephclient.ClusterInfo
	opManagerContext      context.Context
	opConfig              opcontroller.OperatorConfig
	recorder              record.EventRecorder
	shouldRotateCephxKeys bool
}

// Add creates a new CephNVMeOFGateway Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext, opConfig))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) reconcile.Reconciler {
	return &ReconcileCephNVMeOFGateway{
		client:           mgr.GetClient(),
		scheme:           mgr.GetScheme(),
		context:          context,
		opManagerContext: opManagerContext,
		opConfig:         opConfig,
		recorder:         mgr.GetEventRecorderFor("rook-" + controllerName),
	}
}

func watchOwnedCoreObject[T client.Object](c controller.Controller, mgr manager.Manager, obj T) error {
	return c.Watch(
		source.Kind(
			mgr.GetCache(),
			obj,
			handler.TypedEnqueueRequestForOwner[T](
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&cephv1.CephNVMeOFGateway{},
			),
			opcontroller.WatchPredicateForNonCRDObject[T](&cephv1.CephNVMeOFGateway{TypeMeta: controllerTypeMeta}, mgr.GetScheme()),
		),
	)
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	err = addonsv1alpha1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}

	err = csiopv1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}

	// Watch for changes on the CephNVMeOFGateway CRD object
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephNVMeOFGateway{TypeMeta: controllerTypeMeta},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephNVMeOFGateway]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephNVMeOFGateway](mgr.GetScheme()),
		),
	)
	if err != nil {
		return err
	}

	// Watch all other resources
	for _, t := range objectsToWatch {
		err = watchOwnedCoreObject(c, mgr, t)
		if err != nil {
			return err
		}
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephNVMeOFGateway object and makes changes based on the state read
// and what is in the CephNVMeOFGateway.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephNVMeOFGateway) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	defer opcontroller.RecoverAndLogException()
	reconcileResponse, cephNVMeOFGateway, err := r.reconcile(request)
	return reporting.ReportReconcileResult(logger, r.recorder, request, &cephNVMeOFGateway, reconcileResponse, err)
}

func (r *ReconcileCephNVMeOFGateway) reconcile(request reconcile.Request) (reconcile.Result, cephv1.CephNVMeOFGateway, error) {
	cephNVMeOFGateway := &cephv1.CephNVMeOFGateway{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephNVMeOFGateway)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephNVMeOFGateway resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, *cephNVMeOFGateway, nil
		}
		return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to get CephNVMeOFGateway")
	}

	observedGeneration := cephNVMeOFGateway.ObjectMeta.Generation

	generationUpdated, err := opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephNVMeOFGateway)
	if err != nil {
		return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to add finalizer")
	}
	if generationUpdated {
		return reconcile.Result{}, *cephNVMeOFGateway, nil
	}

	if cephNVMeOFGateway.Status == nil {
		cephxUninitialized := keyring.UninitializedCephxStatus()
		err := r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, &cephxUninitialized, k8sutil.EmptyStatus)
		if err != nil {
			return opcontroller.ImmediateRetryResult, *cephNVMeOFGateway, errors.Wrapf(err, "failed set empty status")
		}
		cephNVMeOFGateway.Status = &cephv1.NVMeOFGatewayStatus{
			Status: cephv1.Status{},
			Cephx:  cephv1.LocalCephxStatus{Daemon: cephxUninitialized},
		}
	}

	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		if !cephNVMeOFGateway.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			err := opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephNVMeOFGateway)
			if err != nil {
				return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to remove finalizer")
			}
			r.recorder.Event(cephNVMeOFGateway, v1.EventTypeNormal, string(cephv1.ReconcileSucceeded), "successfully removed finalizer")
			return reconcile.Result{}, *cephNVMeOFGateway, nil
		}
		return reconcileResponse, *cephNVMeOFGateway, nil
	}
	r.cephClusterSpec = &cephCluster.Spec

	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace, r.cephClusterSpec)
	if err != nil {
		return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to populate cluster info")
	}

	if !cephNVMeOFGateway.GetDeletionTimestamp().IsZero() {
		logger.Infof("deleting ceph nvmeof gateway %q", cephNVMeOFGateway.Name)
		r.recorder.Eventf(cephNVMeOFGateway, v1.EventTypeNormal, string(cephv1.ReconcileStarted), "deleting CephNVMeOFGateway %q", cephNVMeOFGateway.Name)

		runningCephVersion, err := cephclient.LeastUptodateDaemonVersion(r.context, r.clusterInfo, config.MonType)
		if err != nil {
			return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrapf(err, "failed to retrieve current ceph version")
		}
		r.clusterInfo.CephVersion = runningCephVersion

		err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephNVMeOFGateway)
		if err != nil {
			return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to remove finalizer")
		}
		r.recorder.Event(cephNVMeOFGateway, v1.EventTypeNormal, string(cephv1.ReconcileSucceeded), "successfully removed finalizer")
		return reconcile.Result{}, *cephNVMeOFGateway, nil
	}

	runningCephVersion, desiredCephVersion, err := currentAndDesiredCephVersion(
		r.opManagerContext, r.opConfig.Image, cephNVMeOFGateway.Namespace, controllerName,
		k8sutil.NewOwnerInfo(cephNVMeOFGateway, r.scheme), r.context, r.cephClusterSpec, r.clusterInfo,
	)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, *cephNVMeOFGateway, nil
		}
		return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to detect ceph version")
	}

	if !cephCluster.Spec.External.Enable && !reflect.DeepEqual(*runningCephVersion, *desiredCephVersion) {
		return opcontroller.WaitForRequeueIfCephClusterIsUpgrading, *cephNVMeOFGateway,
			opcontroller.ErrorCephUpgradingRequeue(desiredCephVersion, runningCephVersion)
	}
	r.clusterInfo.CephVersion = *runningCephVersion

	if err := validateGateway(cephNVMeOFGateway); err != nil {
		return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrapf(err, "invalid configuration")
	}

	r.shouldRotateCephxKeys, err = keyring.ShouldRotateCephxKeys(cephCluster.Spec.Security.CephX.Daemon, *runningCephVersion,
		*desiredCephVersion, cephNVMeOFGateway.Status.Cephx.Daemon)
	if err != nil {
		return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to determine if cephx keys should be rotated")
	}
	if r.shouldRotateCephxKeys {
		logger.Infof("cephx keys will be rotated for %q", request.NamespacedName)
	}

	logger.Debug("reconciling ceph nvmeof gateway deployments")
	_, err = r.reconcileCreateCephNVMeOFGateway(cephNVMeOFGateway)
	if err != nil {
		return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to create deployments")
	}

	cephxStatus := keyring.UpdatedCephxStatus(r.shouldRotateCephxKeys, cephCluster.Spec.Security.CephX.Daemon, r.clusterInfo.CephVersion, cephNVMeOFGateway.Status.Cephx.Daemon)
	err = r.updateStatus(observedGeneration, request.NamespacedName, &cephxStatus, k8sutil.ReadyStatus)
	if err != nil {
		return opcontroller.ImmediateRetryResult, *cephNVMeOFGateway, errors.Wrapf(err, "failed to update status")
	}

	logger.Debug("done reconciling ceph nvmeof gateway")
	return reconcile.Result{}, *cephNVMeOFGateway, nil
}

func (r *ReconcileCephNVMeOFGateway) reconcileCreateCephNVMeOFGateway(cephNVMeOFGateway *cephv1.CephNVMeOFGateway) (reconcile.Result, error) {
	if r.cephClusterSpec.External.Enable {
		_, err := opcontroller.ValidateCephVersionsBetweenLocalAndExternalClusters(r.context, r.clusterInfo)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "refusing to run new crd")
		}
	}

	listOps := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s,%s=%s", k8sutil.AppAttr, AppName, CephNVMeOFGatewayNameLabelKey, cephNVMeOFGateway.Name),
	}
	deployments, err := r.context.Clientset.AppsV1().Deployments(cephNVMeOFGateway.Namespace).List(r.opManagerContext, listOps)
	if err != nil && !kerrors.IsNotFound(err) {
		return reconcile.Result{}, errors.Wrapf(err, "failed to list deployments")
	}

	currentGatewayCount := 0
	if deployments != nil {
		currentGatewayCount = len(deployments.Items)
	}

	if currentGatewayCount > cephNVMeOFGateway.Spec.Instances {
		logger.Infof("scaling down from %d to %d", currentGatewayCount, cephNVMeOFGateway.Spec.Instances)
		err := r.downCephNVMeOFGateway(cephNVMeOFGateway, currentGatewayCount)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to scale down")
		}
	}

	err = r.upCephNVMeOFGateway(cephNVMeOFGateway)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to update gateway")
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileCephNVMeOFGateway) upCephNVMeOFGateway(cephNVMeOFGateway *cephv1.CephNVMeOFGateway) error {
	var configMapName, configHash string
	var err error

	if cephNVMeOFGateway.Spec.ConfigMapRef == "" {
		configMapName, configHash, err = r.createConfigMap(cephNVMeOFGateway)
		if err != nil {
			return errors.Wrap(err, "failed to create configmap")
		}
		logger.Infof("configmap %q created/updated for nvmeof gateway %q with hash %q", configMapName, cephNVMeOFGateway.Name, configHash)
	}

	for i := 0; i < cephNVMeOFGateway.Spec.Instances; i++ {
		daemonID := fmt.Sprintf("%d", i)

		deployment, err := r.makeDeployment(cephNVMeOFGateway, daemonID, configHash)
		if err != nil {
			return errors.Wrapf(err, "failed to make deployment for %q", daemonID)
		}

		_, err = k8sutil.CreateOrUpdateDeployment(r.opManagerContext, r.context.Clientset, deployment)
		if err != nil {
			return errors.Wrapf(err, "failed to create/update deployment for %q", daemonID)
		}

		err = r.createCephNVMeOFService(cephNVMeOFGateway, daemonID)
		if err != nil {
			return errors.Wrapf(err, "failed to create service for %q", daemonID)
		}
	}

	return nil
}

func (r *ReconcileCephNVMeOFGateway) downCephNVMeOFGateway(cephNVMeOFGateway *cephv1.CephNVMeOFGateway, currentCount int) error {
	for i := cephNVMeOFGateway.Spec.Instances; i < currentCount; i++ {
		daemonID := fmt.Sprintf("%d", i)
		name := instanceName(cephNVMeOFGateway, daemonID)

		err := r.context.Clientset.AppsV1().Deployments(cephNVMeOFGateway.Namespace).Delete(r.opManagerContext, name, metav1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete deployment %q", name)
		}

		err = r.context.Clientset.CoreV1().Services(cephNVMeOFGateway.Namespace).Delete(r.opManagerContext, name, metav1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete service %q", name)
		}
	}

	return nil
}

func getNVMeOFGatewayConfig(poolName string) string {
	var config strings.Builder
	config.WriteString(`[gateway]
name = @@POD_NAME@@
group = @@ANA_GROUP@@
addr = @@POD_IP@@
port = 5500

[monitor]
port = 5499

[discovery]
port = 8009

[spdk]
rpc_socket = /var/tmp/spdk.sock

[ceph]
pool = `)
	config.WriteString(poolName)
	return config.String()
}

func (r *ReconcileCephNVMeOFGateway) generateConfigMap(nvmeof *cephv1.CephNVMeOFGateway) *v1.ConfigMap {
	poolName := nvmeof.Spec.Pool
	if poolName == "" {
		poolName = "nvmeofpool"
	}

	data := map[string]string{
		"config": getNVMeOFGatewayConfig(poolName),
	}

	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("rook-ceph-nvmeof-%s-config", nvmeof.Name),
			Namespace: nvmeof.Namespace,
			Labels: map[string]string{
				"app":                         AppName,
				CephNVMeOFGatewayNameLabelKey: nvmeof.Name,
			},
		},
		Data: data,
	}

	return configMap
}

func (r *ReconcileCephNVMeOFGateway) createConfigMap(cephNVMeOFGateway *cephv1.CephNVMeOFGateway) (string, string, error) {
	configMap := r.generateConfigMap(cephNVMeOFGateway)

	err := controllerutil.SetControllerReference(cephNVMeOFGateway, configMap, r.scheme)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to set owner reference for nvmeof configmap %q", configMap.Name)
	}

	if _, err := r.context.Clientset.CoreV1().ConfigMaps(cephNVMeOFGateway.Namespace).Create(r.opManagerContext, configMap, metav1.CreateOptions{}); err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return "", "", errors.Wrap(err, "failed to create nvmeof config map")
		}

		logger.Debugf("updating config map %q that already exists", configMap.Name)
		if _, err = r.context.Clientset.CoreV1().ConfigMaps(cephNVMeOFGateway.Namespace).Update(r.opManagerContext, configMap, metav1.UpdateOptions{}); err != nil {
			return "", "", errors.Wrap(err, "failed to update nvmeof config map")
		}
	}

	return configMap.Name, k8sutil.Hash(fmt.Sprintf("%v", configMap.Data)), nil
}

func validateGateway(g *cephv1.CephNVMeOFGateway) error {
	if g.Spec.Instances < 1 {
		return errors.New("at least one gateway instance is required")
	}
	if g.Spec.Group == "" {
		return errors.New("gateway group name is required")
	}
	return nil
}

func (r *ReconcileCephNVMeOFGateway) updateStatus(observedGeneration int64, namespacedName types.NamespacedName, cephxStatus *cephv1.CephxStatus, status string) error {
	nvmeof := &cephv1.CephNVMeOFGateway{}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := r.client.Get(r.opManagerContext, namespacedName, nvmeof)
		if err != nil {
			if kerrors.IsNotFound(err) {
				return nil
			}
			return errors.Wrapf(err, "failed to get for status update")
		}

		if nvmeof.Status == nil {
			nvmeof.Status = &cephv1.NVMeOFGatewayStatus{}
		}

		nvmeof.Status.Phase = status
		if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
			nvmeof.Status.ObservedGeneration = observedGeneration
		}
		if cephxStatus != nil {
			nvmeof.Status.Cephx.Daemon = *cephxStatus
		}

		if err := reporting.UpdateStatus(r.client, nvmeof); err != nil {
			return errors.Wrapf(err, "failed to set status")
		}
		return nil
	})
	if err != nil {
		return err
	}
	logger.Debugf("status updated to %q", status)
	return nil
}
