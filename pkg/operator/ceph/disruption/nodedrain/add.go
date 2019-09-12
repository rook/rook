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
	"reflect"
	"time"

	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
	"github.com/rook/rook/pkg/operator/k8sutil"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	minStoreResyncPeriod = 10 * time.Hour
	controllerName       = "nodedrain-controller"

	// CanaryAppName is applied to nodedrain canary components (pods, deployments) with the key app
	CanaryAppName = "rook-ceph-drain-canary"
	NodeNameLabel = "node_name"
)

// Add adds a new Controller based on nodedrain.ReconcileNode and registers the relevant watches and handlers
func Add(mgr manager.Manager, context *controllerconfig.Context) error {
	reconcileNode := &ReconcileNode{
		client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		context: context,
	}
	reconciler := reconcile.Reconciler(reconcileNode)
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return err
	}

	// Watch for changes to the nodes
	pred := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			nodeOld := e.ObjectOld.DeepCopyObject().(*corev1.Node)
			nodeNew := e.ObjectNew.DeepCopyObject().(*corev1.Node)
			return !reflect.DeepEqual(nodeOld.Spec, nodeNew.Spec)
		},
	}
	err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return err
	}
	// Watch for changes to the node canary deployments
	err = c.Watch(
		&source.Kind{Type: &appsv1.Deployment{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(obj handler.MapObject) []reconcile.Request {
				deployment, ok := obj.Object.(*appsv1.Deployment)
				if !ok {
					return []reconcile.Request{}
				}
				labels := deployment.GetLabels()
				if appName, ok := labels[k8sutil.AppAttr]; !ok || appName != CanaryAppName {
					return []reconcile.Request{}
				}
				nodeName, ok := deployment.Spec.Template.ObjectMeta.Labels[NodeNameLabel]
				if !ok {
					return []reconcile.Request{}
				}
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}}
				return []reconcile.Request{req}
			}),
		},
	)
	if err != nil {
		return err
	}

	// Watch for changes to the osd pod nodename and enqueue thier nodes
	err = c.Watch(
		&source.Kind{Type: &corev1.Pod{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(obj handler.MapObject) []reconcile.Request {
				pod, ok := obj.Object.(*corev1.Pod)
				if !ok {
					return []reconcile.Request{}
				}
				labels := pod.GetLabels()
				if appName, ok := labels[k8sutil.AppAttr]; !ok || appName != osd.AppName {
					return []reconcile.Request{}
				}
				nodeName := pod.Spec.NodeName
				if len(nodeName) < 1 {
					return []reconcile.Request{}
				}
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeName}}
				return []reconcile.Request{req}
			}),
		},
		predicate.Funcs{
			UpdateFunc: func(event event.UpdateEvent) bool {
				oldPod, ok := event.ObjectOld.(*corev1.Pod)
				if !ok {
					return false
				}
				newPod, ok := event.ObjectNew.(*corev1.Pod)
				if !ok {
					return false
				}
				// only enqueue if the nodename has changed
				if oldPod.Spec.NodeName == newPod.Spec.NodeName {
					return false
				}
				return true
			},
		},
	)
	if err != nil {
		return err
	}

	return nil
}
