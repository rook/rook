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

	csiopv1 "github.com/ceph/ceph-csi-operator/api/v1"
	"github.com/pkg/errors"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/log"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/ceph/file"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"

	cephcsi "github.com/ceph/ceph-csi/api/deploy/kubernetes"
)

const (
	controllerName             = "ceph-fs-subvolumegroup-controller"
	cephSVGFileSystemNameIndex = "FilesystemName/subvolumeGroupName"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       reflect.TypeFor[cephv1.CephFilesystemSubVolumeGroup]().Name(),
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
	if err := mgr.GetFieldIndexer().IndexField(opManagerContext, &cephv1.CephFilesystemSubVolumeGroup{}, cephSVGFileSystemNameIndex, func(obj client.Object) []string {
		svg, ok := obj.(*cephv1.CephFilesystemSubVolumeGroup)
		if !ok {
			return nil
		}

		return []string{fmt.Sprintf("%s/%s", svg.Spec.FilesystemName, getSubvolumeGroupName(svg))}
	}); err != nil {
		return fmt.Errorf("failed to index CephFilesystemSubVolumeGroup by %s: %v", cephSVGFileSystemNameIndex, err)
	}
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
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephFilesystemSubVolumeGroup{TypeMeta: controllerTypeMeta},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephFilesystemSubVolumeGroup]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephFilesystemSubVolumeGroup](mgr.GetScheme()),
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

// Reconcile reads that state of the cluster for a CephFilesystemSubVolumeGroup object and makes changes based on the state read
// and what is in the CephFilesystemSubVolumeGroup.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephFilesystemSubVolumeGroup) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	defer opcontroller.RecoverAndLogException()
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		log.NamedError(request.NamespacedName, logger, "failed to reconcile %q. %v", request.NamespacedName, err)
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
			log.NamedDebug(request.NamespacedName, logger, "cephFilesystemSubVolumeGroup resource %q not found. Ignoring since object must be deleted.", namespacedName)
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
	generationUpdated, err := opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephFilesystemSubVolumeGroup)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer")
	}
	if generationUpdated {
		log.NamedInfo(request.NamespacedName, logger, "reconciling the subvolume group %q after adding finalizer", cephFilesystemSubVolumeGroup.Name)
		return reconcile.Result{}, nil
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
		log.NamedDebug(request.NamespacedName, logger, "deleting subvolume group %q", namespacedName)

		cephFsSvgList := &cephv1.CephFilesystemSubVolumeGroupList{}
		namespaceListOpts := client.InNamespace(cephCluster.Namespace)
		// List cephFilesystemSubvolumeGroup CR based on filesystem and spec.name
		matchingKey := fmt.Sprintf("%s/%s", cephFilesystemSubVolumeGroup.Spec.FilesystemName, getSubvolumeGroupName(cephFilesystemSubVolumeGroup))
		err = r.client.List(r.opManagerContext, cephFsSvgList, &client.MatchingFields{cephSVGFileSystemNameIndex: matchingKey}, namespaceListOpts)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to list cephFilesystemSubvolumeGroup")
		}

		// On external cluster, we don't delete the subvolume group, it has to be deleted manually
		if cephCluster.Spec.External.Enable {
			log.NamedWarning(namespacedName, logger, "external subvolume group deletion is not supported, delete it manually")
		} else if len(cephFsSvgList.Items) <= 1 {
			// If we have more than one cephFilesystemSubvolumeGroup CR with same spec.filesystem and same spec.name,
			// skip the call to deleteSubVolumeGroup(). This allows the finalizer to be removed without
			// checking if the subvolume group contains any data. Thus, any extra CRs referencing the same
			// subvolume group and filesystem can be easily deleted. Only the last subvolumegroup CR referencing the same
			// svg would actually check if there is data in the svg.
			err = r.deleteSubVolumeGroup(cephFilesystemSubVolumeGroup, &cephCluster)
			if err != nil {
				if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
					logger.Info(opcontroller.OperatorNotInitializedMessage)
					return opcontroller.WaitForRequeueIfOperatorNotInitialized, nil
				}
				return reconcile.Result{}, errors.Wrapf(err, "failed to delete ceph filesystem subvolume group %q", cephFilesystemSubVolumeGroup.Name)
			}
		} else {
			log.NamedInfo(request.NamespacedName, logger, "Removing finalizer from SVG CR %s without checking if the subvolume group contains any data as more than one SVG(count %d) contains the same filesystem and same SVG.", cephFilesystemSubVolumeGroup.Name, len(cephFsSvgList.Items))
		}

		if len(cephFsSvgList.Items) <= 1 {
			err = csi.SaveClusterConfig(r.context.Clientset, buildClusterID(cephFilesystemSubVolumeGroup), cephCluster.Namespace, r.clusterInfo, nil)
			if err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to save cluster config")
			}
		}

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephFilesystemSubVolumeGroup)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	// Detect running Ceph version
	runningCephVersion, err := cephclient.LeastUptodateDaemonVersion(r.context, r.clusterInfo, config.OsdType)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to retrieve current ceph %q version", config.OsdType)
	}
	r.clusterInfo.CephVersion = runningCephVersion

	cephFilesystemSubVolumeGroupName := cephFilesystemSubVolumeGroup.Name
	if cephFilesystemSubVolumeGroup.Spec.Name != "" {
		cephFilesystemSubVolumeGroupName = cephFilesystemSubVolumeGroup.Spec.Name
	}
	if cephCluster.Spec.External.Enable {
		log.NamedDebug(request.NamespacedName, logger, "skip creating external subvolume in external mode, create it manually, the controller will assume it's there")
		err = r.updateClusterConfig(cephFilesystemSubVolumeGroup, cephCluster)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to save cluster config")
		}
		r.updateStatus(observedGeneration, namespacedName, cephv1.ConditionReady)
		if csi.EnableCSIOperator() {
			err = csi.CreateUpdateClientProfileSubVolumeGroup(r.clusterInfo.Context, r.client, r.clusterInfo, cephFilesystemSubVolumeGroupName, buildClusterID(cephFilesystemSubVolumeGroup))
			if err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to create ceph csi-op config CR for subvolume")
			}
		}
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
	if cephFilesystem.Status == nil || cephFilesystem.Status.Phase != cephv1.ConditionReady {
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
		err = csi.CreateUpdateClientProfileSubVolumeGroup(r.clusterInfo.Context, r.client, r.clusterInfo, cephFilesystemSubVolumeGroupName, buildClusterID(cephFilesystemSubVolumeGroup))
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to create ceph csi-op config CR for subvolumeGroup")
		}
	}

	// Return and do not requeue
	log.NamedDebug(request.NamespacedName, logger, "done reconciling cephFilesystemSubVolumeGroup %q", namespacedName)
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
			Monitors: csi.MonEndpoints(r.clusterInfo.AllMonitors(), cephCluster.Spec.RequireMsgr2()),
			CephFS: cephcsi.CephFS{
				SubvolumeGroup:     getSubvolumeGroupName(cephFilesystemSubVolumeGroup),
				KernelMountOptions: r.clusterInfo.CSIDriverSpec.CephFS.KernelMountOptions,
				FuseMountOptions:   r.clusterInfo.CSIDriverSpec.CephFS.FuseMountOptions,
			},
			ReadAffinity: cephcsi.ReadAffinity{
				Enabled:             csi.ReadAffinityEnabled(r.clusterInfo.CSIDriverSpec.ReadAffinity.Enabled, r.clusterInfo.CephVersion),
				CrushLocationLabels: r.clusterInfo.CSIDriverSpec.ReadAffinity.CrushLocationLabels,
			},
		},
	}

	csiClusterConfigEntry.CephFS.NetNamespaceFilePath = ""

	err := csi.SaveClusterConfig(r.context.Clientset, buildClusterID(cephFilesystemSubVolumeGroup), cephCluster.Namespace, r.clusterInfo, &csiClusterConfigEntry)
	if err != nil {
		return errors.Wrap(err, "failed to save cluster config")
	}
	return nil
}

// Create the ceph filesystem subvolume group
func (r *ReconcileCephFilesystemSubVolumeGroup) createOrUpdateSubVolumeGroup(cephFilesystemSubVolumeGroup *cephv1.CephFilesystemSubVolumeGroup) error {
	nsName := opcontroller.NsName(cephFilesystemSubVolumeGroup.Namespace, cephFilesystemSubVolumeGroup.Name)
	log.NamedInfo(nsName, logger, "creating ceph filesystem subvolume group")

	err := cephclient.CreateCephFSSubVolumeGroup(r.context, r.clusterInfo, cephFilesystemSubVolumeGroup.Spec.FilesystemName, getSubvolumeGroupName(cephFilesystemSubVolumeGroup), &cephFilesystemSubVolumeGroup.Spec)
	if err != nil {
		return errors.Wrapf(err, "failed to create ceph filesystem subvolume group %q", cephFilesystemSubVolumeGroup.Name)
	}

	return nil
}

// Delete the ceph filesystem subvolume group
func (r *ReconcileCephFilesystemSubVolumeGroup) deleteSubVolumeGroup(cephFilesystemSubVolumeGroup *cephv1.CephFilesystemSubVolumeGroup,
	cephCluster *cephv1.CephCluster,
) error {
	nsName := opcontroller.NsName(cephFilesystemSubVolumeGroup.Namespace, cephFilesystemSubVolumeGroup.Name)
	log.NamedInfo(nsName, logger, "deleting ceph filesystem subvolume group object")
	if err := cephclient.DeleteCephFSSubVolumeGroup(r.context, r.clusterInfo, cephFilesystemSubVolumeGroup.Spec.FilesystemName, getSubvolumeGroupName(cephFilesystemSubVolumeGroup)); err != nil {
		code, ok := exec.ExitStatus(err)
		// If the subvolume group does not exit, we should not return an error
		if ok && code == int(syscall.ENOENT) {
			log.NamedDebug(nsName, logger, "ceph filesystem subvolume group does not exist")
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
					return errors.Wrapf(cleanupErr, "failed to clean up all the ceph resources created by subVolumeGroup %q", nsName)
				}
				msg = fmt.Sprintf("failed to delete ceph filesystem subvolume group %q, started clean up job to delete the subvolumes", cephFilesystemSubVolumeGroup.Name)
			}

			return errors.Wrapf(err, "%s", msg)
		}

		return errors.Wrapf(err, "failed to delete ceph filesystem subvolume group %q", cephFilesystemSubVolumeGroup.Name)
	}

	log.NamedInfo(nsName, logger, "deleted ceph filesystem subvolume group")
	return nil
}

// updateStatus updates an object with a given status
func (r *ReconcileCephFilesystemSubVolumeGroup) updateStatus(observedGeneration int64, name types.NamespacedName, status cephv1.ConditionType) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cephFilesystemSubVolumeGroup := &cephv1.CephFilesystemSubVolumeGroup{}
		if err := r.client.Get(r.opManagerContext, name, cephFilesystemSubVolumeGroup); err != nil {
			if kerrors.IsNotFound(err) {
				log.NamedDebug(name, logger, "CephFilesystemSubVolumeGroup not found. Ignoring since object must be deleted.")
				return nil
			}
			return errors.Wrapf(err, "failed to retrieve ceph filesystem subvolume group %q to update status to %q", name, status)
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
			return errors.Wrapf(err, "failed to set ceph filesystem subvolume group %q status to %q", name, status)
		}
		return nil
	})
	if err != nil {
		log.NamedError(name, logger, "failed to update ceph filesystem subvolume group status to %q after retries. %v", status, err)
		return
	}
	log.NamedDebug(name, logger, "ceph filesystem subvolume group status updated to %q", status)
}

func buildClusterID(cephFilesystemSubVolumeGroup *cephv1.CephFilesystemSubVolumeGroup) string {
	if cephFilesystemSubVolumeGroup.Spec.ClusterID != "" {
		return cephFilesystemSubVolumeGroup.Spec.ClusterID
	}
	clusterID := fmt.Sprintf("%s-%s-file-%s", cephFilesystemSubVolumeGroup.Namespace, cephFilesystemSubVolumeGroup.Spec.FilesystemName, getSubvolumeGroupName(cephFilesystemSubVolumeGroup))
	return k8sutil.Hash(clusterID)
}

func (r *ReconcileCephFilesystemSubVolumeGroup) cleanup(svg *cephv1.CephFilesystemSubVolumeGroup, cephCluster *cephv1.CephCluster) error {
	nsName := opcontroller.NsName(svg.Namespace, svg.Name)
	log.NamedInfo(nsName, logger, "starting cleanup of the ceph resources for subVolumeGroup")
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
