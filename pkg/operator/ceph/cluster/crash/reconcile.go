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
	"fmt"

	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/rbd"
	"github.com/rook/rook/pkg/operator/ceph/file/mds"
	"github.com/rook/rook/pkg/operator/ceph/object"

	"github.com/coreos/pkg/capnslog"

	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/k8sutil"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)
	// Implement reconcile.Reconciler so the controller can reconcile objects
	_ reconcile.Reconciler = &ReconcileNode{}
)

// ReconcileNode reconciles ReplicaSets
type ReconcileNode struct {
	// client can be used to retrieve objects from the APIServer.
	scheme *runtime.Scheme
	client client.Client
}

// Reconcile reconciles a node and ensures that it has a crashcollector deployment
// attached to it.
// The Controller will requeue the Request to be processed again if an error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNode) Reconcile(request reconcile.Request) (reconcile.Result, error) {
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
	err := r.client.Get(context.TODO(), request.NamespacedName, node)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("could not get node %q", request.NamespacedName)
	}

	// Get the list of all the Ceph pods
	cephPods, err := r.cephPodList()
	if err != nil {
		if len(cephPods) == 0 {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to list all ceph pods. %+v", err)
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
		err := r.client.List(context.TODO(), cephClusters, client.InNamespace(namespace))
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("could not get cephcluster in namespaces %q. %+v", namespace, err)
		}
		if len(cephClusters.Items) < 1 {
			logger.Debugf("no CephCluster found in the namespace %q", namespace)
			return reconcile.Result{}, nil
		}

		cephCluster := cephClusters.Items[0]
		if len(cephClusters.Items) > 1 {
			logger.Errorf("more than one CephCluster found in the namespace %q, choosing the first one %q", namespace, cephCluster.GetName())
		}

		uniqueTolerations := controllerconfig.TolerationSet{}
		hasCephPods := false
		for _, cephPod := range cephPods {
			if cephPod.Spec.NodeName == request.Name {
				logger.Debugf("cephPod.Spec.NodeName is %q and request.Name is %q", cephPod.Spec.NodeName, request.Name)
				hasCephPods = true
				for _, podToleration := range cephPod.Spec.Tolerations {
					// Add toleration to the map
					uniqueTolerations.Add(podToleration)
				}
			}
		}

		if hasCephPods {
			tolerations := uniqueTolerations.ToList()
			op, err := r.createOrUpdateCephCrash(*node, tolerations, cephCluster)
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("node reconcile failed on op %q. %+v", op, err)
			}
			logger.Debugf("deployment successfully reconciled for node %q. operation: %q", request.Name, op)
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileNode) cephPodList() ([]corev1.Pod, error) {
	cephPods := make([]corev1.Pod, 0)
	cephAppNames := []string{mon.AppName, mgr.AppName, osd.AppName, object.AppName, mds.AppName, rbd.AppName}

	for _, app := range cephAppNames {
		podList := &corev1.PodList{}
		err := r.client.List(context.TODO(), podList, client.MatchingLabels{k8sutil.AppAttr: app})
		if err != nil {
			return cephPods, fmt.Errorf("could not list the %q pods. %+v", app, err)
		}

		cephPods = append(cephPods, podList.Items...)
	}

	return cephPods, nil
}
