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

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi/peermap"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	controllerName = "rook-ceph-operator-csi-controller"
)

// ReconcileCSI reconciles a ceph-csi driver
type ReconcileCSI struct {
	client             client.Client
	context            *clusterd.Context
	opManagerContext   context.Context
	opConfig           opcontroller.OperatorConfig
	clustersWithHolder []ClusterDetail
}

// ClusterDetail is a struct that holds the information of a cluster, it knows its internals (like
// FSID through clusterInfo) and the Kubernetes object that represents it (like the CephCluster CRD)
type ClusterDetail struct {
	cluster     *cephv1.CephCluster
	clusterInfo *cephclient.ClusterInfo
}

// Add creates a new Ceph CSI Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(opManagerContext, mgr, newReconciler(mgr, context, opManagerContext, opConfig), opConfig)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) reconcile.Reconciler {
	return &ReconcileCSI{
		client:             mgr.GetClient(),
		context:            context,
		opConfig:           opConfig,
		opManagerContext:   opManagerContext,
		clustersWithHolder: []ClusterDetail{},
	}
}

func add(ctx context.Context, mgr manager.Manager, r reconcile.Reconciler, opConfig opcontroller.OperatorConfig) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Infof("%s successfully started", controllerName)

	// Watch for ConfigMap (operator config)
	err = c.Watch(&source.Kind{
		Type: &v1.ConfigMap{TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: v1.SchemeGroupVersion.String()}}}, &handler.EnqueueRequestForObject{}, predicateController(ctx, mgr.GetClient(), opConfig.OperatorNamespace))
	if err != nil {
		return err
	}

	// Watch for CephCluster
	err = c.Watch(&source.Kind{
		Type: &cephv1.CephCluster{TypeMeta: metav1.TypeMeta{Kind: "CephCluster", APIVersion: v1.SchemeGroupVersion.String()}}}, &handler.EnqueueRequestForObject{}, predicateController(ctx, mgr.GetClient(), opConfig.OperatorNamespace))
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

func (r *ReconcileCSI) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// reconcileResult is used to communicate the result of the reconciliation back to the caller
	var reconcileResult reconcile.Result

	serverVersion, err := r.context.Clientset.Discovery().ServerVersion()
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to get server version")
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

	// // Do not nothing if no ceph cluster is present
	if len(cephClusters.Items) == 0 {
		logger.Debug("no ceph cluster found not deploying ceph csi driver")
		EnableRBD, EnableCephFS, EnableNFS = false, false, false
		err = r.stopDrivers(serverVersion)
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to stop Drivers")
		}

		return reconcile.Result{}, nil
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

	csiHostNetworkEnabled, err := strconv.ParseBool(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_HOST_NETWORK", "true"))
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to parse value for 'CSI_ENABLE_HOST_NETWORK'")
	}

	for i, cluster := range cephClusters.Items {
		if !cluster.DeletionTimestamp.IsZero() {
			logger.Debugf("ceph cluster %q is being deleting, no need to reconcile the csi driver", request.NamespacedName)
			return reconcile.Result{}, nil
		}

		if !cluster.Spec.External.Enable && cluster.Spec.CleanupPolicy.HasDataDirCleanPolicy() {
			logger.Debugf("ceph cluster %q has cleanup policy, the cluster will soon go away, no need to reconcile the csi driver", cluster.Name)
			return reconcile.Result{}, nil
		}

		holderEnabled := !csiHostNetworkEnabled || cluster.Spec.Network.IsMultus()
		// Do we have a multus cluster or csi host network disabled?
		// If so deploy the plugin holder with the fsid attached
		if holderEnabled {
			// Load cluster info for later use in updating the ceph-csi configmap
			clusterInfo, _, _, err := opcontroller.LoadClusterInfo(r.context, r.opManagerContext, cluster.Namespace, &cluster.Spec)
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

			logger.Debugf("cluster %q is running on multus or CSI_ENABLE_HOST_NETWORK is false, deploying the ceph-csi plugin holder", cluster.Name)

			r.clustersWithHolder = append(r.clustersWithHolder, ClusterDetail{cluster: &cephClusters.Items[i], clusterInfo: clusterInfo})

		} else {
			logger.Debugf("not a multus cluster %q or CSI_ENABLE_HOST_NETWORK is true, not deploying the ceph-csi plugin holder", request.NamespacedName)
		}
	}

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

	err = peermap.CreateOrUpdateConfig(r.opManagerContext, r.context, &peermap.PeerIDMappings{})
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to create pool ID mapping config map")
	}

	exists, err := checkCsiCephConfigMapExists(r.opManagerContext, r.context.Clientset, r.opConfig.OperatorNamespace)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to get csi ceph.conf configmap")
	}
	CustomCSICephConfigExists = exists

	err = r.validateAndConfigureDrivers(serverVersion, ownerInfo, cephClusters.Items[0].Spec.Resources)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to configure ceph csi")
	}

	return reconcileResult, nil
}
