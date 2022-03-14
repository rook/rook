/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package installer

import (
	"strings"

	"github.com/rook/rook/tests/framework/utils"
)

const (
	// The version from which the upgrade test will start
	Version1_8 = "v1.8.6"
)

// CephManifestsPreviousVersion wraps rook yaml definitions
type CephManifestsPreviousVersion struct {
	settings *TestCephSettings
	latest   *CephManifestsMaster
}

func (m *CephManifestsPreviousVersion) Settings() *TestCephSettings {
	return m.settings
}

func (m *CephManifestsPreviousVersion) GetCRDs(k8shelper *utils.K8sHelper) string {
	return m.settings.readManifestFromGitHub("crds.yaml")
}

// GetRookOperator returns rook Operator manifest
func (m *CephManifestsPreviousVersion) GetOperator() string {
	var manifest string
	if utils.IsPlatformOpenShift() {
		manifest = m.settings.readManifestFromGitHub("operator-openshift.yaml")
	} else {
		manifest = m.settings.readManifestFromGitHub("operator.yaml")
	}
	return m.settings.replaceOperatorSettings(manifest)
}

// GetCommon returns rook-cluster manifest
func (m *CephManifestsPreviousVersion) GetCommon() string {
	return m.settings.readManifestFromGitHub("common.yaml")
}

// GetRookToolBox returns rook-toolbox manifest
func (m *CephManifestsPreviousVersion) GetToolbox() string {
	if m.settings.DirectMountToolbox {
		manifest := strings.ReplaceAll(m.settings.readManifestFromGitHub("direct-mount.yaml"), "name: rook-direct-mount", "name: rook-ceph-tools")
		return strings.ReplaceAll(manifest, "app: rook-direct-mount", "app: rook-ceph-tools")
	}
	return m.settings.readManifestFromGitHub("toolbox.yaml")
}

func (m *CephManifestsPreviousVersion) GetCommonExternal() string {
	return m.settings.readManifestFromGitHub("common-external.yaml")
}

//**********************************************************************************
//**********************************************************************************
// Methods in this section may need to be customized depending on new
// features that are being added in newer releases and may not be possible
// to configure in the previous release. By default, all these methods will
// provide a thin wrapper around the resources created by the master
// implementation.
//**********************************************************************************
//**********************************************************************************

// GetRookCluster returns rook-cluster manifest
func (m *CephManifestsPreviousVersion) GetCephCluster() string {
	return m.latest.GetCephCluster()
}

func (m *CephManifestsPreviousVersion) GetBlockSnapshotClass(snapshotClassName, reclaimPolicy string) string {
	return m.latest.GetBlockSnapshotClass(snapshotClassName, reclaimPolicy)
}

func (m *CephManifestsPreviousVersion) GetFileStorageSnapshotClass(snapshotClassName, reclaimPolicy string) string {
	return m.latest.GetFileStorageSnapshotClass(snapshotClassName, reclaimPolicy)
}

func (m *CephManifestsPreviousVersion) GetBlockPool(poolName, replicaSize string) string {
	return m.latest.GetBlockPool(poolName, replicaSize)
}

func (m *CephManifestsPreviousVersion) GetBlockStorageClass(poolName, storageClassName, reclaimPolicy string) string {
	return m.latest.GetBlockStorageClass(poolName, storageClassName, reclaimPolicy)
}

func (m *CephManifestsPreviousVersion) GetFileStorageClass(fsName, storageClassName string) string {
	return m.latest.GetFileStorageClass(fsName, storageClassName)
}

// GetFilesystem returns the manifest to create a Rook filesystem resource with the given config.
func (m *CephManifestsPreviousVersion) GetFilesystem(name string, activeCount int) string {
	return m.latest.GetFilesystem(name, activeCount)
}

// GetNFS returns the manifest to create a Rook Ceph NFS resource with the given config.
func (m *CephManifestsPreviousVersion) GetNFS(name string, count int) string {
	return m.latest.GetNFS(name, count)
}

func (m *CephManifestsPreviousVersion) GetNFSPool() string {
	return m.latest.GetNFSPool()
}

func (m *CephManifestsPreviousVersion) GetObjectStore(name string, replicaCount, port int, tlsEnable bool) string {
	return m.latest.GetObjectStore(name, replicaCount, port, tlsEnable)
}

func (m *CephManifestsPreviousVersion) GetObjectStoreUser(name, displayName, store, usercaps, maxsize string, maxbuckets, maxobjects int) string {
	return m.latest.GetObjectStoreUser(name, displayName, store, usercaps, maxsize, maxbuckets, maxobjects)
}

//GetBucketStorageClass returns the manifest to create object bucket
func (m *CephManifestsPreviousVersion) GetBucketStorageClass(storeName, storageClassName, reclaimPolicy string) string {
	return m.latest.GetBucketStorageClass(storeName, storageClassName, reclaimPolicy)
}

//GetOBC returns the manifest to create object bucket claim
func (m *CephManifestsPreviousVersion) GetOBC(claimName, storageClassName, objectBucketName, maxObject string, varBucketName bool) string {
	return m.latest.GetOBC(claimName, storageClassName, objectBucketName, maxObject, varBucketName)
}

//GetOBCNotification returns the manifest to create object bucket claim
func (m *CephManifestsPreviousVersion) GetOBCNotification(claimName, storageClassName, objectBucketName, notificationName string, varBucketName bool) string {
	return m.latest.GetOBCNotification(claimName, storageClassName, objectBucketName, notificationName, varBucketName)
}

//GetBucketNotification returns the manifest to create ceph bucket notification
func (m *CephManifestsPreviousVersion) GetBucketNotification(notificationName, topicName string) string {
	return m.latest.GetBucketNotification(notificationName, topicName)
}

//GetBucketTopic returns the manifest to create ceph bucket topic
func (m *CephManifestsPreviousVersion) GetBucketTopic(topicName, storeName, httpEndpointService string) string {
	return m.latest.GetBucketTopic(topicName, storeName, httpEndpointService)
}

func (m *CephManifestsPreviousVersion) GetClient(claimName string, caps map[string]string) string {
	return m.latest.GetClient(claimName, caps)
}

func (m *CephManifestsPreviousVersion) GetExternalCephCluster() string {
	return m.latest.GetExternalCephCluster()
}

// GetRBDMirror returns the manifest to create a Rook Ceph RBD Mirror resource with the given config.
func (m *CephManifestsPreviousVersion) GetRBDMirror(name string, count int) string {
	return m.latest.GetRBDMirror(name, count)
}
