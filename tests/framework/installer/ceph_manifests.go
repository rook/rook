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
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/rook/rook/pkg/operator/ceph/object/cosi"
	"github.com/rook/rook/tests/framework/utils"
)

type CephManifests interface {
	Settings() *TestCephSettings
	GetCRDs(k8shelper *utils.K8sHelper) string
	GetCSINFSRBAC() string
	GetOperator() string
	GetCommon() string
	GetCommonExternal() string
	GetCephCluster() string
	GetExternalCephCluster() string
	GetToolbox() string
	GetBlockPool(poolName, replicaSize string) string
	GetBlockStorageClass(poolName, storageClassName, reclaimPolicy string) string
	GetFileStorageClass(fsName, storageClassName string) string
	GetNFSStorageClass(fsName, nfsClusterName, server, storageClassName string) string
	GetNFSSnapshotClass(fsName, snapshotClassName string) string
	GetBlockSnapshotClass(snapshotClassName, reclaimPolicy string) string
	GetFileStorageSnapshotClass(snapshotClassName, reclaimPolicy string) string
	GetFilesystem(name string, activeCount int) string
	GetNFS(name string, daemonCount int) string
	GetNFSPool() string
	GetRBDMirror(name string, daemonCount int) string
	GetObjectStore(name string, replicaCount, port int, tlsEnable bool, swiftAndKeystone bool) string
	GetObjectStoreUser(name, displayName, store, usercaps, maxsize string, maxbuckets, maxobjects int) string
	GetBucketStorageClass(storeName, storageClassName, reclaimPolicy string) string
	GetOBC(obcName, storageClassName, bucketName string, maxObject string, createBucket bool) string
	GetOBCNotification(obcName, storageClassName, bucketName string, notificationName string, createBucket bool) string
	GetBucketNotification(notificationName string, topicName string) string
	GetBucketTopic(topicName string, storeName string, httpEndpointService string) string
	GetClient(name string, caps map[string]string) string
	GetFilesystemSubvolumeGroup(fsName, groupName string) string
	GetCOSIDriver() string
	GetBucketClass(name, objectstoreUserName, deletionPolicy string) string
	GetBucketClaim(claimName, className string) string
}

// CephManifestsMaster wraps rook yaml definitions
type CephManifestsMaster struct {
	settings *TestCephSettings
}

// NewCephManifests gets the manifest type depending on the Rook version desired
func NewCephManifests(settings *TestCephSettings) CephManifests {
	switch settings.RookVersion {
	case LocalBuildTag:
		return &CephManifestsMaster{settings}
	case Version1_14:
		return &CephManifestsPreviousVersion{settings, &CephManifestsMaster{settings}}
	}
	panic(fmt.Errorf("unrecognized ceph manifest version: %s", settings.RookVersion))
}

func (m *CephManifestsMaster) Settings() *TestCephSettings {
	return m.settings
}

func (m *CephManifestsMaster) GetCRDs(k8shelper *utils.K8sHelper) string {
	return m.settings.readManifest("crds.yaml")
}

func (m *CephManifestsMaster) GetCSINFSRBAC() string {
	return m.settings.readManifest("/csi/nfs/rbac.yaml")
}

func (m *CephManifestsMaster) GetOperator() string {
	var manifest string
	if utils.IsPlatformOpenShift() {
		manifest = m.settings.readManifest("operator-openshift.yaml")
	} else {
		manifest = m.settings.readManifest("operator.yaml")
	}
	return m.settings.replaceOperatorSettings(manifest)
}

func (m *CephManifestsMaster) GetCommonExternal() string {
	return m.settings.readManifest("common-external.yaml")
}

func (m *CephManifestsMaster) GetCommon() string {
	return m.settings.readManifest("common.yaml")
}

func (m *CephManifestsMaster) GetToolbox() string {
	if m.settings.DirectMountToolbox {
		manifest := strings.ReplaceAll(m.settings.readManifest("direct-mount.yaml"), "name: rook-direct-mount", "name: rook-ceph-tools")
		manifest = strings.ReplaceAll(manifest, "name: rook-direct-mount", "name: rook-ceph-tools")
		return strings.ReplaceAll(manifest, "app: rook-direct-mount", "app: rook-ceph-tools")
	}
	manifest := m.settings.readManifest("toolbox.yaml")
	if m.settings.CephVersion.Image != "" {
		// The toolbox uses the ceph image, so replace the version that is being tested
		// The regex allows for any character in the tag ("\S" --> non-whitespace character)
		versionRegex := regexp.MustCompile(`image: quay.io/ceph/ceph:\S+`)
		manifest = versionRegex.ReplaceAllString(manifest, "image: "+m.settings.CephVersion.Image)
	}
	return manifest
}

//**********************************************************************************
//**********************************************************************************
// After a release, copy the methods below this separator into the versioned file
// such as ceph_manifests_previous.go. Methods above this separator do not need to be
// copied since the versioned implementation will load them directly from github.
//**********************************************************************************
//**********************************************************************************

func (m *CephManifestsMaster) GetCephCluster() string {
	crushRoot := "# crushRoot not specified; Rook will use `default`"
	if m.settings.Mons == 1 {
		crushRoot = `crushRoot: "custom-root"`
	}

	pruner := "# daysToRetain not specified;"
	if m.settings.UseCrashPruner {
		pruner = "daysToRetain: 5"
	}

	mgrCount := 1
	if m.settings.MultipleMgrs {
		mgrCount = 2
	}
	clusterSpec := `apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  # set the name to something different from the namespace
  name: ` + m.settings.ClusterName + `
  namespace: ` + m.settings.Namespace + `
spec:
  resources: null
  dataDirHostPath: ` + m.settings.DataDirHostPath + `
  cephVersion:
    image: ` + m.settings.CephVersion.Image + `
    allowUnsupported: ` + strconv.FormatBool(m.settings.CephVersion.AllowUnsupported) + `
  skipUpgradeChecks: true
  continueUpgradeAfterChecksEvenIfNotHealthy: false
  mgr:
    count: ` + strconv.Itoa(mgrCount) + `
    allowMultiplePerNode: true
  dashboard:
    enabled: true
  network:
    hostNetwork: false
    connections:
      requireMsgr2: ` + strconv.FormatBool(m.settings.RequireMsgr2) + `
      encryption:
        enabled: ` + strconv.FormatBool(m.settings.ConnectionsEncrypted) + `
      compression:
        enabled: ` + strconv.FormatBool(m.settings.ConnectionsCompressed) + `
  crashCollector:
    disable: false
    ` + pruner + `
  logCollector:
    enabled: true
    periodicity: daily
    maxLogSize: 500M
  disruptionManagement:
    managePodBudgets: true
    osdMaintenanceTimeout: 30
    pgHealthCheckTimeout: 0
  healthCheck:
    daemonHealth:
      mon:
        interval: 10s
        timeout: 15s
      osd:
        interval: 10s
      status:
        interval: 5s`

	if m.settings.UsePVC {
		clusterSpec += `
  mon:
    count: ` + strconv.Itoa(m.settings.Mons) + `
    allowMultiplePerNode: true
    volumeClaimTemplate:
      spec:
        storageClassName: ` + m.settings.StorageClassName + `
        resources:
          requests:
            storage: 5Gi
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
`
	} else {
		clusterSpec += `
  mon:
    count: ` + strconv.Itoa(m.settings.Mons) + `
    allowMultiplePerNode: true
  storage:
    useAllNodes: ` + strconv.FormatBool(!m.settings.SkipOSDCreation) + `
    useAllDevices: ` + strconv.FormatBool(!m.settings.SkipOSDCreation) + `
    deviceFilter:  ` + getDeviceFilter() + `
    config:
      databaseSizeMB: "1024"
    fullRatio: 0.96
    backfillFullRatio: 0.91
    nearFullRatio: 0.88
`
	}

	if m.settings.ConnectionsEncrypted {
		clusterSpec += `
  csi:
    cephfs:
      kernelMountOptions: ms_mode=secure
  `
	}
	return clusterSpec + `
  priorityClassNames:
    mon: system-node-critical
    osd: system-node-critical
    mgr: system-cluster-critical
`
}

func (m *CephManifestsMaster) GetBlockSnapshotClass(snapshotClassName, reclaimPolicy string) string {
	// Create a CSI driver snapshotclass
	return `
apiVersion: snapshot.storage.k8s.io/v1
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

func (m *CephManifestsMaster) GetFileStorageSnapshotClass(snapshotClassName, reclaimPolicy string) string {
	// Create a CSI driver snapshotclass
	return `
apiVersion: snapshot.storage.k8s.io/v1
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

func (m *CephManifestsMaster) GetBlockPool(poolName string, replicaSize string) string {
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
  parameters:
    compression_mode: aggressive
  mirroring:
    enabled: true
    mode: image
  quotas:
    maxSize: 10Gi
    maxObjects: 1000000
  statusCheck:
    mirror:
      disabled: false
      interval: 10s`
}

func (m *CephManifestsMaster) GetBlockStorageClass(poolName, storageClassName, reclaimPolicy string) string {
	// Create a CSI driver storage class
	sc := `
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
	return sc
}

func (m *CephManifestsMaster) GetFileStorageClass(fsName, storageClassName string) string {
	// Create a CSI driver storage class
	csiCephFSNodeSecret := "rook-csi-cephfs-node"               //nolint:gosec // We safely suppress gosec in tests file
	csiCephFSProvisionerSecret := "rook-csi-cephfs-provisioner" //nolint:gosec // We safely suppress gosec in tests file
	sc := `
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
	if m.settings.ConnectionsEncrypted {
		// Encryption with kernel version <= 5.11 requires 'mounter: fuse'. For kernel version >= 5.12, it requires 'mounter: kernel'.
		// Since the Github action Minikube has kernel version > 5.12, the setting is set to 'mounter: kernel'.
		sc += "  mounter: kernel"
	}
	return sc
}

func (m *CephManifestsMaster) GetNFSStorageClass(fsName, nfsClusterName, server, storageClassName string) string {
	// Create a CSI driver storage class
	csiCephFSNodeSecret := "rook-csi-cephfs-node"               //nolint:gosec // We safely suppress gosec in tests file
	csiCephFSProvisionerSecret := "rook-csi-cephfs-provisioner" //nolint:gosec // We safely suppress gosec in tests file
	sc := `
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ` + storageClassName + `
provisioner: ` + m.settings.OperatorNamespace + `.nfs.csi.ceph.com
parameters:
  clusterID: ` + m.settings.Namespace + `
  fsName: ` + fsName + `
  nfsCluster: ` + nfsClusterName + `
  server: ` + server + `
  pool: ` + fsName + `-data0
  csi.storage.k8s.io/provisioner-secret-name: ` + csiCephFSProvisionerSecret + `
  csi.storage.k8s.io/provisioner-secret-namespace: ` + m.settings.Namespace + `
  csi.storage.k8s.io/node-stage-secret-name: ` + csiCephFSNodeSecret + `
  csi.storage.k8s.io/node-stage-secret-namespace: ` + m.settings.Namespace + `
mountOptions:
  - "nolock"
`
	// "nolock" mountOptions has been added to prevent the following error in ci environment:
	// rpc error: code = Internal desc = mount failed: exit status 32
	// Mounting command: mount
	// Mounting arguments: -t nfs <src> <dest>
	// Output: mount.nfs: rpc.statd is not running but is required for remote locking.
	// mount.nfs: Either use '-o nolock' to keep locks local, or start statd.
	return sc
}

func (m *CephManifestsMaster) GetNFSSnapshotClass(snapshotClassName, reclaimPolicy string) string {
	// return NFS CSI snapshotclass object.
	return `
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: ` + snapshotClassName + `
driver: ` + m.settings.OperatorNamespace + `.nfs.csi.ceph.com
deletionPolicy: ` + reclaimPolicy + `
parameters:
  clusterID: ` + m.settings.Namespace + `
  csi.storage.k8s.io/snapshotter-secret-name: rook-csi-cephfs-provisioner
  csi.storage.k8s.io/snapshotter-secret-namespace: ` + m.settings.Namespace + `
`
}

// GetFilesystem returns the manifest to create a Rook filesystem resource with the given config.
func (m *CephManifestsMaster) GetFilesystem(name string, activeCount int) string {
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
    resources: null
    activeCount: ` + strconv.Itoa(activeCount) + `
    activeStandby: true`
}

// GetFilesystem returns the manifest to create a Rook Ceph NFS resource with the given config.
func (m *CephManifestsMaster) GetNFS(name string, count int) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephNFS
metadata:
  name: ` + name + `
  namespace: ` + m.settings.Namespace + `
spec:
  rados:
    pool: .nfs
    namespace: nfs-ns
  server:
    active: ` + strconv.Itoa(count)
}

// GetFilesystem returns the manifest to create a Rook Ceph NFS resource with the given config.
func (m *CephManifestsMaster) GetNFSPool() string {
	return `apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: dot-nfs
  namespace: ` + m.settings.Namespace + `
spec:
  name: .nfs
  replicated:
    size: 1
    requireSafeReplicaSize: false`
}

func (m *CephManifestsMaster) GetObjectStore(name string, replicaCount, port int, tlsEnable bool, swiftAndKeystone bool) string {
	type Spec struct {
		Name             string
		TLS              bool
		Port             int
		ReplicaCount     int
		SwiftAndKeystone bool
		Manifests        *CephManifestsMaster
	}

	spec := Spec{
		Name:             name,
		TLS:              tlsEnable,
		ReplicaCount:     replicaCount,
		Port:             port,
		SwiftAndKeystone: swiftAndKeystone,
		Manifests:        m,
	}

	tmpl := `apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: {{ .Name }}
  namespace: {{ .Manifests.Settings.Namespace }}
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
  {{ if .SwiftAndKeystone }}
  auth:
    keystone:
      acceptedRoles:
        - admin
        - member
        - service
      implicitTenants: "true"
      revocationInterval: 1200
      serviceUserSecretName: usersecret
      tokenCacheSize: 1000
      url: https://keystone.{{ .Manifests.Settings.Namespace }}.svc/
  protocols:
    swift:
      accountInUrl: false
      urlPrefix: foobar
    s3:
      enabled: true
      authUseKeystone: true
  {{ end }}
  gateway:
    resources: null
    {{ if .TLS }}securePort: {{ .Port }}{{ else }}port: {{ .Port }}{{ end }}
    instances: {{ .ReplicaCount }}
    {{ if .SwiftAndKeystone }}
    caBundleRef: keystone-bundle
    {{ end }}
    {{ if .TLS }}sslCertificateRef: {{ .Name }}{{ end }}`

	return renderTemplate(tmpl, spec)
}

func (m *CephManifestsMaster) GetObjectStoreUser(name, displayName, store, usercaps, maxsize string, maxbuckets, maxobjects int) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: ` + name + `
  namespace: ` + m.settings.Namespace + `
spec:
  displayName: ` + displayName + `
  store: ` + store + `
  quotas:
    maxBuckets: ` + strconv.Itoa(maxbuckets) + `
    maxObjects: ` + strconv.Itoa(maxobjects) + `
    maxSize: ` + maxsize + `
  capabilities:
    user: "` + usercaps + `"
    bucket: "` + usercaps + `"
`
}

// GetBucketStorageClass returns the manifest to create object bucket
func (m *CephManifestsMaster) GetBucketStorageClass(storeName, storageClassName, reclaimPolicy string) string {
	return `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: ` + storageClassName + `
provisioner: ` + m.settings.Namespace + `.ceph.rook.io/bucket
reclaimPolicy: ` + reclaimPolicy + `
parameters:
    objectStoreName: ` + storeName + `
    objectStoreNamespace: ` + m.settings.Namespace
}

// GetOBC returns the manifest to create object bucket claim
func (m *CephManifestsMaster) GetOBC(claimName string, storageClassName string, objectBucketName string, maxObject string, varBucketName bool) string {
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

// GetOBCNotification returns the manifest to create object bucket claim
func (m *CephManifestsMaster) GetOBCNotification(claimName string, storageClassName string, objectBucketName string, notificationName string, varBucketName bool) string {
	bucketParameter := "generateBucketName"
	if varBucketName {
		bucketParameter = "bucketName"
	}
	return `apiVersion: objectbucket.io/v1alpha1
kind: ObjectBucketClaim
metadata:
  name: ` + claimName + `
  labels:
    bucket-notification-` + notificationName + `: ` + notificationName + `
spec:
  ` + bucketParameter + `: ` + objectBucketName + `
  storageClassName: ` + storageClassName
}

// GetBucketNotification returns the manifest to create ceph bucket notification
func (m *CephManifestsMaster) GetBucketNotification(notificationName string, topicName string) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephBucketNotification
metadata:
  name: ` + notificationName + `
spec:
  topic: ` + topicName + `
  events:
    - s3:ObjectCreated:Put
    - s3:ObjectRemoved:Delete
`
}

// GetBucketTopic returns the manifest to create ceph bucket topic
func (m *CephManifestsMaster) GetBucketTopic(topicName string, storeName string, httpEndpointService string) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephBucketTopic
metadata:
  name: ` + topicName + `
spec:
  endpoint:
    http:
      uri: ` + httpEndpointService + `
      sendCloudEvents: false
  objectStoreName: ` + storeName + `
  objectStoreNamespace: ` + m.settings.Namespace
}

func (m *CephManifestsMaster) GetClient(claimName string, caps map[string]string) string {
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

func (m *CephManifestsMaster) GetExternalCephCluster() string {
	return `apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: ` + m.settings.ClusterName + `
  namespace: ` + m.settings.Namespace + `
spec:
  external:
    enable: true
  dataDirHostPath: ` + m.settings.DataDirHostPath + `
  healthCheck:
    daemonHealth:
      status:
        interval: 5s`
}

// GetRBDMirror returns the manifest to create a Rook Ceph RBD Mirror resource with the given config.
func (m *CephManifestsMaster) GetRBDMirror(name string, count int) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephRBDMirror
metadata:
  name: ` + name + `
  namespace: ` + m.settings.Namespace + `
spec:
  count: ` + strconv.Itoa(count)
}

func (m *CephManifestsMaster) GetFilesystemSubvolumeGroup(fsName, groupName string) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephFilesystemSubVolumeGroup
metadata:
  name: ` + groupName + `
  namespace: ` + m.settings.Namespace + `
spec:
  filesystemName: ` + fsName + `
  quota: 10G
  dataPoolName: ` + fsName + "-data0"
}

func (m *CephManifestsMaster) GetCOSIDriver() string {
	// TODO: use the official image once it is available
	return `apiVersion: ceph.rook.io/v1
kind: CephCOSIDriver
metadata:
  name: ` + cosi.CephCOSIDriverName + `
  namespace: ` + m.settings.OperatorNamespace + `
spec:
  deploymentStrategy: Auto `
}

func (m *CephManifestsMaster) GetBucketClass(name, objectStoreUserSecretName, deletionPolicy string) string {
	return `apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClass
metadata:
  name: ` + name + `
  namespace: ` + m.settings.OperatorNamespace + `
driverName: ` + cosi.CephCOSIDriverPrefix + `.ceph.objectstorage.k8s.io
deletionPolicy: ` + deletionPolicy + `
parameters:
  objectStoreUserSecretName:  ` + objectStoreUserSecretName + `
  objectStoreUserSecretNamespace: ` + m.settings.Namespace
}

func (m *CephManifestsMaster) GetBucketClaim(name, bucketClassName string) string {
	return `apiVersion: objectstorage.k8s.io/v1alpha1
kind: BucketClaim
metadata:
  name: ` + name + `
  namespace: ` + m.settings.OperatorNamespace + `
spec:
  bucketClassName: ` + bucketClassName + `
  protocols:
    - s3 `
}
