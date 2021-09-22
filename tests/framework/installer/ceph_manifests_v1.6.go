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
	"strconv"
	"strings"

	"github.com/rook/rook/tests/framework/utils"
)

const (
	// The version from which the upgrade test will start
	Version1_6 = "v1.6.7"
)

// CephManifestsV1_6 wraps rook yaml definitions
type CephManifestsV1_6 struct {
	settings *TestCephSettings
}

func (m *CephManifestsV1_6) Settings() *TestCephSettings {
	return m.settings
}

func (m *CephManifestsV1_6) GetCRDs(k8shelper *utils.K8sHelper) string {
	return m.settings.readManifestFromGithub("crds.yaml")
}

// GetRookOperator returns rook Operator manifest
func (m *CephManifestsV1_6) GetOperator() string {
	var manifest string
	if utils.IsPlatformOpenShift() {
		manifest = m.settings.readManifestFromGithub("operator-openshift.yaml")
	} else {
		manifest = m.settings.readManifestFromGithub("operator.yaml")
	}
	return m.settings.replaceOperatorSettings(manifest)
}

// GetCommon returns rook-cluster manifest
func (m *CephManifestsV1_6) GetCommon() string {
	return m.settings.readManifestFromGithub("common.yaml")
}

// GetRookToolBox returns rook-toolbox manifest
func (m *CephManifestsV1_6) GetToolbox() string {
	if m.settings.DirectMountToolbox {
		manifest := strings.ReplaceAll(m.settings.readManifestFromGithub("direct-mount.yaml"), "name: rook-direct-mount", "name: rook-ceph-tools")
		return strings.ReplaceAll(manifest, "app: rook-direct-mount", "app: rook-ceph-tools")
	}
	return m.settings.readManifestFromGithub("toolbox.yaml")
}

func (m *CephManifestsV1_6) GetCommonExternal() string {
	return m.settings.readManifestFromGithub("common-external.yaml")
}

//**********************************************************************************
//**********************************************************************************
// After a release, replace the methods below this separator from the
// ceph_manifests.go. Methods above this separator do not need to be
// copied since they will load them directly from github.
//**********************************************************************************
//**********************************************************************************

// GetRookCluster returns rook-cluster manifest
func (m *CephManifestsV1_6) GetCephCluster() string {
	crushRoot := "# crushRoot not specified; Rook will use `default`"
	if m.settings.Mons == 1 {
		crushRoot = `crushRoot: "custom-root"`
	}

	if m.settings.UsePVC {
		return `apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  # set the name to something different from the namespace
  name: ` + m.settings.ClusterName + `
  namespace: ` + m.settings.Namespace + `
spec:
  dataDirHostPath: ` + m.settings.DataDirHostPath + `
  mon:
    count: ` + strconv.Itoa(m.settings.Mons) + `
    allowMultiplePerNode: true
    volumeClaimTemplate:
      spec:
        storageClassName: ` + m.settings.StorageClassName + `
        resources:
          requests:
            storage: 5Gi
  cephVersion:
    image: ` + m.settings.CephVersion.Image + `
    allowUnsupported: ` + strconv.FormatBool(m.settings.CephVersion.AllowUnsupported) + `
  skipUpgradeChecks: false
  continueUpgradeAfterChecksEvenIfNotHealthy: false
  dashboard:
    enabled: true
  network:
    hostNetwork: false
  crashCollector:
    disable: false
  storage:
    config:
      ` + crushRoot + `
    storageClassDeviceSets:
    - name: set1
      count: 1
      portable: false
      tuneDeviceClass: true
      encrypted: false
      volumeClaimTemplates:
      - metadata:
          name: data
        spec:
          resources:
            requests:
              storage: 10Gi
          storageClassName: ` + m.settings.StorageClassName + `
          volumeMode: Block
          accessModes:
            - ReadWriteOnce
  disruptionManagement:
    managePodBudgets: false
    osdMaintenanceTimeout: 30
    pgHealthCheckTimeout: 0
    manageMachineDisruptionBudgets: false
    machineDisruptionBudgetNamespace: openshift-machine-api`
	}

	return `apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: ` + m.settings.ClusterName + `
  namespace: ` + m.settings.Namespace + `
spec:
  cephVersion:
    image: ` + m.settings.CephVersion.Image + `
    allowUnsupported: ` + strconv.FormatBool(m.settings.CephVersion.AllowUnsupported) + `
  dataDirHostPath: ` + m.settings.DataDirHostPath + `
  network:
    hostNetwork: false
  mon:
    count: ` + strconv.Itoa(m.settings.Mons) + `
    allowMultiplePerNode: true
  dashboard:
    enabled: true
  skipUpgradeChecks: true
  metadataDevice:
  storage:
    useAllNodes: ` + strconv.FormatBool(!m.settings.SkipOSDCreation) + `
    useAllDevices: ` + strconv.FormatBool(!m.settings.SkipOSDCreation) + `
    deviceFilter:  ''
    config:
      databaseSizeMB: "1024"
      journalSizeMB: "1024"
  mgr:
    modules:
    - name: pg_autoscaler
      enabled: true
    - name: rook
      enabled: true
  healthCheck:
    daemonHealth:
      mon:
        interval: 10s
        timeout: 15s
      osd:
        interval: 10s
      status:
        interval: 5s`
}

func (m *CephManifestsV1_6) GetBlockSnapshotClass(snapshotClassName, reclaimPolicy string) string {
	// Create a CSI driver snapshotclass
	return `
apiVersion: snapshot.storage.k8s.io/v1beta1
kind: VolumeSnapshotClass
metadata:
  name: ` + snapshotClassName + `
driver: ` + m.settings.OperatorNamespace + `.rbd.csi.ceph.com
deletionPolicy: ` + reclaimPolicy + `
parameters:
  clusterID: ` + m.settings.Namespace + `
  csi.storage.k8s.io/snapshotter-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/snapshotter-secret-namespace: ` + m.settings.Namespace + `
`
}

func (m *CephManifestsV1_6) GetFileStorageSnapshotClass(snapshotClassName, reclaimPolicy string) string {
	// Create a CSI driver snapshotclass
	return `
apiVersion: snapshot.storage.k8s.io/v1beta1
kind: VolumeSnapshotClass
metadata:
  name: ` + snapshotClassName + `
driver: ` + m.settings.OperatorNamespace + `.cephfs.csi.ceph.com
deletionPolicy: ` + reclaimPolicy + `
parameters:
  clusterID: ` + m.settings.Namespace + `
  csi.storage.k8s.io/snapshotter-secret-name: rook-csi-cephfs-provisioner
  csi.storage.k8s.io/snapshotter-secret-namespace: ` + m.settings.Namespace + `
`
}

func (m *CephManifestsV1_6) GetBlockPool(poolName, replicaSize string) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: ` + poolName + `
  namespace: ` + m.settings.Namespace + `
spec:
  replicated:
    size: ` + replicaSize + `
    targetSizeRatio: .5
    requireSafeReplicaSize: false
  compressionMode: aggressive
  mirroring:
    enabled: true
    mode: image
  quotas:
    maxBytes: 10737418240
    maxObjects: 1000000
  statusCheck:
    mirror:
      disabled: false
      interval: 10s`
}

func (m *CephManifestsV1_6) GetBlockStorageClass(poolName, storageClassName, reclaimPolicy string) string {
	// Create a CSI driver storage class
	return `
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ` + storageClassName + `
provisioner: ` + m.settings.OperatorNamespace + `.rbd.csi.ceph.com
reclaimPolicy: ` + reclaimPolicy + `
parameters:
  pool: ` + poolName + `
  clusterID: ` + m.settings.Namespace + `
  csi.storage.k8s.io/provisioner-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/provisioner-secret-namespace: ` + m.settings.Namespace + `
  csi.storage.k8s.io/node-stage-secret-name: rook-csi-rbd-node
  csi.storage.k8s.io/node-stage-secret-namespace: ` + m.settings.Namespace + `
  imageFeatures: layering
  csi.storage.k8s.io/fstype: ext4
`
}

func (m *CephManifestsV1_6) GetFileStorageClass(fsName, storageClassName string) string {
	// Create a CSI driver storage class
	csiCephFSNodeSecret := "rook-csi-cephfs-node"               //nolint:gosec // We safely suppress gosec in tests file
	csiCephFSProvisionerSecret := "rook-csi-cephfs-provisioner" //nolint:gosec // We safely suppress gosec in tests file
	return `
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ` + storageClassName + `
provisioner: ` + m.settings.OperatorNamespace + `.cephfs.csi.ceph.com
parameters:
  clusterID: ` + m.settings.Namespace + `
  fsName: ` + fsName + `
  pool: ` + fsName + `-data0
  csi.storage.k8s.io/provisioner-secret-name: ` + csiCephFSProvisionerSecret + `
  csi.storage.k8s.io/provisioner-secret-namespace: ` + m.settings.Namespace + `
  csi.storage.k8s.io/node-stage-secret-name: ` + csiCephFSNodeSecret + `
  csi.storage.k8s.io/node-stage-secret-namespace: ` + m.settings.Namespace + `
`
}

// GetFilesystem returns the manifest to create a Rook filesystem resource with the given config.
func (m *CephManifestsV1_6) GetFilesystem(name string, activeCount int) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephFilesystem
metadata:
  name: ` + name + `
  namespace: ` + m.settings.Namespace + `
spec:
  metadataPool:
    replicated:
      size: 1
      requireSafeReplicaSize: false
  dataPools:
  - replicated:
      size: 1
      requireSafeReplicaSize: false
    compressionMode: none
  metadataServer:
    activeCount: ` + strconv.Itoa(activeCount) + `
    activeStandby: true`
}

// GetFilesystem returns the manifest to create a Rook Ceph NFS resource with the given config.
func (m *CephManifestsV1_6) GetNFS(name, pool string, count int) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephNFS
metadata:
  name: ` + name + `
  namespace: ` + m.settings.Namespace + `
spec:
  rados:
    pool: ` + pool + `
    namespace: nfs-ns
  server:
    active: ` + strconv.Itoa(count)
}

func (m *CephManifestsV1_6) GetObjectStore(name string, replicaCount, port int, tlsEnable bool) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: ` + name + `
  namespace: ` + m.settings.Namespace + `
spec:
  metadataPool:
    replicated:
      size: 1
      requireSafeReplicaSize: false
    compressionMode: passive
  dataPool:
    replicated:
      size: 1
      requireSafeReplicaSize: false
  gateway:
    sslCertificateRef:
    port: ` + strconv.Itoa(port) + `
    instances: ` + strconv.Itoa(replicaCount) + `
  healthCheck:
    bucket:
      disabled: false
      interval: 10s
`
}

func (m *CephManifestsV1_6) GetObjectStoreUser(name, displayName, store, usercaps, maxsize string, maxbuckets, maxobjects int) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: ` + name + `
  namespace: ` + m.settings.Namespace + `
spec:
  displayName: ` + displayName + `
  store: ` + store
}

//GetBucketStorageClass returns the manifest to create object bucket
func (m *CephManifestsV1_6) GetBucketStorageClass(storeName, storageClassName, reclaimPolicy, region string) string {
	return `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: ` + storageClassName + `
provisioner: ` + m.settings.Namespace + `.ceph.rook.io/bucket
reclaimPolicy: ` + reclaimPolicy + `
parameters:
    objectStoreName: ` + storeName + `
    objectStoreNamespace: ` + m.settings.Namespace + `
    region: ` + region
}

//GetOBC returns the manifest to create object bucket claim
func (m *CephManifestsV1_6) GetOBC(claimName, storageClassName, objectBucketName, maxObject string, varBucketName bool) string {
	bucketParameter := "generateBucketName"
	if varBucketName {
		bucketParameter = "bucketName"
	}
	return `apiVersion: objectbucket.io/v1alpha1
kind: ObjectBucketClaim
metadata:
  name: ` + claimName + `
spec:
  ` + bucketParameter + `: ` + objectBucketName + `
  storageClassName: ` + storageClassName + `
  additionalConfig:
    maxObjects: "` + maxObject + `"`
}

func (m *CephManifestsV1_6) GetClient(claimName string, caps map[string]string) string {
	clientCaps := []string{}
	for name, cap := range caps {
		str := name + ": " + cap
		clientCaps = append(clientCaps, str)
	}
	return `apiVersion: ceph.rook.io/v1
kind: CephClient
metadata:
  name: ` + claimName + `
  namespace: ` + m.settings.Namespace + `
spec:
  caps:
    ` + strings.Join(clientCaps, "\n    ")
}

func (m *CephManifestsV1_6) GetExternalCephCluster() string {
	return `apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: ` + m.settings.Namespace + `
  namespace: ` + m.settings.Namespace + `
spec:
  external:
    enable: true
  dataDirHostPath: ` + m.settings.DataDirHostPath + ``
}

// GetRBDMirror returns the manifest to create a Rook Ceph RBD Mirror resource with the given config.
func (m *CephManifestsV1_6) GetRBDMirror(name string, count int) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephRBDMirror
metadata:
  name: ` + name + `
  namespace: ` + m.settings.Namespace + `
spec:
  count: ` + strconv.Itoa(count)
}
