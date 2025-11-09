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
	"gopkg.in/ini.v1"
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
	packageName    = "ceph-nvmeof-gateway"
	controllerName = packageName + "-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", packageName)

var objectsToWatch = []client.Object{
	&v1.Service{TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: v1.SchemeGroupVersion.String()}},
	&appsv1.Deployment{TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: appsv1.SchemeGroupVersion.String()}},
}

var controllerTypeMeta = metav1.TypeMeta{
	Kind:       reflect.TypeFor[cephv1.CephNVMeOFGateway]().Name(),
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

var currentAndDesiredCephVersion = opcontroller.CurrentAndDesiredCephVersion

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

// Add creates a new CephNVMeOFGateway Controller and adds it to the Manager.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext, opConfig))
}

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
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return errors.Wrap(err, "failed to create controller")
	}
	logger.Info("successfully started")

	err = addonsv1alpha1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return errors.Wrap(err, "failed to add addonsv1alpha1 to scheme")
	}

	err = csiopv1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return errors.Wrap(err, "failed to add csiopv1 to scheme")
	}

	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephNVMeOFGateway{TypeMeta: controllerTypeMeta},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephNVMeOFGateway]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephNVMeOFGateway](mgr.GetScheme()),
		),
	)
	if err != nil {
		return errors.Wrap(err, "failed to watch CephNVMeOFGateway CRD")
	}

	for _, t := range objectsToWatch {
		err = watchOwnedCoreObject(c, mgr, t)
		if err != nil {
			return errors.Wrapf(err, "failed to watch resource type %s", t.GetObjectKind().GroupVersionKind().String())
		}
	}

	return nil
}

// Reconcile reads the state of the cluster for a CephNVMeOFGateway object and makes changes based on the state read.
func (r *ReconcileCephNVMeOFGateway) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	defer opcontroller.RecoverAndLogException()
	reconcileResponse, cephNVMeOFGateway, err := r.reconcile(request)
	result, reportErr := reporting.ReportReconcileResult(logger, r.recorder, request, &cephNVMeOFGateway, reconcileResponse, err)
	return result, reportErr
}

func (r *ReconcileCephNVMeOFGateway) reconcile(request reconcile.Request) (reconcile.Result, cephv1.CephNVMeOFGateway, error) {
	cephNVMeOFGateway := &cephv1.CephNVMeOFGateway{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephNVMeOFGateway)
	if err != nil {
		if kerrors.IsNotFound(err) {
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

	_, err = r.reconcileCreateCephNVMeOFGateway(cephNVMeOFGateway)
	if err != nil {
		return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to create deployments")
	}

	cephxStatus := keyring.UpdatedCephxStatus(r.shouldRotateCephxKeys, cephCluster.Spec.Security.CephX.Daemon, r.clusterInfo.CephVersion, cephNVMeOFGateway.Status.Cephx.Daemon)
	err = r.updateStatus(observedGeneration, request.NamespacedName, &cephxStatus, k8sutil.ReadyStatus)
	if err != nil {
		logger.Errorf("failed to update status: %v", err)
		return opcontroller.ImmediateRetryResult, *cephNVMeOFGateway, errors.Wrapf(err, "failed to update status")
	}

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
		LabelSelector: fmt.Sprintf("%s=%s,app.kubernetes.io/part-of=%s", k8sutil.AppAttr, AppName, cephNVMeOFGateway.Name),
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
	for i := 0; i < cephNVMeOFGateway.Spec.Instances; i++ {
		daemonID := k8sutil.IndexToName(i)

		var configMapName, configHash string
		var err error

		if cephNVMeOFGateway.Spec.ConfigMapRef == "" {
			configMapName, configHash, err = r.createConfigMap(cephNVMeOFGateway, daemonID)
			if err != nil {
				return errors.Wrapf(err, "failed to create configmap for %q", daemonID)
			}
			logger.Infof("configmap %q created/updated for nvmeof gateway %q instance %q with hash %q", configMapName, cephNVMeOFGateway.Name, daemonID, configHash)
		} else {
			configMapName = cephNVMeOFGateway.Spec.ConfigMapRef
			configHash = "" // Will be empty for custom configmap
		}

		deployment, err := r.makeDeployment(cephNVMeOFGateway, daemonID, configMapName, configHash)
		if err != nil {
			return errors.Wrapf(err, "failed to make deployment for %q", daemonID)
		}

		err = controllerutil.SetControllerReference(cephNVMeOFGateway, deployment, r.scheme)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference for deployment %q", deployment.Name)
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
		daemonID := k8sutil.IndexToName(i)
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

// getNVMeOFGatewayConfig generates a complete nvmeof.conf configuration file
// with all values filled in (no placeholders). User overrides from nvmeofConfig
// are merged on top of the default configuration.
func getNVMeOFGatewayConfig(poolName, podName, podIP, anaGroup string, userConfig map[string]map[string]string) (string, error) {
	cfg := ini.Empty()
	// Set default [gateway] section
	gatewaySection, err := cfg.NewSection("gateway")
	if err != nil {
		return "", errors.Wrap(err, "failed to create gateway section")
	}
	gatewaySection.Key("name").SetValue(podName)
	gatewaySection.Key("group").SetValue(anaGroup)
	gatewaySection.Key("addr").SetValue(podIP)
	gatewaySection.Key("port").SetValue("5500")
	gatewaySection.Key("enable_auth").SetValue("False")
	gatewaySection.Key("state_update_notify").SetValue("True")
	gatewaySection.Key("state_update_timeout_in_msec").SetValue("2000")
	gatewaySection.Key("state_update_interval_sec").SetValue("5")
	gatewaySection.Key("enable_spdk_discovery_controller").SetValue("False")
	gatewaySection.Key("encryption_key").SetValue("/etc/ceph/encryption.key")
	gatewaySection.Key("rebalance_period_sec").SetValue("7")
	gatewaySection.Key("max_gws_in_grp").SetValue("16")
	gatewaySection.Key("max_ns_to_change_lb_grp").SetValue("8")
	gatewaySection.Key("verify_listener_ip").SetValue("False")
	gatewaySection.Key("enable_monitor_client").SetValue("True")

	// Set default [discovery] section
	discoverySection, err := cfg.NewSection("discovery")
	if err != nil {
		return "", errors.Wrap(err, "failed to create discovery section")
	}
	discoverySection.Key("addr").SetValue("0.0.0.0")
	discoverySection.Key("port").SetValue("8009")

	// Set default [ceph] section
	cephSection, err := cfg.NewSection("ceph")
	if err != nil {
		return "", errors.Wrap(err, "failed to create ceph section")
	}
	cephSection.Key("id").SetValue("admin")
	cephSection.Key("pool").SetValue(poolName)
	cephSection.Key("config_file").SetValue("/etc/ceph/ceph.conf")

	// Set default [mtls] section
	mtlsSection, err := cfg.NewSection("mtls")
	if err != nil {
		return "", errors.Wrap(err, "failed to create mtls section")
	}
	mtlsSection.Key("server_key").SetValue("./server.key")
	mtlsSection.Key("client_key").SetValue("./client.key")
	mtlsSection.Key("server_cert").SetValue("./server.crt")
	mtlsSection.Key("client_cert").SetValue("./client.crt")

	// Set default [spdk] section
	spdkSection, err := cfg.NewSection("spdk")
	if err != nil {
		return "", errors.Wrap(err, "failed to create spdk section")
	}
	spdkSection.Key("bdevs_per_cluster").SetValue("32")
	spdkSection.Key("mem_size").SetValue("4096")
	spdkSection.Key("tgt_path").SetValue("/usr/local/bin/nvmf_tgt")
	spdkSection.Key("timeout").SetValue("60.0")
	spdkSection.Key("rpc_socket").SetValue("/var/tmp/spdk.sock")

	// Set default [monitor] section
	monitorSection, err := cfg.NewSection("monitor")
	if err != nil {
		return "", errors.Wrap(err, "failed to create monitor section")
	}
	monitorSection.Key("port").SetValue("5499")

	// Apply user overrides
	for sectionName, options := range userConfig {
		section := cfg.Section(sectionName)
		if section == nil {
			// Section doesn't exist, create it
			var createErr error
			section, createErr = cfg.NewSection(sectionName)
			if createErr != nil {
				return "", errors.Wrapf(createErr, "failed to create section %q", sectionName)
			}
		}
		for key, value := range options {
			section.Key(key).SetValue(value)
		}
	}

	// Write to string with proper formatting
	var buf strings.Builder
	_, err = cfg.WriteTo(&buf)
	if err != nil {
		return "", errors.Wrap(err, "failed to write config to string")
	}

	return buf.String(), nil
}

func (r *ReconcileCephNVMeOFGateway) generateConfigMap(nvmeof *cephv1.CephNVMeOFGateway, daemonID string) (*v1.ConfigMap, error) {
	poolName := nvmeof.Spec.Pool
	anaGroup := nvmeof.Spec.Group
	podName := instanceName(nvmeof, daemonID)
	// Use placeholder that will be replaced at runtime with actual pod IP
	// The init container will replace @@POD_IP@@ with the actual pod IP
	podIP := "@@POD_IP@@"

	configContent, err := getNVMeOFGatewayConfig(poolName, podName, podIP, anaGroup, nvmeof.Spec.NVMeOFConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate nvmeof config")
	}
	data := map[string]string{
		"config": configContent,
	}

	configMapName := fmt.Sprintf("rook-ceph-nvmeof-%s-%s-config", nvmeof.Name, daemonID)

	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: nvmeof.Namespace,
			Labels:    getLabels(nvmeof, daemonID),
		},
		Data: data,
	}

	return configMap, nil
}

func (r *ReconcileCephNVMeOFGateway) createConfigMap(cephNVMeOFGateway *cephv1.CephNVMeOFGateway, daemonID string) (string, string, error) {
	configMap, err := r.generateConfigMap(cephNVMeOFGateway, daemonID)
	if err != nil {
		logger.Errorf("failed to generate configmap: %v", err)
		return "", "", err
	}

	err = controllerutil.SetControllerReference(cephNVMeOFGateway, configMap, r.scheme)
	if err != nil {
		logger.Errorf("failed to set owner reference: %v", err)
		return "", "", errors.Wrapf(err, "failed to set owner reference for nvmeof configmap %q", configMap.Name)
	}

	if _, err := r.context.Clientset.CoreV1().ConfigMaps(cephNVMeOFGateway.Namespace).Create(r.opManagerContext, configMap, metav1.CreateOptions{}); err != nil {
		if !kerrors.IsAlreadyExists(err) {
			logger.Errorf("failed to create configmap: %v", err)
			return "", "", errors.Wrap(err, "failed to create nvmeof config map")
		}

		if _, err = r.context.Clientset.CoreV1().ConfigMaps(cephNVMeOFGateway.Namespace).Update(r.opManagerContext, configMap, metav1.UpdateOptions{}); err != nil {
			logger.Errorf("failed to update configmap: %v", err)
			return "", "", errors.Wrap(err, "failed to update nvmeof config map")
		}
	}

	configHash := k8sutil.Hash(fmt.Sprintf("%v", configMap.Data))
	return configMap.Name, configHash, nil
}

func validateGateway(g *cephv1.CephNVMeOFGateway) error {
	if g.Spec.Instances < 1 {
		return errors.New("at least one gateway instance is required")
	}

	if g.Spec.Group == "" {
		return errors.New("gateway group name is required")
	}

	if g.Spec.Pool == "" {
		return errors.New("pool name is required")
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
			logger.Errorf("failed to get gateway: %v", err)
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
		logger.Errorf("retry loop failed: %v", err)
		return err
	}
	return nil
}
