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

// Package subvolumegroup to manage CephFS subvolume groups
package subvolumegroup

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"syscall"
	"time"

	csiopv1a1 "github.com/ceph/ceph-csi-operator/api/v1alpha1"
	"github.com/pkg/errors"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/util/exec"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/ceph/file"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	cephcsi "github.com/ceph/ceph-csi/api/deploy/kubernetes"
)

const (
	controllerName = "ceph-fs-subvolumegroup-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

var cephFilesystemSubVolumeGroup = reflect.TypeOf(cephv1.CephFilesystemSubVolumeGroup{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephFilesystemSubVolumeGroup,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

// ReconcileCephFilesystemSubVolumeGroup reconciles a CephFilesystemSubVolumeGroup object
type ReconcileCephFilesystemSubVolumeGroup struct {
	client           client.Client
	scheme           *runtime.Scheme
	context          *clusterd.Context
	clusterInfo      *cephclient.ClusterInfo
	opManagerContext context.Context
	opConfig         opcontroller.OperatorConfig
}

// Add creates a new CephFilesystemSubVolumeGroup Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext, opConfig))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) reconcile.Reconciler {
	return &ReconcileCephFilesystemSubVolumeGroup{
		client:           mgr.GetClient(),
		scheme:           mgr.GetScheme(),
		context:          context,
		opManagerContext: opManagerContext,
		opConfig:         opConfig,
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the CephFilesystemSubVolumeGroup CRD object
	err = c.Watch(source.Kind[client.Object](mgr.GetCache(), &cephv1.CephFilesystemSubVolumeGroup{TypeMeta: controllerTypeMeta}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate()))
	if err != nil {
		return err
	}

	err = csiopv1a1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephFilesystemSubVolumeGroup object and makes changes based on the state read
// and what is in the CephFilesystemSubVolumeGroup.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephFilesystemSubVolumeGroup) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %q. %v", request.NamespacedName, err)
	}

	return reconcileResponse, err
}

func (r *ReconcileCephFilesystemSubVolumeGroup) reconcile(request reconcile.Request) (reconcile.Result, error) {
	namespacedName := request.NamespacedName
	// Fetch the CephFilesystemSubVolumeGroup instance
	cephFilesystemSubVolumeGroup := &cephv1.CephFilesystemSubVolumeGroup{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephFilesystemSubVolumeGroup)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("cephFilesystemSubVolumeGroup resource %q not found. Ignoring since object must be deleted.", namespacedName)
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "failed to get cephFilesystemSubVolumeGroup")
	}
	// update observedGeneration local variable with current generation value,
	// because generation can be changed before reconcile got completed
	// CR status will be updated at end of reconcile, so to reflect the reconcile has finished
	observedGeneration := cephFilesystemSubVolumeGroup.ObjectMeta.Generation

	// Set a finalizer so we can do cleanup before the object goes away
	err = opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephFilesystemSubVolumeGroup)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer")
	}

	// The CR was just created, initializing status fields
	if cephFilesystemSubVolumeGroup.Status == nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, cephv1.ConditionProgressing)
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		// We skip the deleteSubVolumeGroup() function since everything is gone already
		//
		// Also, only remove the finalizer if the CephCluster is gone
		// If not, we should wait for it to be ready
		// This handles the case where the operator is not ready to accept Ceph command but the cluster exists
		if !cephFilesystemSubVolumeGroup.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Remove finalizer
			err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephFilesystemSubVolumeGroup)
			if err != nil {
				return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, nil
		}
		return reconcileResponse, nil
	}

	// Populate clusterInfo during each reconcile
	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace, &cephCluster.Spec)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to populate cluster info")
	}
	r.clusterInfo.Context = r.opManagerContext

	// DELETE: the CR was deleted
	if !cephFilesystemSubVolumeGroup.GetDeletionTimestamp().IsZero() {
		logger.Debugf("deleting subvolume group %q", namespacedName)
		// On external cluster, we don't delete the subvolume group, it has to be deleted manually
		if cephCluster.Spec.External.Enable {
			logger.Warningf("external subvolume group %q deletion is not supported, delete it manually", namespacedName)
		} else {
			err = r.deleteSubVolumeGroup(cephFilesystemSubVolumeGroup, &cephCluster)
			if err != nil {
				if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
					logger.Info(opcontroller.OperatorNotInitializedMessage)
					return opcontroller.WaitForRequeueIfOperatorNotInitialized, nil
				}
				return reconcile.Result{}, errors.Wrapf(err, "failed to delete ceph filesystem subvolume group %q", cephFilesystemSubVolumeGroup.Name)
			}
		}

		err = csi.SaveClusterConfig(r.context.Clientset, buildClusterID(cephFilesystemSubVolumeGroup), cephCluster.Namespace, r.clusterInfo, nil)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to save cluster config")
		}

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephFilesystemSubVolumeGroup)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	if cephCluster.Spec.External.Enable {
		logger.Debug("external subvolume group creation is not supported, create it manually, the controller will assume it's there")
		err = r.updateClusterConfig(cephFilesystemSubVolumeGroup, cephCluster)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to save cluster config")
		}
		r.updateStatus(observedGeneration, namespacedName, cephv1.ConditionReady)
		return reconcile.Result{}, nil
	}
	// Build the NamespacedName to fetch the Filesystem and make sure it exists, if not we cannot
	// create the subvolume group

	cephFilesystem := &cephv1.CephFilesystem{}
	cephFilesystemNamespacedName := types.NamespacedName{Name: cephFilesystemSubVolumeGroup.Spec.FilesystemName, Namespace: request.Namespace}
	err = r.client.Get(r.opManagerContext, cephFilesystemNamespacedName, cephFilesystem)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return reconcile.Result{}, errors.Wrapf(err, "failed to fetch ceph filesystem %q, cannot create subvolume group %q", cephFilesystemSubVolumeGroup.Spec.FilesystemName, cephFilesystemSubVolumeGroup.Name)
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "failed to get cephFilesystemSubVolumeGroup")
	}

	// If the CephFilesystem is not ready to accept commands, we should wait for it to be ready
	if cephFilesystem.Status.Phase != cephv1.ConditionReady {
		// We know the CR is present so it should a matter of second for it to become ready
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, errors.Wrapf(err, "failed to fetch ceph filesystem %q, cannot create subvolume group %q", cephFilesystemSubVolumeGroup.Spec.FilesystemName, cephFilesystemSubVolumeGroup.Name)
	}

	// Create or Update ceph filesystem subvolume group

	err = r.createOrUpdateSubVolumeGroup(cephFilesystemSubVolumeGroup)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, nil
		}
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, cephv1.ConditionFailure)
		return reconcile.Result{}, errors.Wrapf(err, "failed to create or update ceph filesystem subvolume group %q", cephFilesystemSubVolumeGroup.Name)
	}

	err = r.updateClusterConfig(cephFilesystemSubVolumeGroup, cephCluster)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to save cluster config")
	}

	err = cephclient.PinCephFSSubVolumeGroup(r.context, r.clusterInfo, cephFilesystemSubVolumeGroup.Spec.FilesystemName, cephFilesystemSubVolumeGroup, getSubvolumeGroupName(cephFilesystemSubVolumeGroup))
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to pin filesystem subvolume group %q", cephFilesystemSubVolumeGroup.Name)
	}

	r.updateStatus(observedGeneration, request.NamespacedName, cephv1.ConditionReady)

	if csi.EnableCSIOperator() {
		err = csi.CreateUpdateClientProfileSubVolumeGroup(r.clusterInfo.Context, r.client, r.clusterInfo, cephFilesystemNamespacedName, buildClusterID(cephFilesystemSubVolumeGroup), cephCluster.Name)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to create ceph csi-op config CR for subVolGrp ns")
		}
	}

	// Return and do not requeue
	logger.Debugf("done reconciling cephFilesystemSubVolumeGroup %q", namespacedName)
	return reconcile.Result{}, nil
}

func getSubvolumeGroupName(cephFilesystemSubVolumeGroup *cephv1.CephFilesystemSubVolumeGroup) string {
	if cephFilesystemSubVolumeGroup.Spec.Name != "" {
		return cephFilesystemSubVolumeGroup.Spec.Name
	}
	return cephFilesystemSubVolumeGroup.Name
}

func (r *ReconcileCephFilesystemSubVolumeGroup) updateClusterConfig(cephFilesystemSubVolumeGroup *cephv1.CephFilesystemSubVolumeGroup, cephCluster cephv1.CephCluster) error {
	// Update CSI config map
	// If the mon endpoints change, the mon health check go routine will take care of updating the
	// config map, so no special care is needed in this controller
	csiClusterConfigEntry := csi.CSIClusterConfigEntry{
		Namespace: r.clusterInfo.Namespace,
		ClusterInfo: cephcsi.ClusterInfo{
			Monitors: csi.MonEndpoints(r.clusterInfo.Monitors, cephCluster.Spec.RequireMsgr2()),
			CephFS: cephcsi.CephFS{
				SubvolumeGroup:     getSubvolumeGroupName(cephFilesystemSubVolumeGroup),
				KernelMountOptions: r.clusterInfo.CSIDriverSpec.CephFS.KernelMountOptions,
				FuseMountOptions:   r.clusterInfo.CSIDriverSpec.CephFS.FuseMountOptions,
			},
			ReadAffinity: cephcsi.ReadAffinity{
				Enabled:             r.clusterInfo.CSIDriverSpec.ReadAffinity.Enabled,
				CrushLocationLabels: r.clusterInfo.CSIDriverSpec.ReadAffinity.CrushLocationLabels,
			},
		},
	}

	// If the cluster has Multus enabled we need to append the network namespace of the driver's
	// holder DaemonSet in the csi configmap
	if cephCluster.Spec.Network.IsMultus() {
		netNamespaceFilePath, err := csi.GenerateNetNamespaceFilePath(r.opManagerContext, r.client, cephCluster.Namespace, r.opConfig.OperatorNamespace, csi.CephFSDriverShortName)
		if err != nil {
			return errors.Wrap(err, "failed to generate cephfs net namespace file path")
		}
		csiClusterConfigEntry.CephFS.NetNamespaceFilePath = netNamespaceFilePath
	}

	err := csi.SaveClusterConfig(r.context.Clientset, buildClusterID(cephFilesystemSubVolumeGroup), cephCluster.Namespace, r.clusterInfo, &csiClusterConfigEntry)
	if err != nil {
		return errors.Wrap(err, "failed to save cluster config")
	}
	return nil
}

// Create the ceph filesystem subvolume group
func (r *ReconcileCephFilesystemSubVolumeGroup) createOrUpdateSubVolumeGroup(cephFilesystemSubVolumeGroup *cephv1.CephFilesystemSubVolumeGroup) error {
	logger.Infof("creating ceph filesystem subvolume group %s in namespace %s", cephFilesystemSubVolumeGroup.Name, cephFilesystemSubVolumeGroup.Namespace)

	err := cephclient.CreateCephFSSubVolumeGroup(r.context, r.clusterInfo, cephFilesystemSubVolumeGroup.Spec.FilesystemName, getSubvolumeGroupName(cephFilesystemSubVolumeGroup), &cephFilesystemSubVolumeGroup.Spec)
	if err != nil {
		return errors.Wrapf(err, "failed to create ceph filesystem subvolume group %q", cephFilesystemSubVolumeGroup.Name)
	}

	return nil
}

// Delete the ceph filesystem subvolume group
func (r *ReconcileCephFilesystemSubVolumeGroup) deleteSubVolumeGroup(cephFilesystemSubVolumeGroup *cephv1.CephFilesystemSubVolumeGroup,
	cephCluster *cephv1.CephCluster) error {
	namespacedName := fmt.Sprintf("%s/%s", cephFilesystemSubVolumeGroup.Namespace, cephFilesystemSubVolumeGroup.Name)
	logger.Infof("deleting ceph filesystem subvolume group object %q", namespacedName)
	if err := cephclient.DeleteCephFSSubVolumeGroup(r.context, r.clusterInfo, cephFilesystemSubVolumeGroup.Spec.FilesystemName, getSubvolumeGroupName(cephFilesystemSubVolumeGroup)); err != nil {
		code, ok := exec.ExitStatus(err)
		// If the subvolume group does not exit, we should not return an error
		if ok && code == int(syscall.ENOENT) {
			logger.Debugf("ceph filesystem subvolume group %q do not exist", namespacedName)
			return nil
		}
		// If the subvolume group has subvolumes the command will fail with:
		// Error ENOTEMPTY: error in rmdir /volumes/csi
		if ok && (code == int(syscall.ENOTEMPTY)) {
			msg := fmt.Sprintf("failed to delete ceph filesystem subvolume group %q, remove the subvolumes first", cephFilesystemSubVolumeGroup.Name)
			if opcontroller.ForceDeleteRequested(cephFilesystemSubVolumeGroup.GetAnnotations()) {
				// cleanup cephFS subvolumes
				cleanupErr := r.cleanup(cephFilesystemSubVolumeGroup, cephCluster)
				if cleanupErr != nil {
					return errors.Wrapf(cleanupErr, "failed to clean up all the ceph resources created by subVolumeGroup %q", namespacedName)
				}
				msg = fmt.Sprintf("failed to delete ceph filesystem subvolume group %q, started clean up job to delete the subvolumes", cephFilesystemSubVolumeGroup.Name)
			}

			return errors.Wrapf(err, msg)
		}

		return errors.Wrapf(err, "failed to delete ceph filesystem subvolume group %q", cephFilesystemSubVolumeGroup.Name)
	}

	logger.Infof("deleted ceph filesystem subvolume group %q", namespacedName)
	return nil
}

// updateStatus updates an object with a given status
func (r *ReconcileCephFilesystemSubVolumeGroup) updateStatus(observedGeneration int64, name types.NamespacedName, status cephv1.ConditionType) {
	cephFilesystemSubVolumeGroup := &cephv1.CephFilesystemSubVolumeGroup{}
	if err := r.client.Get(r.opManagerContext, name, cephFilesystemSubVolumeGroup); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("CephFilesystemSubVolumeGroup %q not found. Ignoring since object must be deleted.", name)
			return
		}
		logger.Warningf("failed to retrieve ceph filesystem subvolume group %q to update status to %q. %v", name, status, err)
		return
	}
	if cephFilesystemSubVolumeGroup.Status == nil {
		cephFilesystemSubVolumeGroup.Status = &cephv1.CephFilesystemSubVolumeGroupStatus{}
	}

	cephFilesystemSubVolumeGroup.Status.Phase = status
	cephFilesystemSubVolumeGroup.Status.Info = map[string]string{
		"clusterID": buildClusterID(cephFilesystemSubVolumeGroup),
		"pinning":   formatPinning(cephFilesystemSubVolumeGroup.Spec.Pinning),
	}

	if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
		cephFilesystemSubVolumeGroup.Status.ObservedGeneration = observedGeneration
	}
	if err := reporting.UpdateStatus(r.client, cephFilesystemSubVolumeGroup); err != nil {
		logger.Errorf("failed to set ceph filesystem subvolume group %q status to %q. %v", name, status, err)
		return
	}
	logger.Debugf("ceph filesystem subvolume group %q status updated to %q", name, status)
}

func buildClusterID(cephFilesystemSubVolumeGroup *cephv1.CephFilesystemSubVolumeGroup) string {
	clusterID := fmt.Sprintf("%s-%s-file-%s", cephFilesystemSubVolumeGroup.Namespace, cephFilesystemSubVolumeGroup.Spec.FilesystemName, getSubvolumeGroupName(cephFilesystemSubVolumeGroup))
	return k8sutil.Hash(clusterID)
}

func (r *ReconcileCephFilesystemSubVolumeGroup) cleanup(svg *cephv1.CephFilesystemSubVolumeGroup, cephCluster *cephv1.CephCluster) error {
	logger.Infof("starting cleanup of the ceph resources for subVolumeGroup %q in namespace %q", svg.Name, svg.Namespace)
	svgName := svg.Spec.Name
	// use resource name if `spec.Name` is empty in the subvolumeGroup CR.
	if svgName == "" {
		svgName = svg.Name
	}
	cleanupConfig := map[string]string{
		opcontroller.CephFSSubVolumeGroupNameEnv: svgName,
		opcontroller.CephFSNameEnv:               svg.Spec.FilesystemName,
		opcontroller.CSICephFSRadosNamesaceEnv:   "csi",
		opcontroller.CephFSMetaDataPoolNameEnv:   file.GenerateMetaDataPoolName(svg.Spec.FilesystemName),
	}
	cleanup := opcontroller.NewResourceCleanup(svg, cephCluster, r.opConfig.Image, cleanupConfig)
	jobName := k8sutil.TruncateNodeNameForJob("cleanup-svg-%s", fmt.Sprintf("%s-%s", svg.Spec.FilesystemName, svg.Name))
	err := cleanup.StartJob(r.clusterInfo.Context, r.context.Clientset, jobName)
	if err != nil {
		return errors.Wrapf(err, "failed to run clean up job to clean the ceph resources in cephFS subVolumeGroup %q", svg.Name)
	}
	return nil
}

func formatPinning(pinning cephv1.CephFilesystemSubVolumeGroupSpecPinning) string {
	var formatted string

	if pinning.Export != nil {
		formatted = fmt.Sprintf("export=%d", *pinning.Export)
	} else if pinning.Distributed != nil {
		formatted = fmt.Sprintf("distributed=%d", *pinning.Distributed)
	} else if pinning.Random != nil {
		formatted = fmt.Sprintf("random=%.2f", *pinning.Random)
	} else {
		formatted = fmt.Sprintf("distributed=%d", 1)
	}

	return formatted
}
