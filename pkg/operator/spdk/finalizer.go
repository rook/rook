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

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	spdkv1alpha1 "github.com/rook/rook/pkg/apis/spdk.rook.io/v1alpha1"
)

const finalizerName = "cluster.spdk.rook.io"

func (r *ClusterReconciler) addFinalizer(cluster *spdkv1alpha1.Cluster) error {
	if contains(cluster.GetFinalizers(), finalizerName) {
		return nil
	}

	r.Log.Info("add finalizer")
	controllerutil.AddFinalizer(cluster, finalizerName)
	return r.Update(context.Background(), cluster)
}

func (r *ClusterReconciler) finalize(cluster *spdkv1alpha1.Cluster) error {
	if !contains(cluster.GetFinalizers(), finalizerName) {
		return nil
	}

	err := cleanupCluster(r, cluster)
	if err != nil {
		return err
	}

	r.Log.Info("remove finalizer")
	controllerutil.RemoveFinalizer(cluster, finalizerName)
	return r.Update(context.Background(), cluster)
}

func cleanupCluster(r *ClusterReconciler, cluster *spdkv1alpha1.Cluster) error {
	for i := range cluster.Spec.SpdkNodes {
		node := &cluster.Spec.SpdkNodes[i]
		err := cleanupNode(r, cluster, node)
		if err != nil {
			return err
		}
	}
	return nil
}

func cleanupNode(r *ClusterReconciler, cluster *spdkv1alpha1.Cluster, node *spdkv1alpha1.SpdkNode) error {
	ctx := context.Background()
	stsName := stsName(node)

	sts := &appsv1.StatefulSet{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      stsName,
	}, sts)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	return r.Delete(ctx, sts)
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
