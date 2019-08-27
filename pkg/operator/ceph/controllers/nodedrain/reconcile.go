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

package nodedrain

import (
	"context"
	"fmt"

	"github.com/coreos/pkg/capnslog"

	"github.com/rook/rook/pkg/operator/ceph/controllers/controllerconfig"
	"github.com/rook/rook/pkg/operator/k8sutil"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "nodedrain-controller")
	// Implement reconcile.Reconciler so the controller can reconcile objects
	_ reconcile.Reconciler = &ReconcileNode{}
)

const (
	nodeHostNameKey = "kubernetes.io/hostname"
)

// ReconcileNode reconciles ReplicaSets
type ReconcileNode struct {
	// client can be used to retrieve objects from the APIServer.
	scheme  *runtime.Scheme
	client  client.Client
	options *controllerconfig.Options
}

// Reconcile reconciles a node and ensures that it has a drain-detection deployment
// attached to it.
// The Controller will requeue the Request to be processed again if an error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNode) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime loggin interface
	result, err := r.reconcile(request)
	if err != nil {
		logger.Error(err)
	}
	return result, err
}

func (r *ReconcileNode) reconcile(request reconcile.Request) (reconcile.Result, error) {

	logger.Debugf("reconciling node: %s", request.Name)

	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: request.Name}}

	err := r.client.Get(context.TODO(), request.NamespacedName, node)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("Could not get node %s", request.NamespacedName)
	}

	nodeHostnameLabel, ok := node.ObjectMeta.Labels[nodeHostNameKey]
	if !ok {
		return reconcile.Result{}, fmt.Errorf("Label key %s does not exist on node %s", nodeHostNameKey, request.NamespacedName)
	}

	// Create or Update the deployment default/foo
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("rook-ceph-canary-%s", request.Name),
			Namespace: r.options.OperatorNamespace,
		},
	}

	// CreateOrUpdate the deployment
	mutateFunc := func() error {

		// lablels for the pod, the deployment, and the deploymentSelector
		deploymentLabels := map[string]string{
			nodeHostNameKey: nodeHostnameLabel,
			k8sutil.AppAttr: CanaryAppName,
		}

		nodeSelector := map[string]string{nodeHostNameKey: nodeHostnameLabel}

		// Deployment selector is immutable so we set this value only if
		// a new object is going to be created
		if deploy.ObjectMeta.CreationTimestamp.IsZero() {
			deploy.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: deploymentLabels,
			}
		}

		// update
		deploy.ObjectMeta.Labels = deploymentLabels

		// update the Deployment pod template
		deploy.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: deploymentLabels},
			Spec: corev1.PodSpec{
				NodeSelector: nodeSelector,
				Containers:   newDoNothingContainers(),
			},
		}
		controllerutil.SetControllerReference(node, deploy, r.scheme)

		return nil
	}
	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.client, deploy, mutateFunc)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("Node reconcile failed on op: %s : %+v", op, err)
	}
	logger.Debugf("Deployment successfully reconciled. operation: %s", op)
	return reconcile.Result{}, nil
}

// returns a container that does nothing
func newDoNothingContainers() []corev1.Container {
	return []corev1.Container{{
		Image:   "busybox",
		Name:    "busybox",
		Command: []string{"bin/sh"},
		Args:    []string{"-c", "sleep infinity"},
	}}
}
