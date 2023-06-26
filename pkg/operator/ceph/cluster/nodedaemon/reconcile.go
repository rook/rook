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

package nodedaemon

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/rbd"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	appsv1 "k8s.io/api/apps/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

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

// Reconcile reconciles a node and ensures that it has necessary node-monitoring daemons.
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

func (r *ReconcileNode) cleanupExporterResources(clusterNamespace string, ns string, nodeName string) (reconcile.Result, error) {
	err := k8sutil.DeleteServiceMonitor(r.context, r.opManagerContext, ns, cephExporterAppName)
	if err != nil {
		logger.Debugf("failed to delete service monitor for ceph exporter in namespace %q on node %q. %v", ns, nodeName, err)
	}
	err = k8sutil.DeleteService(r.opManagerContext, r.context.Clientset, r.opConfig.OperatorNamespace, cephExporterAppName)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to delete ceph exporter metrics service in namespace %q on node %q", ns, nodeName)
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileNode) reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger.Debugf("reconciling node: %q", request.Name)

	// get the node object
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: request.Name}}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, node)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// if a node is not present, check if there are any node daemons to remove
			err := r.listNodeDaemonsAndDelete(request.Name, "")
			if err != nil {
				logger.Errorf("failed to list and delete deployment on node %q; user should delete them manually. %v", request.Name, err)
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

	// Get all the namespaces where the Ceph daemons are running
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

	// For each cephcluster, reconcile the node daemons
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

		allDisabled := r.removeDisabledCrashCollectorDaemons(cephCluster.Spec, namespace) && r.removeDisabledCephExporterDaemons(cephCluster.Spec, namespace)
		if allDisabled {
			return reconcile.Result{}, nil
		}

		// checking if secret "rook-ceph-crash-collector-keyring" is present which is required to create crashcollector pods
		// this is also an indicator that other daemons can be started
		secret := &corev1.Secret{}
		err = r.client.Get(r.opManagerContext, types.NamespacedName{Name: crashCollectorKeyName, Namespace: namespace}, secret)
		if err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debugf("secret %q in namespace %q not found. retrying in %q. %v", crashCollectorKeyName, namespace, waitForRequeueIfSecretNotCreated.RequeueAfter.String(), err)
				return waitForRequeueIfSecretNotCreated, nil
			}

			return reconcile.Result{}, errors.Wrapf(err, "failed to list the secret %q in namespace %q.", crashCollectorKeyName, namespace)
		}

		clusterImage := cephCluster.Spec.CephVersion.Image
		cephVersion, err := opcontroller.GetImageVersion(cephCluster)
		if err != nil {
			logger.Errorf("ceph version not found for image %q used by cluster %q in namespace %q. %v", clusterImage, cephCluster.Name, cephCluster.Namespace, err)
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

		// If the node has Ceph pods we create the daemons
		if hasCephPods {
			tolerations := uniqueTolerations.ToList()
			err := r.createOrUpdateNodeDaemons(*node, tolerations, cephCluster, cephVersion)
			if err != nil {
				return reconcile.Result{}, errors.Wrap(err, "node reconcile failed")
			}
		} else {
			// If there are no Ceph pods, check that there are no crash collector or ceph-exporter pods in case Ceph pods moved to another node
			// Thus the crash collector and ceph-exporter must be removed from that node
			err := r.listNodeDaemonsAndDelete(request.Name, namespace)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "failed to list and delete deployments in namespace %q on node %q", namespace, request.Name)
			}
			result, err := r.cleanupExporterResources(cephCluster.Namespace, namespace, request.Name)
			if err != nil {
				return result, errors.Wrapf(err, "failed to cleanup exporter resources in namespace %q on node %q", namespace, request.Name)
			}
		}
		// Cleanup exporter if the ceph version isn't supported
		if !cephVersion.IsAtLeast(MinVersionForCephExporter) {
			for _, cephPod := range cephPods {
				if cephPod.Spec.NodeName == request.Name {
					if err := r.listDeploymentAndDelete(cephExporterAppName, request.Name, namespace); err != nil {
						return reconcile.Result{}, errors.Wrap(err, "failed to delete ceph-exporter")
					}
					result, err := r.cleanupExporterResources(cephCluster.Namespace, namespace, request.Name)
					if err != nil {
						return result, errors.Wrapf(err, "failed to cleanup exporter resources in namespace %q on node %q", namespace, request.Name)
					}
				}
			}
		}

		if err := r.reconcileCrashPruner(namespace, cephCluster, cephVersion); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileNode) createOrUpdateNodeDaemons(node corev1.Node, tolerations []corev1.Toleration, cephCluster cephv1.CephCluster, cephVersion *cephver.CephVersion) error {
	if !cephCluster.Spec.CrashCollector.Disable {
		op, err := r.createOrUpdateCephCrash(node, tolerations, cephCluster, cephVersion)
		if err != nil {
			if op == "unchanged" {
				logger.Debugf("crash collector unchanged on node %q", node.Name)
			} else {
				return errors.Wrapf(err, "crash collector reconcile failed on op %q", op)
			}
		} else {
			logger.Debugf("crash collector successfully reconciled for node %q. operation: %q", node.Name, op)
		}
	}
	if cephVersion.IsAtLeast(MinVersionForCephExporter) && !cephCluster.Spec.Monitoring.MetricsDisabled {
		op, err := r.createOrUpdateCephExporter(node, tolerations, cephCluster, cephVersion)
		if err != nil {
			if op == "unchanged" {
				logger.Debugf("ceph exporter unchanged on node %q", node.Name)
			} else {
				return errors.Wrapf(err, "ceph exporter reconcile failed on op %q", op)
			}
		} else {
			// CephVersion change is done temporarily, as some regression was detected in Ceph version 17.2.6 which is summarised here https://github.com/ceph/ceph/pull/50718#issuecomment-1505608312.
			// Thus, disabling ceph-exporter for now until all the regression are fixed.
			if cephVersion.IsAtLeast(MinVersionForCephExporter) {
				logger.Debugf("ceph exporter successfully reconciled for node %q. operation: %q", node.Name, op)
				// create the metrics service
				service, err := MakeCephExporterMetricsService(cephCluster, exporterServiceMetricName, r.scheme)
				if err != nil {
					return err
				}
				if _, err := k8sutil.CreateOrUpdateService(r.opManagerContext, r.context.Clientset, cephCluster.Namespace, service); err != nil {
					return errors.Wrap(err, "failed to create ceph-exporter metrics service")
				}

				if cephCluster.Spec.Monitoring.Enabled {
					if err := EnableCephExporterServiceMonitor(r.context, cephCluster, r.scheme, r.opManagerContext); err != nil {
						return errors.Wrap(err, "failed to enable service monitor")
					}
					logger.Debug("service monitor for ceph exporter was enabled successfully")
				}
			}

		}
	}

	return nil
}

func (r *ReconcileNode) removeDisabledCrashCollectorDaemons(spec cephv1.ClusterSpec, namespace string) bool {
	// If the crash daemons are disabled in the spec let's remove them
	if spec.CrashCollector.Disable {
		r.deleteNodeDaemon(crashCollectorAppName, namespace)
	}

	return spec.CrashCollector.Disable
}

func (r *ReconcileNode) removeDisabledCephExporterDaemons(spec cephv1.ClusterSpec, namespace string) bool {
	// If the ceph-exporter daemons are disabled in the spec let's remove them
	if spec.Monitoring.MetricsDisabled {
		r.deleteNodeDaemon(cephExporterAppName, namespace)
	}

	return spec.Monitoring.MetricsDisabled
}

func (r *ReconcileNode) listDeploymentAndDelete(appName, nodeName, ns string) error {
	deploymentList := &appsv1.DeploymentList{}
	namespaceListOpts := client.InNamespace(ns)
	err := r.client.List(r.opManagerContext, deploymentList, client.MatchingLabels{k8sutil.AppAttr: appName, NodeNameLabel: nodeName}, namespaceListOpts)
	if err != nil {
		return errors.Wrapf(err, "failed to list deployments in namespace %q", ns)
	}
	for _, d := range deploymentList.Items {
		logger.Infof("deleting deployment %q for node %q", d.ObjectMeta.Name, nodeName)
		err := r.deleteDeployment(d)
		if err != nil {
			return errors.Wrapf(err, "failed to delete deployment %q in namespace %q", d.Name, d.Namespace)
		}
		logger.Infof("successfully removed deployment %q in namespace %q from node %q", d.Name, d.Namespace, nodeName)
	}

	return nil
}

func (r *ReconcileNode) deleteNodeDaemon(appName, namespace string) {
	deploymentList := &appsv1.DeploymentList{}
	namespaceListOpts := client.InNamespace(namespace)

	// Try to fetch the list of existing deployment and remove them
	err := r.client.List(r.opManagerContext, deploymentList, client.MatchingLabels{k8sutil.AppAttr: appName}, namespaceListOpts)
	if err != nil {
		logger.Errorf("failed to list deployments in namespace %q, delete it/them manually. %v", namespace, err)
		return
	}

	//  Try to delete all the node daemons
	for _, d := range deploymentList.Items {
		err := r.deleteDeployment(d)
		if err != nil {
			logger.Errorf("failed to delete deployment %q in namespace %q, delete it manually. %v", d.Name, d.Namespace, err)
			continue
		}
		logger.Infof("Deployments %q in namespace %q successfully removed", d.Name, d.Namespace)
	}
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

func (r *ReconcileNode) listNodeDaemonsAndDelete(nodeName, ns string) error {
	// delete the crash daemons on the given node
	if err := r.listDeploymentAndDelete(crashCollectorAppName, nodeName, ns); err != nil {
		return errors.Wrap(err, "failed to delete crash collector")
	}

	// delete the ceph-exporter daemons on the given node
	if err := r.listDeploymentAndDelete(cephExporterAppName, nodeName, ns); err != nil {
		return errors.Wrap(err, "failed to delete ceph-exporter")
	}

	return nil
}

func (r *ReconcileNode) deleteDeployment(deployment appsv1.Deployment) error {
	// delete a specific deployment)
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
		return errors.Wrapf(err, "could not delete deployment %q in namespace %q", deploymentName, namespace)
	}

	return nil
}
