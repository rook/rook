/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package spdk

import (
	"context"

	"github.com/coreos/pkg/capnslog"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	spdkv1alpha1 "github.com/rook/rook/pkg/apis/spdk.rook.io/v1alpha1"
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client
	Log    *capnslog.PackageLogger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=spdk.rook.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=spdk.rook.io,resources=clusters/status,verbs=get;update;patch

func (r *ClusterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	r.Log.Infof("reconcile spdk cluster %s", req.NamespacedName)

	// fetch cluster instance
	cluster := &spdkv1alpha1.Cluster{}
	err := r.Client.Get(ctx, req.NamespacedName, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Infof("cluster %s already deleted", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		r.Log.Error(err)
		return ctrl.Result{}, err
	}
	r.Log.Debugf("%+v", cluster.Spec)

	// handle deletion
	if cluster.GetDeletionTimestamp() != nil {
		r.Log.Infof("delete spdk cluster %s/%s", cluster.Namespace, cluster.Name)
		return ctrl.Result{}, r.finalize(cluster)
	} else {
		err = r.addFinalizer(cluster)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// do the job
	err = nil
	switch cluster.Status.Status {
	case "", spdkv1alpha1.SpdkStatusDeploySpdk:
		err = r.deploySpdk(cluster)
	case spdkv1alpha1.SpdkStatusDeployCsi:
		err = r.deployCsi(cluster)
	case spdkv1alpha1.SpdkStatusRunning:
		err = r.updateCluster(cluster)
	case spdkv1alpha1.SpdkStatusError:
		r.Log.Info("skip reconciliation due to error state")
		return ctrl.Result{}, nil
	}

	// update status
	errStatus := r.Status().Update(ctx, cluster)
	if err == nil {
		err = errStatus
	}
	requeue := cluster.Status.Status != spdkv1alpha1.SpdkStatusRunning
	return ctrl.Result{Requeue: requeue}, err
}

func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&spdkv1alpha1.Cluster{}).
		Complete(r)
}
