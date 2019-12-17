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
	"os"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"

	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
	"github.com/rook/rook/pkg/operator/k8sutil"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

// ReconcileNode reconciles ReplicaSets
type ReconcileNode struct {
	// client can be used to retrieve objects from the APIServer.
	scheme  *runtime.Scheme
	client  client.Client
	context *controllerconfig.Context
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

	if !r.context.ReconcileCanaries.Get() {
		return reconcile.Result{}, nil
	}

	logger.Debugf("reconciling node: %s", request.Name)

	// Create or Update the deployment default/foo
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sutil.TruncateNodeName(fmt.Sprintf("%s-%%s", CanaryAppName), request.Name),
			Namespace: r.context.OperatorNamespace,
		},
	}

	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: request.Name}}

	err := r.client.Get(context.TODO(), request.NamespacedName, node)
	if kerrors.IsNotFound(err) {
		// delete any canary deployments if the node doesn't exist
		r.client.Delete(context.TODO(), deploy)
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, errors.Errorf("could not get node %q", request.NamespacedName)
	}

	nodeHostnameLabel, ok := node.ObjectMeta.Labels[corev1.LabelHostname]
	if !ok {
		return reconcile.Result{}, errors.Errorf("Label key %s does not exist on node %s", corev1.LabelHostname, request.NamespacedName)
	}

	osdPodList := &corev1.PodList{}
	err = r.client.List(context.TODO(), osdPodList, client.MatchingLabels{k8sutil.AppAttr: osd.AppName})
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "could not list the osd pods")
	}

	// map with tolerations as keys and empty struct as values for uniqueness
	uniqueTolerations := controllerconfig.TolerationSet{}

	occupiedByOSD := false
	for _, osdPod := range osdPodList.Items {
		if osdPod.Spec.NodeName == request.Name {
			labels := osdPod.GetLabels()
			deploymentList := &appsv1.DeploymentList{}
			labelSelector := map[string]string{
				osd.OsdIdLabelKey: labels[osd.OsdIdLabelKey],
				k8sutil.AppAttr:   osd.AppName,
			}
			var deployment appsv1.Deployment
			err := r.client.List(context.TODO(), deploymentList, client.MatchingLabels(labelSelector), client.InNamespace(osdPod.GetNamespace()))
			if err != nil || len(deploymentList.Items) < 1 {
				logger.Errorf("cannot find deployment for osd id %q in namespace %q", labels[osd.OsdIdLabelKey], osdPod.GetNamespace())
			} else {
				deployment = deploymentList.Items[0]
			}
			if len(deploymentList.Items) > 1 {
				logger.Errorf("found multiple deployments for osd id %q in namespace %q: %+v", labels[osd.OsdIdLabelKey], osdPod.GetNamespace(), deploymentList)
			}

			occupiedByOSD = true
			// get the osd tolerations
			for _, osdToleration := range deployment.Spec.Template.Spec.Tolerations {
				if osdToleration.Key == "node.kubernetes.io/unschedulable" {
					logger.Errorf(
						"osd %q in namespace %q tolerates the drain taint, but the drain canary will not.",
						labels[osd.OsdIdLabelKey],
						labels[k8sutil.ClusterAttr],
					)
				} else {
					uniqueTolerations.Add(osdToleration)
				}
			}
		}
	}

	// CreateOrUpdate the deployment
	mutateFunc := func() error {

		// lablels for the pod, the deployment, and the deploymentSelector
		selectorLabels := map[string]string{
			corev1.LabelHostname: nodeHostnameLabel,
			k8sutil.AppAttr:      CanaryAppName,
			NodeNameLabel:        node.GetName(),
		}

		nodeSelector := map[string]string{corev1.LabelHostname: nodeHostnameLabel}

		// Deployment selector is immutable so we set this value only if
		// a new object is going to be created
		if deploy.ObjectMeta.CreationTimestamp.IsZero() {
			deploy.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			}
		}

		ownerReferences := []metav1.OwnerReference{}
		// get the operator Deployment to use as an owner reference
		operatorPodKey := types.NamespacedName{Name: os.Getenv(k8sutil.PodNameEnvVar), Namespace: r.context.OperatorNamespace}
		operatorDeployment, err := getDeploymentForPod(r.client, operatorPodKey)
		if err != nil {
			logger.Errorf("could not find rook operator deployment for pod %+v. %v", operatorPodKey, err)
		} else {
			operatorDeployment := operatorDeployment
			controllerBool := true
			operatorOwnerRef := metav1.OwnerReference{
				APIVersion: operatorDeployment.APIVersion,
				Kind:       operatorDeployment.Kind,
				UID:        operatorDeployment.GetUID(),
				Name:       operatorDeployment.GetName(),
				Controller: &controllerBool,
			}
			ownerReferences = append(ownerReferences, operatorOwnerRef)
		}
		deploy.ObjectMeta.OwnerReferences = ownerReferences

		// update the deployment labels
		topology, _ := osd.ExtractRookTopologyFromLabels(node.GetLabels())
		for key, value := range topology {
			selectorLabels[key] = value
		}
		deploy.ObjectMeta.Labels = selectorLabels

		// update the Deployment pod template
		deploy.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: selectorLabels},
			Spec: corev1.PodSpec{
				NodeSelector: nodeSelector,
				Containers:   newDoNothingContainers(r.context.RookImage),
				Tolerations:  uniqueTolerations.ToList(),
			},
		}

		return nil
	}
	if occupiedByOSD {
		op, err := controllerutil.CreateOrUpdate(context.TODO(), r.client, deploy, mutateFunc)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "node reconcile failed on op %s", op)
		}
		logger.Debugf("deployment successfully reconciled for node %s. operation: %s", request.Name, op)
	} else {
		logger.Debugf("not watching for drains on node %s as there are no osds running there.", request.Name)

		// get the canary deployment
		canarayDeployment := &appsv1.Deployment{}
		key := types.NamespacedName{Name: deploy.GetName(), Namespace: deploy.GetNamespace()}
		err := r.client.Get(context.TODO(), key, canarayDeployment)
		if err != nil && !kerrors.IsNotFound(err) {
			return reconcile.Result{}, errors.Wrapf(err, "could not fetch deployment %q", key)
		} else if kerrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}

		// delete the canary deployments that aren't triggered by drains, but are on nodes that aren't occupied by OSDs
		// if it's on a draining node, we don't want to kill the canary.
		if canarayDeployment.Status.ReadyReplicas > 0 {
			err := r.client.Delete(context.TODO(), canarayDeployment)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "could not delete deployment %q", key)
			}
		}
	}
	return reconcile.Result{}, nil
}

// returns a container that does nothing
func newDoNothingContainers(rookImage string) []corev1.Container {
	return []corev1.Container{{
		Image:   rookImage,
		Name:    "sleep",
		Command: []string{"/tini"},
		Args:    []string{"--", "sleep", "infinity"},
	}}
}

func getDeploymentForPod(client client.Client, podKey types.NamespacedName) (*appsv1.Deployment, error) {
	// get pod
	operatorPod := &corev1.Pod{}
	err := client.Get(context.TODO(), podKey, operatorPod)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get pod %+v", podKey)
	} else {
		for _, rsRef := range operatorPod.ObjectMeta.OwnerReferences {
			if *rsRef.Controller {
				// get rs
				replicaSet := &appsv1.ReplicaSet{}
				replicaSetKey := types.NamespacedName{Name: rsRef.Name, Namespace: podKey.Namespace}
				err := client.Get(context.TODO(), replicaSetKey, replicaSet)
				if err != nil {
					return nil, errors.Wrapf(err, "could not get replicaset %+v", replicaSetKey)
				} else {
					for _, depRef := range replicaSet.ObjectMeta.OwnerReferences {
						if *depRef.Controller {
							// get deployment
							operatorDeployment := &appsv1.Deployment{}
							operatorDeploymentKey := types.NamespacedName{Name: depRef.Name, Namespace: podKey.Namespace}
							err := client.Get(context.TODO(), operatorDeploymentKey, operatorDeployment)
							return operatorDeployment, err
						}
					}
				}
			}
		}
	}
	return nil, errors.Errorf("could not get deployment for pod %+v", podKey)
}
