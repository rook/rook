/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package cluster to manage a Ceph cluster.
package cluster

import (
	"fmt"

	opkit "github.com/rook/operator-kit"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func (c *ClusterController) watchLegacyClusters(namespace string, stopCh chan struct{}, resourceHandlerFuncs cache.ResourceEventHandlerFuncs) {
	// watch for cluster.rook.io/v1beta1 events if the CRD exists
	if _, err := c.context.RookClientset.CephV1beta1().Clusters(namespace).List(metav1.ListOptions{}); err != nil {
		logger.Infof("skipping watching for legacy rook cluster events (legacy cluster CRD probably doesn't exist): %+v", err)
	} else {
		logger.Infof("start watching legacy rook clusters in all namespaces")
		watcherLegacy := opkit.NewWatcher(ClusterResourceRookLegacy, namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1beta1().RESTClient())
		go watcherLegacy.Watch(&cephv1beta1.Cluster{}, stopCh)
	}
}

func getClusterObject(obj interface{}) (cluster *cephv1.CephCluster, migrationNeeded bool, err error) {
	var ok bool
	cluster, ok = obj.(*cephv1.CephCluster)
	if ok {
		// the cluster object is of the latest type, simply return it
		cluster = cluster.DeepCopy()
		setClusterDefaults(cluster)
		return cluster, false, nil
	}

	// type assertion to current cluster type failed, try instead asserting to the legacy cluster types
	// then convert it to the current cluster type
	clusterRookLegacy, ok := obj.(*cephv1beta1.Cluster)
	if ok {
		return convertRookLegacyCluster(clusterRookLegacy.DeepCopy()), true, nil
	}

	return nil, false, fmt.Errorf("not a known cluster object: %+v", obj)
}

func setClusterDefaults(cluster *cephv1.CephCluster) {
	// The ceph version image should be set in the CRD.
	// If/when the v1beta1 CRD is converted to v1, we could set this permanently during the conversion instead of
	// setting this default in memory every time we run the operator.
	if cluster.Spec.CephVersion.Image == "" {
		logger.Infof("setting default luminous image: %s", cephv1.DefaultLuminousImage)
		cluster.Spec.CephVersion.Image = cephv1.DefaultLuminousImage
	}
}

func (c *ClusterController) migrateClusterObject(clusterToMigrate *cephv1.CephCluster, legacyObj interface{}) error {
	logger.Infof("migrating legacy cluster %s in namespace %s", clusterToMigrate.Name, clusterToMigrate.Namespace)

	_, err := c.context.RookClientset.CephV1().CephClusters(clusterToMigrate.Namespace).Get(clusterToMigrate.Name, metav1.GetOptions{})
	if err == nil {
		// cluster of current type with same name/namespace already exists, don't overwrite it
		logger.Warningf("cluster object %s in namespace %s already exists, will not overwrite with migrated legacy cluster.",
			clusterToMigrate.Name, clusterToMigrate.Namespace)
	} else {
		if !errors.IsNotFound(err) {
			return err
		}

		// cluster of current type does not already exist, create it now to complete the migration
		_, err = c.context.RookClientset.CephV1().CephClusters(clusterToMigrate.Namespace).Create(clusterToMigrate)
		if err != nil {
			return err
		}

		logger.Infof("completed migration of legacy cluster %s in namespace %s", clusterToMigrate.Name, clusterToMigrate.Namespace)
	}

	// delete the legacy cluster instance, it should not be used anymore now that a migrated instance of the current type exists
	deletePropagation := metav1.DeletePropagationOrphan
	if _, ok := legacyObj.(*cephv1beta1.Cluster); ok {
		logger.Infof("deleting legacy rook cluster %s in namespace %s", clusterToMigrate.Name, clusterToMigrate.Namespace)
		return c.context.RookClientset.CephV1beta1().Clusters(clusterToMigrate.Namespace).Delete(
			clusterToMigrate.Name, &metav1.DeleteOptions{PropagationPolicy: &deletePropagation})
	}

	return fmt.Errorf("not a known cluster object: %+v", legacyObj)
}

// ************************************************************************************************
// Rook legacy conversion functions (rook.io/v1alpha1)
// ************************************************************************************************

// converts a legacy ceph.cluster.rook.io/v1beta1 object to the current cephcluster.ceph.rook.io/v1 object.
// Traverses through the entire object to convert all specs/fields.
func convertRookLegacyCluster(legacyCluster *cephv1beta1.Cluster) *cephv1.CephCluster {
	if legacyCluster == nil {
		return nil
	}

	legacySpec := legacyCluster.Spec

	cluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      legacyCluster.Name,
			Namespace: legacyCluster.Namespace,
		},
		Spec: cephv1.ClusterSpec{
			Storage:   legacySpec.Storage,
			Placement: legacySpec.Placement,
			Network: rookv1alpha2.NetworkSpec{
				HostNetwork: legacySpec.Network.HostNetwork,
			},
			Resources:       legacySpec.Resources,
			DataDirHostPath: legacySpec.DataDirHostPath,
			Mon: cephv1.MonSpec{
				Count:                legacySpec.Mon.Count,
				AllowMultiplePerNode: legacySpec.Mon.AllowMultiplePerNode,
			},
		},
	}
	setClusterDefaults(cluster)

	return cluster
}
