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

package crash

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/rbd"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/batch/v1"
	"k8s.io/api/batch/v1beta1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/version"

	"github.com/rook/rook/pkg/operator/ceph/file/mds"
	"github.com/rook/rook/pkg/operator/ceph/file/mirror"
	"github.com/rook/rook/pkg/operator/ceph/object"

	"github.com/coreos/pkg/capnslog"

	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/k8sutil"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)
	// Implement reconcile.Reconciler so the controller can reconcile objects
	_ reconcile.Reconciler = &ReconcileNode{}

	// wait for secret "rook-ceph-crash-collector-keyring" to be created
	waitForRequeueIfSecretNotCreated = reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}
)

const (
	MinVersionForCronV1 = "1.21.0"
)

// ReconcileNode reconciles ReplicaSets
type ReconcileNode struct {
	// client can be used to retrieve objects from the APIServer.
	scheme           *runtime.Scheme
	client           client.Client
	context          *clusterd.Context
	opManagerContext context.Context
	opConfig         opcontroller.OperatorConfig
}

// Reconcile reconciles a node and ensures that it has a crashcollector deployment
// attached to it.
// The Controller will requeue the Request to be processed again if an error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNode) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	result, err := r.reconcile(request)
	if err != nil {
		logger.Error(err)
	}
	return result, err
}

func (r *ReconcileNode) reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger.Debugf("reconciling node: %q", request.Name)

	// get the node object
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: request.Name}}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, node)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// if a node is not present, check if there are any crashcollector deployment for that node and delete it.
			err := r.listCrashCollectorAndDelete(request.Name, request.Namespace)
			if err != nil {
				logger.Errorf("failed to list and delete crash collector deployment on node %q; user should delete them manually. %v", request.Name, err)
			}
		} else {
			return reconcile.Result{}, errors.Wrapf(err, "could not get node %q", request.Name)
		}
	}

	// Get the list of all the Ceph pods
	cephPods, err := r.cephPodList()
	if err != nil {
		if len(cephPods) == 0 {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to list all ceph pods")
	}

	namespaceToPodList := make(map[string][]corev1.Pod)
	for _, cephPod := range cephPods {
		podNamespace := cephPod.GetNamespace()
		podList, ok := namespaceToPodList[podNamespace]
		if !ok {
			// initialize list
			namespaceToPodList[podNamespace] = []corev1.Pod{cephPod}
		} else {
			// append cephPod to namespace's pod list
			namespaceToPodList[podNamespace] = append(podList, cephPod)
		}
	}

	for namespace, cephPods := range namespaceToPodList {
		// get dataDirHostPath from the CephCluster
		cephClusters := &cephv1.CephClusterList{}
		err := r.client.List(r.opManagerContext, cephClusters, client.InNamespace(namespace))
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "could not get cephcluster in namespaces %q", namespace)
		}
		if len(cephClusters.Items) < 1 {
			logger.Debugf("no CephCluster found in the namespace %q", namespace)
			return reconcile.Result{}, nil
		}

		cephCluster := cephClusters.Items[0]
		if len(cephClusters.Items) > 1 {
			logger.Errorf("more than one CephCluster found in the namespace %q, choosing the first one %q", namespace, cephCluster.GetName())
		}

		// If the crash controller is disabled in the spec let's do a noop
		if cephCluster.Spec.CrashCollector.Disable {
			deploymentList := &appsv1.DeploymentList{}
			namespaceListOpts := client.InNamespace(request.Namespace)

			// Try to fetch the list of existing deployment and remove them
			err := r.client.List(r.opManagerContext, deploymentList, client.MatchingLabels{k8sutil.AppAttr: AppName}, namespaceListOpts)
			if err != nil {
				logger.Errorf("failed to list crash collector deployments, delete it/them manually. %v", err)
				return reconcile.Result{}, nil
			}

			//  Try to delete all the crash deployments
			for _, d := range deploymentList.Items {
				err := r.deleteCrashCollector(d)
				if err != nil {
					logger.Errorf("failed to delete crash collector deployment %q, delete it manually. %v", d.Name, err)
					continue
				}
				logger.Infof("crash collector deployment %q successfully removed", d.Name)
			}

			return reconcile.Result{}, nil
		}

		// checking if secret "rook-ceph-crash-collector-keyring" is present which is required to create crashcollector pods
		secret := &corev1.Secret{}
		err = r.client.Get(r.opManagerContext, types.NamespacedName{Name: crashCollectorKeyName, Namespace: namespace}, secret)
		if err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debugf("secret %q not found. retrying in %q. %v", crashCollectorKeyName, waitForRequeueIfSecretNotCreated.RequeueAfter.String(), err)
				return waitForRequeueIfSecretNotCreated, nil
			}

			return reconcile.Result{}, errors.Wrapf(err, "failed to list the %q secret.", crashCollectorKeyName)
		}

		clusterImage := cephCluster.Spec.CephVersion.Image
		cephVersion, err := opcontroller.GetImageVersion(cephCluster)
		if err != nil {
			logger.Errorf("ceph version not found for image %q used by cluster %q. %v", clusterImage, cephCluster.Name, err)
			return reconcile.Result{}, nil
		}

		uniqueTolerations := controllerconfig.TolerationSet{}
		hasCephPods := false
		for _, cephPod := range cephPods {
			if cephPod.Spec.NodeName == request.Name {
				hasCephPods = true
				for _, podToleration := range cephPod.Spec.Tolerations {
					// Add toleration to the map
					uniqueTolerations.Add(podToleration)
				}
			}
		}

		// If the node has Ceph pods we create a crash collector
		if hasCephPods {
			tolerations := uniqueTolerations.ToList()
			op, err := r.createOrUpdateCephCrash(*node, tolerations, cephCluster, cephVersion)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "node reconcile failed on op %q", op)
			}
			logger.Debugf("deployment successfully reconciled for node %q. operation: %q", request.Name, op)
			// If there are no Ceph pods, check that there are no crash collector pods in case Ceph pods moved to another node
			// Thus the crash collector must be removed from that node
		} else {
			err := r.listCrashCollectorAndDelete(request.Name, request.Namespace)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "failed to list and delete crash collector deployments on node %q", request.Name)
			}
		}

		if err := r.reconcileCrashRetention(namespace, cephCluster, cephVersion); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileNode) cephPodList() ([]corev1.Pod, error) {
	cephPods := make([]corev1.Pod, 0)
	cephAppNames := []string{mon.AppName, mgr.AppName, osd.AppName, object.AppName, mds.AppName, rbd.AppName, mirror.AppName}

	for _, app := range cephAppNames {
		podList := &corev1.PodList{}
		err := r.client.List(r.opManagerContext, podList, client.MatchingLabels{k8sutil.AppAttr: app})
		if err != nil {
			return cephPods, errors.Wrapf(err, "could not list the %q pods", app)
		}

		cephPods = append(cephPods, podList.Items...)
	}

	return cephPods, nil
}

func (r *ReconcileNode) listCrashCollectorAndDelete(nodeName, ns string) error {
	deploymentList := &appsv1.DeploymentList{}
	namespaceListOpts := client.InNamespace(ns)
	err := r.client.List(r.opManagerContext, deploymentList, client.MatchingLabels{k8sutil.AppAttr: AppName, NodeNameLabel: nodeName}, namespaceListOpts)
	if err != nil {
		return errors.Wrap(err, "failed to list crash collector deployments")
	}
	for _, d := range deploymentList.Items {
		logger.Infof("deleting deployment %q for node %q", d.ObjectMeta.Name, nodeName)
		err := r.deleteCrashCollector(d)
		if err != nil {
			return errors.Wrapf(err, "failed to delete crash collector deployment %q", d.Name)
		}
		logger.Infof("successfully removed crash collector deployment %q from node %q", d.Name, nodeName)
	}

	return nil
}

func (r *ReconcileNode) deleteCrashCollector(deployment appsv1.Deployment) error {
	deploymentName := deployment.ObjectMeta.Name
	namespace := deployment.ObjectMeta.Namespace
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
		},
	}

	err := r.client.Delete(r.opManagerContext, dep)
	if err != nil && !kerrors.IsNotFound(err) {
		return errors.Wrapf(err, "could not delete crash collector deployment %q", deploymentName)
	}

	return nil
}

func (r *ReconcileNode) reconcileCrashRetention(namespace string, cephCluster cephv1.CephCluster, cephVersion *cephver.CephVersion) error {
	k8sVersion, err := k8sutil.GetK8SVersion(r.context.Clientset)
	if err != nil {
		return errors.Wrap(err, "failed to get k8s version")
	}
	useCronJobV1 := k8sVersion.AtLeast(version.MustParseSemantic(MinVersionForCronV1))

	objectMeta := metav1.ObjectMeta{
		Name:      prunerName,
		Namespace: namespace,
	}

	if cephCluster.Spec.CrashCollector.DaysToRetain == 0 {
		logger.Debug("deleting cronjob if it exists...")

		var cronJob client.Object
		// minimum k8s version required for v1 cronJob is 'v1.21.0'. Apply v1 if k8s version is at least 'v1.21.0', else apply v1beta1 cronJob.
		if useCronJobV1 {
			// delete v1beta1 cronJob if it already exists
			err = r.client.Delete(r.opManagerContext, &v1beta1.CronJob{ObjectMeta: objectMeta})
			if err != nil && !kerrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete CronJob v1beta1 %q", prunerName)
			}
			cronJob = &v1.CronJob{ObjectMeta: objectMeta}
		} else {
			cronJob = &v1beta1.CronJob{ObjectMeta: objectMeta}
		}

		err := r.client.Delete(r.opManagerContext, cronJob)
		if err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debug("cronJob resource not found. Ignoring since object must be deleted.")
			} else {
				return err
			}
		} else {
			logger.Debug("successfully deleted crash pruner cronjob.")
		}
	} else {
		logger.Debugf("daysToRetain set to: %d", cephCluster.Spec.CrashCollector.DaysToRetain)
		op, err := r.createOrUpdateCephCron(cephCluster, cephVersion, useCronJobV1)
		if err != nil {
			return errors.Wrapf(err, "node reconcile failed on op %q", op)
		}
		logger.Debugf("cronjob successfully reconciled. operation: %q", op)
	}
	return nil
}
