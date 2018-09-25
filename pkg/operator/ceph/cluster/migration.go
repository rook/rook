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
	"strconv"

	opkit "github.com/rook/operator-kit"
	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookv1alpha1 "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func (c *ClusterController) watchLegacyClusters(namespace string, stopCh chan struct{}, resourceHandlerFuncs cache.ResourceEventHandlerFuncs) {
	// watch for cluster.rook.io/v1alpha1 events if the CRD exists
	if _, err := c.context.RookClientset.RookV1alpha1().Clusters(namespace).List(metav1.ListOptions{}); err != nil {
		logger.Infof("skipping watching for legacy rook cluster events (legacy cluster CRD probably doesn't exist): %+v", err)
	} else {
		logger.Infof("start watching legacy rook clusters in all namespaces")
		watcherLegacy := opkit.NewWatcher(ClusterResourceRookLegacy, namespace, resourceHandlerFuncs, c.context.RookClientset.RookV1alpha1().RESTClient())
		go watcherLegacy.Watch(&rookv1alpha1.Cluster{}, stopCh)
	}
}

func getClusterObject(obj interface{}) (cluster *cephv1beta1.Cluster, migrationNeeded bool, err error) {
	var ok bool
	cluster, ok = obj.(*cephv1beta1.Cluster)
	if ok {
		// the cluster object is of the latest type, simply return it
		return cluster.DeepCopy(), false, nil
	}

	// type assertion to current cluster type failed, try instead asserting to the legacy cluster types
	// then convert it to the current cluster type
	clusterRookLegacy, ok := obj.(*rookv1alpha1.Cluster)
	if ok {
		return convertRookLegacyCluster(clusterRookLegacy.DeepCopy()), true, nil
	}

	return nil, false, fmt.Errorf("not a known cluster object: %+v", obj)
}

func (c *ClusterController) migrateClusterObject(clusterToMigrate *cephv1beta1.Cluster, legacyObj interface{}) error {
	logger.Infof("migrating legacy cluster %s in namespace %s", clusterToMigrate.Name, clusterToMigrate.Namespace)

	_, err := c.context.RookClientset.CephV1beta1().Clusters(clusterToMigrate.Namespace).Get(clusterToMigrate.Name, metav1.GetOptions{})
	if err == nil {
		// cluster of current type with same name/namespace already exists, don't overwrite it
		logger.Warningf("cluster object %s in namespace %s already exists, will not overwrite with migrated legacy cluster.",
			clusterToMigrate.Name, clusterToMigrate.Namespace)
	} else {
		if !errors.IsNotFound(err) {
			return err
		}

		// cluster of current type does not already exist, create it now to complete the migration
		_, err = c.context.RookClientset.CephV1beta1().Clusters(clusterToMigrate.Namespace).Create(clusterToMigrate)
		if err != nil {
			return err
		}

		logger.Infof("completed migration of legacy cluster %s in namespace %s", clusterToMigrate.Name, clusterToMigrate.Namespace)
	}

	// delete the legacy cluster instance, it should not be used anymore now that a migrated instance of the current type exists
	deletePropagation := metav1.DeletePropagationOrphan
	if _, ok := legacyObj.(*rookv1alpha1.Cluster); ok {
		logger.Infof("deleting legacy rook cluster %s in namespace %s", clusterToMigrate.Name, clusterToMigrate.Namespace)
		return c.context.RookClientset.RookV1alpha1().Clusters(clusterToMigrate.Namespace).Delete(
			clusterToMigrate.Name, &metav1.DeleteOptions{PropagationPolicy: &deletePropagation})
	}

	return fmt.Errorf("not a known cluster object: %+v", legacyObj)
}

// ************************************************************************************************
// Rook legacy conversion functions (rook.io/v1alpha1)
// ************************************************************************************************

// converts a legacy cluster.rook.io/v1alpha1 object to the current cluster.ceph.rook.io/v1beta1 object.
// Traverses through the entire object to convert all specs/fields.
func convertRookLegacyCluster(legacyCluster *rookv1alpha1.Cluster) *cephv1beta1.Cluster {
	if legacyCluster == nil {
		return nil
	}

	legacySpec := legacyCluster.Spec

	// default to `3` mons during upgrade when legacy monCount is `0`
	if legacySpec.MonCount <= 0 {
		legacySpec.MonCount = mon.DefaultMonCount
	}

	cluster := &cephv1beta1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      legacyCluster.Name,
			Namespace: legacyCluster.Namespace,
		},
		Spec: cephv1beta1.ClusterSpec{
			Storage:   convertRookLegacyStorageScope(legacySpec.Storage),
			Placement: convertRookLegacyPlacementSpec(legacySpec.Placement),
			Network: rookv1alpha2.NetworkSpec{
				HostNetwork: legacySpec.HostNetwork,
			},
			Resources:       convertRookLegacyResourceSpec(legacySpec.Resources),
			DataDirHostPath: legacySpec.DataDirHostPath,
			Mon: cephv1beta1.MonSpec{
				Count: legacySpec.MonCount,
				// preserve "legacy" behavior to place "multiple mons on one node"
				AllowMultiplePerNode: true,
			},
		},
	}

	return cluster
}

func convertRookLegacyStorageScope(legacyStorageSpec rookv1alpha1.StorageSpec) rookv1alpha2.StorageScopeSpec {
	s := rookv1alpha2.StorageScopeSpec{
		Nodes:       convertRookLegacyStorageNodes(legacyStorageSpec.Nodes),
		UseAllNodes: legacyStorageSpec.UseAllNodes,
		Selection: rookv1alpha2.Selection{
			UseAllDevices: legacyStorageSpec.UseAllDevices,
			DeviceFilter:  legacyStorageSpec.DeviceFilter,
			Devices:       []rookv1alpha2.Device{}, // rookv1alpha1 did not support cluster level devices
			Directories:   convertRookLegacyStorageDirs(legacyStorageSpec.Directories),
		},
		Config:   map[string]string{},
		Location: legacyStorageSpec.Location,
	}

	setRookLegacyStoreConfig(s.Config, legacyStorageSpec.Config.StoreConfig)
	if legacyStorageSpec.MetadataDevice != "" {
		s.Config[config.MetadataDeviceKey] = legacyStorageSpec.MetadataDevice
	}

	return s
}

func convertRookLegacyStorageNodes(legacyNodes []rookv1alpha1.Node) []rookv1alpha2.Node {
	nodes := make([]rookv1alpha2.Node, len(legacyNodes))
	for i, ln := range legacyNodes {
		nodes[i] = rookv1alpha2.Node{
			Name:      ln.Name,
			Resources: ln.Resources,
			Selection: rookv1alpha2.Selection{
				UseAllDevices: ln.UseAllDevices,
				DeviceFilter:  ln.DeviceFilter,
				Devices:       convertRookLegacyStorageDevices(ln.Devices),
				Directories:   convertRookLegacyStorageDirs(ln.Directories),
			},
			Config:   map[string]string{},
			Location: ln.Config.Location,
		}

		setRookLegacyStoreConfig(nodes[i].Config, ln.Config.StoreConfig)

		if ln.MetadataDevice != "" {
			nodes[i].Config[config.MetadataDeviceKey] = ln.MetadataDevice
		}
	}

	return nodes
}

func convertRookLegacyStorageDevices(legacyDevices []rookv1alpha1.Device) []rookv1alpha2.Device {
	devices := make([]rookv1alpha2.Device, len(legacyDevices))
	for i, ld := range legacyDevices {
		devices[i] = rookv1alpha2.Device{
			Name:     ld.Name,
			FullPath: "",
			Config:   map[string]string{}, // there was no concept of per device config in rookv1alpha1
		}
	}

	return devices
}

func convertRookLegacyStorageDirs(legacyDirs []rookv1alpha1.Directory) []rookv1alpha2.Directory {
	dirs := make([]rookv1alpha2.Directory, len(legacyDirs))
	for i, ld := range legacyDirs {
		dirs[i] = rookv1alpha2.Directory{
			Path:   ld.Path,
			Config: map[string]string{}, // there was no concept of per directory config in rookv1alpha1
		}
	}

	return dirs
}

func convertRookLegacyStoreConfig(legacyStoreConfig rookv1alpha1.StoreConfig) config.StoreConfig {
	return config.StoreConfig{
		StoreType:      legacyStoreConfig.StoreType,
		WalSizeMB:      legacyStoreConfig.WalSizeMB,
		DatabaseSizeMB: legacyStoreConfig.DatabaseSizeMB,
		JournalSizeMB:  legacyStoreConfig.JournalSizeMB,
	}
}

func setRookLegacyStoreConfig(configMap map[string]string, legacyStoreConfig rookv1alpha1.StoreConfig) {
	if legacyStoreConfig.StoreType != "" {
		configMap[config.StoreTypeKey] = legacyStoreConfig.StoreType
	}
	if legacyStoreConfig.WalSizeMB != 0 {
		configMap[config.WalSizeMBKey] = strconv.Itoa(legacyStoreConfig.WalSizeMB)
	}
	if legacyStoreConfig.DatabaseSizeMB != 0 {
		configMap[config.DatabaseSizeMBKey] = strconv.Itoa(legacyStoreConfig.DatabaseSizeMB)
	}
	if legacyStoreConfig.JournalSizeMB != 0 {
		configMap[config.JournalSizeMBKey] = strconv.Itoa(legacyStoreConfig.JournalSizeMB)
	}
}

func convertRookLegacyPlacementSpec(legacyPlacementSpec rookv1alpha1.PlacementSpec) rookv1alpha2.PlacementSpec {
	return rookv1alpha2.PlacementSpec{
		rookv1alpha2.PlacementKeyAll: rookv1alpha2.ConvertLegacyPlacement(legacyPlacementSpec.All),
		cephv1beta1.PlacementKeyMgr:  rookv1alpha2.ConvertLegacyPlacement(legacyPlacementSpec.Mgr),
		cephv1beta1.PlacementKeyMon:  rookv1alpha2.ConvertLegacyPlacement(legacyPlacementSpec.Mon),
		cephv1beta1.PlacementKeyOSD:  rookv1alpha2.ConvertLegacyPlacement(legacyPlacementSpec.OSD),
	}
}

func convertRookLegacyResourceSpec(legacyResourceSpec rookv1alpha1.ResourceSpec) rookv1alpha2.ResourceSpec {
	return rookv1alpha2.ResourceSpec{
		cephv1beta1.ResourcesKeyMgr: legacyResourceSpec.Mgr,
		cephv1beta1.ResourcesKeyMon: legacyResourceSpec.Mon,
		cephv1beta1.ResourcesKeyOSD: legacyResourceSpec.OSD,
	}
}
