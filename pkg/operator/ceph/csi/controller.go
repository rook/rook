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
	"fmt"
	"os"

	csiopv1 "github.com/ceph/ceph-csi-operator/api/v1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
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
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Infof("%s successfully started", controllerName)

	err = addonsv1alpha1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}

	// Watch for ConfigMap (operator config)
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.ConfigMap{TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: corev1.SchemeGroupVersion.String()}},
			&handler.TypedEnqueueRequestForObject[*corev1.ConfigMap]{},
			cmPredicate(),
		),
	)
	if err != nil {
		return err
	}

	// Watch for CephCluster
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephCluster{TypeMeta: metav1.TypeMeta{Kind: "CephCluster", APIVersion: corev1.SchemeGroupVersion.String()}},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephCluster]{},
			cephClusterPredicate(ctx, mgr.GetClient(), opConfig.OperatorNamespace),
		),
	)
	if err != nil {
		return err
	}

	err = csiopv1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the operator config map and makes changes based on the state read
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCSI) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	defer opcontroller.RecoverAndLogException()
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileCSI) reconcile(request reconcile.Request) (reconcile.Result, error) {
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
	err = CreateCsiConfigMap(r.opManagerContext, r.opConfig.OperatorNamespace, r.context.Clientset, ownerInfo)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed creating csi config map")
	}

	if err := k8sutil.ApplyOperatorSettingsConfigmap(r.opManagerContext, r.context.Clientset); err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to apply operator settings configmap")
	}

	// Set driver names based on operator namespace
	driverPrefix := fmt.Sprintf("%s.", r.opConfig.OperatorNamespace)
	CephFSDriverName = driverPrefix + cephFSDriverSuffix
	RBDDriverName = driverPrefix + rbdDriverSuffix
	NFSDriverName = driverPrefix + nfsDriverSuffix

	// See if there is a CephCluster
	cephClusters := &cephv1.CephClusterList{}
	err = r.client.List(r.opManagerContext, cephClusters, &client.ListOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("no ceph cluster found not deploying ceph csi driver")
			return reconcile.Result{}, nil
		}
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to list ceph clusters")
	}

	if len(cephClusters.Items) == 0 {
		logger.Debug("no ceph cluster found not deploying ceph csi driver")
		return reconcile.Result{}, nil
	}

	err = peermap.CreateOrUpdateConfig(r.opManagerContext, r.context, &peermap.PeerIDMappings{})
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to create pool ID mapping config map")
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

		if r.firstCephCluster == nil {
			r.firstCephCluster = &cephClusters.Items[i].Spec
		}

		clusterInfo, _, _, err := opcontroller.LoadClusterInfo(r.context, r.opManagerContext, cluster.Namespace, &cephClusters.Items[i].Spec)
		if err != nil {
			if errors.Is(err, opcontroller.ClusterInfoNoClusterNoSecret) {
				logger.Infof("cluster info for cluster %q is not ready yet, will retry in %s, proceeding with ready clusters", cluster.Name, opcontroller.WaitForRequeueIfCephClusterNotReady.RequeueAfter.String())
				reconcileResult = opcontroller.WaitForRequeueIfCephClusterNotReady
				continue
			}
			return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to load cluster info for cluster %q", cluster.Name)
		}
		clusterInfo.OwnerInfo = k8sutil.NewOwnerInfo(&cephClusters.Items[i], r.scheme)
	}

	return reconcileResult, nil
}
