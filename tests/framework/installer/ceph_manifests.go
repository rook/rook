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
	"strconv"

	"github.com/google/uuid"
)

type CephManifests interface {
	GetRookCRDs() string
	GetRookOperator(namespace string) string
	GetClusterRoles(namespace, systemNamespace string) string
	GetRookCluster(settings *ClusterSettings) string
	GetRookToolBox(namespace string) string
	GetCleanupPod(node, removalDir string) string
	GetBlockPoolDef(poolName string, namespace string, replicaSize string) string
	GetBlockStorageClassDef(poolName string, storageClassName string, reclaimPolicy string, namespace string, varClusterName bool) string
	GetBlockPvcDef(claimName string, storageClassName string, accessModes string) string
	GetBlockPoolStorageClassAndPvcDef(namespace string, poolName string, storageClassName string, reclaimPolicy string, blockName string, accessMode string) string
	GetBlockPoolStorageClass(namespace string, poolName string, storageClassName string, reclaimPolicy string) string
	GetFilesystem(namepace, name string) string
	GetObjectStore(namespace, name string, replicaCount, port int) string
}

type ClusterSettings struct {
	Namespace       string
	StoreType       string
	DataDirHostPath string
	UseAllDevices   bool
	Mons            int
}

//CephManifestsMaster wraps rook yaml definitions
type CephManifestsMaster struct {
	imageTag string
}

// NewCephManifests gets the manifest type depending on the Rook version desired
func NewCephManifests(version string) CephManifests {
	switch version {
	case VersionMaster:
		return &CephManifestsMaster{imageTag: VersionMaster}
	case Version0_8:
		return &CephManifestsV0_8{imageTag: Version0_8}
	}
	panic(fmt.Errorf("unrecognized ceph manifest version: %s", version))
}

func (m *CephManifestsMaster) GetRookCRDs() string {
	return `apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: clusters.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: Cluster
    listKind: ClusterList
    plural: clusters
    singular: cluster
  scope: Namespaced
  version: v1beta1
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: filesystems.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: Filesystem
    listKind: FilesystemList
    plural: filesystems
    singular: filesystem
  scope: Namespaced
  version: v1beta1
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: objectstores.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: ObjectStore
    listKind: ObjectStoreList
    plural: objectstores
    singular: objectstore
  scope: Namespaced
  version: v1beta1
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: pools.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: Pool
    listKind: PoolList
    plural: pools
    singular: pool
  scope: Namespaced
  version: v1beta1
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: volumes.rook.io
spec:
  group: rook.io
  names:
    kind: Volume
    listKind: VolumeList
    plural: volumes
    singular: volume
  scope: Namespaced
  version: v1alpha2`
}

//GetRookOperator returns rook Operator  manifest
func (m *CephManifestsMaster) GetRookOperator(namespace string) string {

	return `kind: Namespace
apiVersion: v1
metadata:
  name: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: Role
metadata:
  name: rook-ceph-system
  namespace: ` + namespace + `
  labels:
    operator: rook
    storage-backend: ceph
rules:
- apiGroups:
  - ""
  resources:
  - pods
  - configmaps
  verbs:
  - get
  - list
  - watch
  - patch
  - create
  - update
  - delete
- apiGroups:
  - extensions
  resources:
  - daemonsets
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - delete
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-ceph-cluster-mgmt
  labels:
    operator: rook
    storage-backend: ceph
rules:
- apiGroups:
  - ""
  resources:
  - secrets
  - pods
  - services
  - configmaps
  verbs:
  - get
  - list
  - watch
  - patch
  - create
  - update
  - delete
- apiGroups:
  - extensions
  resources:
  - deployments
  - daemonsets
  - replicasets
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - delete
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-ceph-global
  labels:
    operator: rook
    storage-backend: ceph
rules:
- apiGroups:
  - ""
  resources:
  - pods
  - nodes
  - nodes/proxy
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - events
  - persistentvolumes
  - persistentvolumeclaims
  verbs:
  - get
  - list
  - watch
  - patch
  - create
  - update
  - delete
- apiGroups:
  - storage.k8s.io
  resources:
  - storageclasses
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - batch
  resources:
  - jobs
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - delete
- apiGroups:
  - ceph.rook.io
  resources:
  - "*"
  verbs:
  - "*"
- apiGroups:
  - rook.io
  resources:
  - "*"
  verbs:
  - "*"
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-system
  namespace: ` + namespace + `
  labels:
    operator: rook
    storage-backend: ceph
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-system
  namespace: ` + namespace + `
  labels:
    operator: rook
    storage-backend: ceph
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-system
subjects:
- kind: ServiceAccount
  name: rook-ceph-system
  namespace: ` + namespace + `
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-global
  namespace: ` + namespace + `
  labels:
    operator: rook
    storage-backend: ceph
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-global
subjects:
- kind: ServiceAccount
  name: rook-ceph-system
  namespace: ` + namespace + `
---
apiVersion: apps/v1beta1
kind: Deployment
metadata:
  name: rook-ceph-operator
  namespace: ` + namespace + `
  labels:
    operator: rook
    storage-backend: ceph
spec:
  replicas: 1
  template:
    metadata:
      labels:
        app: rook-ceph-operator
    spec:
      serviceAccountName: rook-ceph-system
      containers:
      - name: rook-ceph-operator
        image: rook/ceph:` + m.imageTag + `
        args: ["ceph", "operator"]
        env:
        - name: ROOK_LOG_LEVEL
          value: INFO
        - name: ROOK_MON_HEALTHCHECK_INTERVAL
          value: "10s"
        - name: ROOK_MON_OUT_TIMEOUT
          value: "15s"
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace`
}

//GetRookCluster returns rook-cluster manifest
func (m *CephManifestsMaster) GetClusterRoles(namespace, systemNamespace string) string {
	return `apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-cluster
  namespace: ` + namespace + `
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cluster
  namespace: ` + namespace + `
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: [ "get", "list", "watch", "create", "update", "delete" ]
---
# Allow the operator to create resources in this cluster's namespace
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cluster-mgmt
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-cluster-mgmt
subjects:
- kind: ServiceAccount
  name: rook-ceph-system
  namespace: ` + systemNamespace + `
---
# Allow the pods in this namespace to work with configmaps
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cluster
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-cluster
subjects:
- kind: ServiceAccount
  name: rook-ceph-cluster
  namespace: ` + namespace
}

//GetRookCluster returns rook-cluster manifest
func (m *CephManifestsMaster) GetRookCluster(settings *ClusterSettings) string {
	return `apiVersion: ceph.rook.io/v1beta1
kind: Cluster
metadata:
  name: ` + settings.Namespace + `
  namespace: ` + settings.Namespace + `
spec:
  serviceAccount: rook-ceph-cluster
  dataDirHostPath: ` + settings.DataDirHostPath + `
  network:
    hostNetwork: false
  mon:
    count: ` + strconv.Itoa(settings.Mons) + `
    allowMultiplePerNode: true
  dashboard:
    enabled: true
  metadataDevice:
  storage:
    useAllNodes: true
    useAllDevices: ` + strconv.FormatBool(settings.UseAllDevices) + `
    deviceFilter:
    location:
    config:
      storeType: "` + settings.StoreType + `"
      databaseSizeMB: "1024"
      journalSizeMB: "1024"`
}

//GetRookToolBox returns rook-toolbox manifest
func (m *CephManifestsMaster) GetRookToolBox(namespace string) string {
	return `apiVersion: v1
kind: Pod
metadata:
  name: rook-ceph-tools
  namespace: ` + namespace + `
spec:
  dnsPolicy: ClusterFirstWithHostNet
  containers:
  - name: rook-ceph-tools
    image: rook/ceph-toolbox:` + m.imageTag + `
    imagePullPolicy: IfNotPresent
    env:
      - name: ROOK_ADMIN_SECRET
        valueFrom:
          secretKeyRef:
            name: rook-ceph-mon
            key: admin-secret
    securityContext:
      privileged: true
    volumeMounts:
      - mountPath: /dev
        name: dev
      - mountPath: /sys/bus
        name: sysbus
      - mountPath: /lib/modules
        name: libmodules
      - name: mon-endpoint-volume
        mountPath: /etc/rook
  hostNetwork: false
  volumes:
    - name: dev
      hostPath:
        path: /dev
    - name: sysbus
      hostPath:
        path: /sys/bus
    - name: libmodules
      hostPath:
        path: /lib/modules
    - name: mon-endpoint-volume
      configMap:
        name: rook-ceph-mon-endpoints
        items:
        - key: data
          path: mon-endpoints`
}

//GetCleanupPod gets a cleanup Pod manifest
func (m *CephManifestsMaster) GetCleanupPod(node, removalDir string) string {
	return `apiVersion: batch/v1
kind: Job
metadata:
  name: rook-cleanup-` + uuid.Must(uuid.NewRandom()).String() + `
spec:
    template:
      spec:
          restartPolicy: Never
          containers:
              - name: rook-cleaner
                image: rook/ceph:` + m.imageTag + `
                securityContext:
                    privileged: true
                volumeMounts:
                    - name: cleaner
                      mountPath: /scrub
                command:
                    - "sh"
                    - "-c"
                    - "rm -rf /scrub/*"
          nodeSelector:
            kubernetes.io/hostname: ` + node + `
          volumes:
              - name: cleaner
                hostPath:
                   path:  ` + removalDir
}

func (m *CephManifestsMaster) GetBlockPoolDef(poolName string, namespace string, replicaSize string) string {
	return `apiVersion: ceph.rook.io/v1beta1
kind: Pool
metadata:
  name: ` + poolName + `
  namespace: ` + namespace + `
spec:
  replicated:
    size: ` + replicaSize
}

func (m *CephManifestsMaster) GetBlockStorageClassDef(poolName string, storageClassName string, reclaimPolicy string, namespace string, varClusterName bool) string {
	namespaceParameter := "clusterNamespace"
	if varClusterName {
		namespaceParameter = "clusterName"
	}
	return `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: ` + storageClassName + `
provisioner: ceph.rook.io/block
reclaimPolicy: ` + reclaimPolicy + `
parameters:
    pool: ` + poolName + `
    ` + namespaceParameter + `: ` + namespace
}

func (m *CephManifestsMaster) GetBlockPvcDef(claimName string, storageClassName string, accessModes string) string {
	return `apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ` + claimName + `
  annotations:
    volume.beta.kubernetes.io/storage-class: ` + storageClassName + `
spec:
  accessModes:
    - ` + accessModes + `
  resources:
    requests:
      storage: 1M`
}

func (m *CephManifestsMaster) GetBlockPoolStorageClassAndPvcDef(namespace string, poolName string, storageClassName string, reclaimPolicy string, blockName string, accessMode string) string {
	return concatYaml(m.GetBlockPoolDef(poolName, namespace, "1"),
		concatYaml(m.GetBlockStorageClassDef(poolName, storageClassName, reclaimPolicy, namespace, false), m.GetBlockPvcDef(blockName, storageClassName, accessMode)))
}

func (m *CephManifestsMaster) GetBlockPoolStorageClass(namespace string, poolName string, storageClassName string, reclaimPolicy string) string {
	return concatYaml(m.GetBlockPoolDef(poolName, namespace, "1"), m.GetBlockStorageClassDef(poolName, storageClassName, reclaimPolicy, namespace, false))
}

func (m *CephManifestsMaster) GetFilesystem(namespace, name string) string {
	return `apiVersion: ceph.rook.io/v1beta1
kind: Filesystem
metadata:
  name: ` + name + `
  namespace: ` + namespace + `
spec:
  metadataPool:
    replicated:
      size: 1
  dataPools:
  - replicated:
      size: 1
  metadataServer:
    activeCount: 1
    activeStandby: true`
}

func (m *CephManifestsMaster) GetObjectStore(namespace, name string, replicaCount, port int) string {
	return `apiVersion: ceph.rook.io/v1beta1
kind: ObjectStore
metadata:
  name: ` + name + `
  namespace: ` + namespace + `
spec:
  metadataPool:
    replicated:
      size: 1
  dataPool:
    replicated:
      size: 1
  gateway:
    type: s3
    sslCertificateRef:
    port: ` + strconv.Itoa(port) + `
    securePort:
    instances: ` + strconv.Itoa(replicaCount) + `
    allNodes: false
`
}
