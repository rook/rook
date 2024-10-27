/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package csi

import (
	"context"
	"os"
	"strconv"

	csiopv1a1 "github.com/ceph/ceph-csi-operator/api/v1alpha1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	addonsv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/api/csiaddons/v1alpha1"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"

	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi/peermap"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
)

const (
	controllerName = "rook-ceph-operator-csi-controller"
)

// ReconcileCSI reconciles a ceph-csi driver
type ReconcileCSI struct {
	scheme           *runtime.Scheme
	client           client.Client
	context          *clusterd.Context
	opManagerContext context.Context
	opConfig         opcontroller.OperatorConfig
	// the first cluster CR which will determine some settings for the csi driver
	firstCephCluster *cephv1.ClusterSpec
}

// Add creates a new Ceph CSI Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(opManagerContext, mgr, newReconciler(mgr, context, opManagerContext, opConfig), opConfig)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) reconcile.Reconciler {
	return &ReconcileCSI{
		scheme:           mgr.GetScheme(),
		client:           mgr.GetClient(),
		context:          context,
		opConfig:         opConfig,
		opManagerContext: opManagerContext,
	}
}

func add(ctx context.Context, mgr manager.Manager, r reconcile.Reconciler, opConfig opcontroller.OperatorConfig) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Infof("%s successfully started", controllerName)

	// Add CSIAddons client to controller mgr
	err = addonsv1alpha1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}

	// Watch for ConfigMap (operator config)
	configmapKind := source.Kind[client.Object](
		mgr.GetCache(),
		&v1.ConfigMap{TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: v1.SchemeGroupVersion.String()}},
		&handler.EnqueueRequestForObject{}, predicateController(ctx, mgr.GetClient(), opConfig.OperatorNamespace),
	)
	err = c.Watch(configmapKind)
	if err != nil {
		return err
	}

	// Watch for CephCluster
	clusterKind := source.Kind[client.Object](
		mgr.GetCache(),
		&cephv1.CephCluster{TypeMeta: metav1.TypeMeta{Kind: "CephCluster", APIVersion: v1.SchemeGroupVersion.String()}},
		&handler.EnqueueRequestForObject{}, predicateController(ctx, mgr.GetClient(), opConfig.OperatorNamespace),
	)
	err = c.Watch(clusterKind)
	if err != nil {
		return err
	}

	err = csiopv1a1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the operator config map and makes changes based on the state read
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCSI) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

// allow overriding for unit tests
var reconcileSaveCSIDriverOptions = SaveCSIDriverOptions

func (r *ReconcileCSI) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// reconcileResult is used to communicate the result of the reconciliation back to the caller
	var reconcileResult reconcile.Result

	ownerRef, err := k8sutil.GetDeploymentOwnerReference(r.opManagerContext, r.context.Clientset, os.Getenv(k8sutil.PodNameEnvVar), r.opConfig.OperatorNamespace)
	if err != nil {
		logger.Warningf("could not find deployment owner reference to assign to csi drivers. %v", err)
	}
	if ownerRef != nil {
		blockOwnerDeletion := false
		ownerRef.BlockOwnerDeletion = &blockOwnerDeletion
	}

	ownerInfo := k8sutil.NewOwnerInfoWithOwnerRef(ownerRef, r.opConfig.OperatorNamespace)
	// create an empty config map. config map will be filled with data
	// later when clusters have mons
	err = CreateCsiConfigMap(r.opManagerContext, r.opConfig.OperatorNamespace, r.context.Clientset, ownerInfo)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed creating csi config map")
	}

	// Fetch the operator's configmap. We force the NamespaceName to the operator since the request
	// could be a CephCluster. If so the NamespaceName will be the one from the cluster and thus the
	// CM won't be found
	opNamespaceName := types.NamespacedName{Name: opcontroller.OperatorSettingConfigMapName, Namespace: r.opConfig.OperatorNamespace}
	opConfig := &v1.ConfigMap{}
	err = r.client.Get(r.opManagerContext, opNamespaceName, opConfig)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("operator's configmap resource not found. will use default value or env var.")
			r.opConfig.Parameters = make(map[string]string)
		} else {
			// Error reading the object - requeue the request.
			return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to get operator's configmap")
		}
	} else {
		// Populate the operator's config
		r.opConfig.Parameters = opConfig.Data
	}

	serverVersion, err := r.context.Clientset.Discovery().ServerVersion()
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to get server version")
	}

	enableCSIOperator, err = strconv.ParseBool(k8sutil.GetValue(r.opConfig.Parameters, "ROOK_USE_CSI_OPERATOR", "false"))
	if err != nil {
		return reconcileResult, errors.Wrap(err, "unable to parse value for 'ROOK_USE_CSI_OPERATOR'")
	}

	// do not recocnile if csi driver is disabled
	disableCSI, err := strconv.ParseBool(k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_DISABLE_DRIVER", "false"))
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "unable to parse value for 'ROOK_CSI_DISABLE_DRIVER")
	} else if disableCSI {
		logger.Info("ceph csi driver is disabled")
	}

	// See if there is a CephCluster
	cephClusters := &cephv1.CephClusterList{}
	err = r.client.List(r.opManagerContext, cephClusters, &client.ListOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("no ceph cluster found not deploying ceph csi driver")
			EnableRBD, EnableCephFS, EnableNFS = false, false, false
			err = r.stopDrivers(serverVersion)
			if err != nil {
				return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to stop Drivers")
			}

			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to list ceph clusters")
	}

	// Do nothing if no ceph cluster is present
	if len(cephClusters.Items) == 0 {
		logger.Debug("no ceph cluster found not deploying ceph csi driver")
		EnableRBD, EnableCephFS, EnableNFS = false, false, false
		err = r.stopDrivers(serverVersion)
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to stop Drivers")
		}

		return reconcile.Result{}, nil
	}

	// if at least one cephcluster is present update the csi lograte sidecar
	// with the first listed ceph cluster specs with logrotate enabled
	r.setCSILogrotateParams(cephClusters.Items)

	err = peermap.CreateOrUpdateConfig(r.opManagerContext, r.context, &peermap.PeerIDMappings{})
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to create pool ID mapping config map")
	}

	exists, err := checkCsiCephConfigMapExists(r.opManagerContext, r.context.Clientset, r.opConfig.OperatorNamespace)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to get csi ceph.conf configmap")
	}
	CustomCSICephConfigExists = exists

	for i, cluster := range cephClusters.Items {
		if !cluster.DeletionTimestamp.IsZero() {
			logger.Debugf("ceph cluster %q is being deleting, no need to reconcile the csi driver", request.NamespacedName)
			return reconcile.Result{}, nil
		}

		if !cluster.Spec.External.Enable && cluster.Spec.CleanupPolicy.HasDataDirCleanPolicy() {
			logger.Debugf("ceph cluster %q has cleanup policy, the cluster will soon go away, no need to reconcile the csi driver", cluster.Name)
			return reconcile.Result{}, nil
		}

		if r.firstCephCluster == nil {
			r.firstCephCluster = &cephClusters.Items[i].Spec
		}

		// Load cluster info for later use in updating the ceph-csi configmap
		clusterInfo, _, _, err := opcontroller.LoadClusterInfo(r.context, r.opManagerContext, cluster.Namespace, &cephClusters.Items[i].Spec)
		if err != nil {
			// This avoids a requeue with exponential backoff and allows the controller to reconcile
			// more quickly when the cluster is ready.
			if errors.Is(err, opcontroller.ClusterInfoNoClusterNoSecret) {
				logger.Infof("cluster info for cluster %q is not ready yet, will retry in %s, proceeding with ready clusters", cluster.Name, opcontroller.WaitForRequeueIfCephClusterNotReady.RequeueAfter.String())
				reconcileResult = opcontroller.WaitForRequeueIfCephClusterNotReady
				continue
			}
			return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to load cluster info for cluster %q", cluster.Name)
		}
		clusterInfo.OwnerInfo = k8sutil.NewOwnerInfo(&cephClusters.Items[i], r.scheme)

		// ensure any remaining holder-related configs are cleared
		holderEnabled = false
		err = reconcileSaveCSIDriverOptions(r.context.Clientset, cluster.Namespace, clusterInfo)
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to update CSI driver options for cluster %q", cluster.Name)
		}

		// disable Rook-managed CSI drivers if CSI operator is enabled
		if EnableCSIOperator() {
			logger.Info("disabling csi-driver since EnableCSIOperator is true")
			err := r.stopDrivers(serverVersion)
			if err != nil {
				return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to stop csi Drivers")
			}
			err = r.reconcileOperatorConfig(cluster, clusterInfo, serverVersion)
			if err != nil {
				return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to reconcile csi-op config CR")
			}
			return reconcileResult, nil
		}
	}

	if !disableCSI && !EnableCSIOperator() {
		err = r.validateAndConfigureDrivers(serverVersion, ownerInfo)
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to configure ceph csi")
		}
	}

	return reconcileResult, nil
}

func (r *ReconcileCSI) reconcileOperatorConfig(cluster cephv1.CephCluster, clusterInfo *cephclient.ClusterInfo, serverVersion *version.Info) error {
	if err := r.setParams(serverVersion); err != nil {
		return errors.Wrapf(err, "failed to configure CSI parameters")
	}

	if err := validateCSIParam(); err != nil {
		return errors.Wrapf(err, "failed to validate CSI parameters")
	}

	err := r.createOrUpdateOperatorConfig(cluster)
	if err != nil {
		return errors.Wrap(err, "failed to configure csi operator operator config cr")
	}

	err = r.createOrUpdateDriverResources(cluster, clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to configure ceph-CSI operator drivers cr")
	}
	return nil
}

func (r *ReconcileCSI) setCSILogrotateParams(cephClustersItems []cephv1.CephCluster) {
	logger.Debug("set logrotate values in csi param")
	spec := cephClustersItems[0].Spec
	for _, cluster := range cephClustersItems {
		if cluster.Spec.LogCollector.Enabled {
			spec = cluster.Spec
			break
		}
	}
	csiRootPath = spec.DataDirHostPath
	if spec.DataDirHostPath == "" {
		csiRootPath = k8sutil.DataDir
	}

	CSIParam.CSILogRotation = spec.LogCollector.Enabled
	if spec.LogCollector.Enabled {
		maxSize, period := opcontroller.GetLogRotateConfig(spec)
		CSIParam.CSILogRotationMaxSize = maxSize.String()
		CSIParam.CSILogRotationPeriod = period
	}
}
