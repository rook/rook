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
)

// CephManifestsV1_2 wraps rook yaml definitions
type CephManifestsV1_2 struct {
	imageTag string
}

func (m *CephManifestsV1_2) GetRookCRDs() string {
	return `
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephclusters.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephCluster
    listKind: CephClusterList
    plural: cephclusters
    singular: cephcluster
  scope: Namespaced
  version: v1
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            annotations: {}
            cephVersion:
              properties:
                allowUnsupported:
                  type: boolean
                image:
                  type: string
            dashboard:
              properties:
                enabled:
                  type: boolean
                urlPrefix:
                  type: string
                port:
                  type: integer
                  minimum: 0
                  maximum: 65535
                ssl:
                  type: boolean
            dataDirHostPath:
              pattern: ^/(\S+)
              type: string
            mon:
              properties:
                allowMultiplePerNode:
                  type: boolean
                count:
                  maximum: 9
                  minimum: 1
                  type: integer
            mgr:
              properties:
                modules:
                  items:
                    properties:
                      name:
                        type: string
                      enabled:
                        type: boolean
            network:
              properties:
                hostNetwork:
                  type: boolean
            storage:
              properties:
                disruptionManagement:
                  properties:
                    managePodBudgets:
                      type: boolean
                    osdMaintenanceTimeout:
                      type: integer
                    manageMachineDisruptionBudgets:
                      type: boolean
                useAllNodes:
                  type: boolean
                nodes:
                  items:
                    properties:
                      name:
                        type: string
                      config:
                        properties:
                          metadataDevice:
                            type: string
                          storeType:
                            type: string
                            pattern: ^(filestore|bluestore)$
                          databaseSizeMB:
                            type: string
                          walSizeMB:
                            type: string
                          journalSizeMB:
                            type: string
                          osdsPerDevice:
                            type: string
                          encryptedDevice:
                            type: string
                            pattern: ^(true|false)$
                      useAllDevices:
                        type: boolean
                      deviceFilter: {}
                      directories:
                        type: array
                        items:
                          properties:
                            path:
                              type: string
                      devices:
                        type: array
                        items:
                          properties:
                            name:
                              type: string
                            config: {}
                      location: {}
                      resources: {}
                  type: array
                useAllDevices:
                  type: boolean
                deviceFilter: {}
                location: {}
                directories:
                  type: array
                  items:
                    properties:
                      path:
                        type: string
                config: {}
                topologyAware:
                  type: boolean
            monitoring:
              properties:
                enabled:
                  type: boolean
                rulesNamespace:
                  type: string
            rbdMirroring:
              properties:
                workers:
                  type: integer
            placement: {}
            resources: {}
          required:
          - mon
  additionalPrinterColumns:
    - name: DataDirHostPath
      type: string
      description: Directory used on the K8s nodes
      JSONPath: .spec.dataDirHostPath
    - name: MonCount
      type: string
      description: Number of MONs
      JSONPath: .spec.mon.count
    - name: Age
      type: date
      JSONPath: .metadata.creationTimestamp
    - name: State
      type: string
      description: Current State
      JSONPath: .status.state
    - name: Health
      type: string
      description: Ceph Health
      JSONPath: .status.ceph.health
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephfilesystems.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephFilesystem
    listKind: CephFilesystemList
    plural: cephfilesystems
    singular: cephfilesystem
  scope: Namespaced
  version: v1
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            metadataServer:
              properties:
                activeCount:
                  minimum: 1
                  maximum: 10
                  type: integer
                activeStandby:
                  type: boolean
                annotations: {}
                placement: {}
                resources: {}
            metadataPool:
              properties:
                failureDomain:
                  type: string
                replicated:
                  properties:
                    size:
                      minimum: 1
                      maximum: 10
                      type: integer
                erasureCoded:
                  properties:
                    dataChunks:
                      type: integer
                    codingChunks:
                      type: integer
            dataPools:
              type: array
              items:
                properties:
                  failureDomain:
                    type: string
                  replicated:
                    properties:
                      size:
                        minimum: 1
                        maximum: 10
                        type: integer
                  erasureCoded:
                    properties:
                      dataChunks:
                        type: integer
                      codingChunks:
                        type: integer
  additionalPrinterColumns:
    - name: ActiveMDS
      type: string
      description: Number of desired active MDS daemons
      JSONPath: .spec.metadataServer.activeCount
    - name: Age
      type: date
      JSONPath: .metadata.creationTimestamp
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephnfses.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephNFS
    listKind: CephNFSList
    plural: cephnfses
    singular: cephnfs
    shortNames:
    - nfs
  scope: Namespaced
  version: v1
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            rados:
              properties:
                pool:
                  type: string
                namespace:
                  type: string
            server:
              properties:
                active:
                  type: integer
                annotations: {}
                placement: {}
                resources: {}
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephobjectstores.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephObjectStore
    listKind: CephObjectStoreList
    plural: cephobjectstores
    singular: cephobjectstore
  scope: Namespaced
  version: v1
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            gateway:
              properties:
                type:
                  type: string
                sslCertificateRef: {}
                port:
                  type: integer
                securePort: {}
                instances:
                  type: integer
                annotations: {}
                placement: {}
                resources: {}
            metadataPool:
              properties:
                failureDomain:
                  type: string
                replicated:
                  properties:
                    size:
                      type: integer
                erasureCoded:
                  properties:
                    dataChunks:
                      type: integer
                    codingChunks:
                      type: integer
            dataPool:
              properties:
                failureDomain:
                  type: string
                replicated:
                  properties:
                    size:
                      type: integer
                erasureCoded:
                  properties:
                    dataChunks:
                      type: integer
                    codingChunks:
                      type: integer
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephobjectstoreusers.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephObjectStoreUser
    listKind: CephObjectStoreUserList
    plural: cephobjectstoreusers
    singular: cephobjectstoreuser
  scope: Namespaced
  version: v1
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephblockpools.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephBlockPool
    listKind: CephBlockPoolList
    plural: cephblockpools
    singular: cephblockpool
  scope: Namespaced
  version: v1
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
  version: v1alpha2
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: objectbuckets.objectbucket.io
spec:
  group: objectbucket.io
  versions:
    - name: v1alpha1
      served: true
      storage: true
  names:
    kind: ObjectBucket
    listKind: ObjectBucketList
    plural: objectbuckets
    singular: objectbucket
    shortNames:
      - ob
      - obs
  scope: Cluster
  subresources:
    status: {}
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: objectbucketclaims.objectbucket.io
spec:
  versions:
    - name: v1alpha1
      served: true
      storage: true
  group: objectbucket.io
  names:
    kind: ObjectBucketClaim
    listKind: ObjectBucketClaimList
    plural: objectbucketclaims
    singular: objectbucketclaim
    shortNames:
      - obc
      - obcs
  scope: Namespaced
  subresources:
    status: {}`
}

// GetRookOperator returns rook Operator manifest
func (m *CephManifestsV1_2) GetRookOperator(namespace string) string {
	return `
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
  - apps
  - extensions
  resources:
  - pods
  - configmaps
  - services
  - daemonsets
  - statefulsets
  - deployments
  verbs:
  - get
  - list
  - watch
  - patch
  - create
  - update
  - delete
- apiGroups:
  - k8s.cni.cncf.io
  resources:
  - network-attachment-definitions
  verbs:
  - get
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-ceph-cluster-mgmt
  labels:
    operator: rook
    storage-backend: ceph
aggregationRule:
  clusterRoleSelectors:
  - matchLabels:
      rbac.ceph.rook.io/aggregate-to-rook-ceph-cluster-mgmt: "true"
rules: []
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-ceph-cluster-mgmt-rules
  labels:
    operator: rook
    storage-backend: ceph
    rbac.ceph.rook.io/aggregate-to-rook-ceph-cluster-mgmt: "true"
rules:
- apiGroups:
  - ""
  resources:
  - secrets
  - pods
  - pods/log
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
  - apps
  - extensions
  resources:
  - deployments
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
  name: rook-ceph-global
  labels:
    operator: rook
    storage-backend: ceph
aggregationRule:
  clusterRoleSelectors:
  - matchLabels:
      rbac.ceph.rook.io/aggregate-to-rook-ceph-global: "true"
rules: []
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-ceph-global-rules
  labels:
    operator: rook
    storage-backend: ceph
    rbac.ceph.rook.io/aggregate-to-rook-ceph-global: "true"
rules:
- apiGroups:
  - ""
  resources:
  # Pod access is needed for fencing
  - pods
  # Node access is needed for determining nodes where mons should run
  - nodes
  - nodes/proxy
  - services
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - events
    # PVs and PVCs are managed by the Rook provisioner
  - persistentvolumes
  - persistentvolumeclaims
  - endpoints
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
- apiGroups:
  - policy
  - apps
  - extensions
  resources:
  # This is for the clusterdisruption controller
  - poddisruptionbudgets
  # This is for both clusterdisruption and nodedrain controllers
  - deployments
  - replicasets
  verbs:
  - "*"
- apiGroups:
  - healthchecking.openshift.io
  resources:
  - machinedisruptionbudgets
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - delete
- apiGroups:
  - machine.openshift.io
  resources:
  - machines
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - delete
- apiGroups:
  - storage.k8s.io
  resources:
  - csidrivers
  verbs:
  - create
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr-cluster
aggregationRule:
  clusterRoleSelectors:
  - matchLabels:
      rbac.ceph.rook.io/aggregate-to-rook-ceph-mgr-cluster: "true"
rules: []
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr-cluster-rules
  labels:
    rbac.ceph.rook.io/aggregate-to-rook-ceph-mgr-cluster: "true"
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
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
  verbs:
  - create
  - patch
  - list
  - get
  - watch
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-osd
rules:
- apiGroups:
  - ""
  resources:
  - nodes
  verbs:
  - get
  - list
---
# Aspects of Rook Ceph Agent that require access to secrets
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-ceph-agent-mount
  labels:
    operator: rook
    storage-backend: ceph
aggregationRule:
  clusterRoleSelectors:
  - matchLabels:
      rbac.ceph.rook.io/aggregate-to-rook-ceph-agent-mount: "true"
rules: []
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-agent-mount-rules
  labels:
    operator: rook
    storage-backend: ceph
    rbac.ceph.rook.io/aggregate-to-rook-ceph-agent-mount: "true"
rules:
- apiGroups:
  - ""
  resources:
  - secrets
  verbs:
  - get
---
# Aspects of ceph-mgr that require access to the system namespace
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr-system
aggregationRule:
  clusterRoleSelectors:
  - matchLabels:
      rbac.ceph.rook.io/aggregate-to-rook-ceph-mgr-system: "true"
rules: []
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr-system-rules
  labels:
    rbac.ceph.rook.io/aggregate-to-rook-ceph-mgr-system: "true"
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
  verbs:
  - get
  - list
  - watch
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-object-bucket
  labels:
    operator: rook
    storage-backend: ceph
    rbac.ceph.rook.io/aggregate-to-rook-ceph-mgr-cluster: "true"
rules:
- apiGroups:
  - ""
  verbs:
  - "*"
  resources:
  - secrets
  - configmaps
- apiGroups:
    - storage.k8s.io
  resources:
    - storageclasses
  verbs:
    - get
    - list
    - watch
- apiGroups:
  - "objectbucket.io"
  verbs:
  - "*"
  resources:
  - "*"
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-object-bucket
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-object-bucket
subjects:
  - kind: ServiceAccount
    name: rook-ceph-system
    namespace: ` + namespace + `
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
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-csi-rbd-plugin-sa
  namespace: ` + namespace + `
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rbd-csi-nodeplugin
aggregationRule:
  clusterRoleSelectors:
  - matchLabels:
      rbac.ceph.rook.io/aggregate-to-rbd-csi-nodeplugin: "true"
rules: []
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rbd-csi-nodeplugin-rules
  labels:
    rbac.ceph.rook.io/aggregate-to-rbd-csi-nodeplugin: "true"
rules:
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "update"]
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list"]
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rbd-csi-nodeplugin
subjects:
  - kind: ServiceAccount
    name: rook-csi-rbd-plugin-sa
    namespace: ` + namespace + `
roleRef:
  kind: ClusterRole
  name: rbd-csi-nodeplugin
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-csi-rbd-provisioner-sa
  namespace: ` + namespace + `
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rbd-external-provisioner-runner
aggregationRule:
  clusterRoleSelectors:
  - matchLabels:
      rbac.ceph.rook.io/aggregate-to-rbd-external-provisioner-runner: "true"
rules: []
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rbd-external-provisioner-runner-rules
  labels:
    rbac.ceph.rook.io/aggregate-to-rbd-external-provisioner-runner: "true"
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete", "update", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["list", "watch", "create", "update", "patch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshots"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotcontents"]
    verbs: ["create", "get", "list", "watch", "update", "delete"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apiextensions.k8s.io"]
    resources: ["customresourcedefinitions"]
    verbs: ["create", "list", "watch", "delete", "get", "update"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshots/status"]
    verbs: ["update"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims/status"]
    verbs: ["update", "patch"]
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rbd-csi-provisioner-role
subjects:
  - kind: ServiceAccount
    name: rook-csi-rbd-provisioner-sa
    namespace: ` + namespace + `
roleRef:
  kind: ClusterRole
  name: rbd-external-provisioner-runner
  apiGroup: rbac.authorization.k8s.io
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  namespace: ` + namespace + `
  name: rbd-external-provisioner-cfg
rules:
  - apiGroups: [""]
    resources: ["endpoints"]
    verbs: ["get", "watch", "list", "delete", "update", "create"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "watch", "create", "delete"]
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "watch", "list", "delete", "update", "create"]
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rbd-csi-provisioner-role-cfg
  namespace: ` + namespace + `
subjects:
  - kind: ServiceAccount
    name: rook-csi-rbd-provisioner-sa
    namespace: ` + namespace + `
roleRef:
  kind: Role
  name: rbd-external-provisioner-cfg
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-csi-cephfs-plugin-sa
  namespace: ` + namespace + `
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: cephfs-csi-nodeplugin
aggregationRule:
  clusterRoleSelectors:
  - matchLabels:
      rbac.ceph.rook.io/aggregate-to-cephfs-csi-nodeplugin: "true"
rules: []
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: cephfs-csi-nodeplugin-rules
  labels:
    rbac.ceph.rook.io/aggregate-to-cephfs-csi-nodeplugin: "true"
rules:
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "update"]
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list"]

---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: cephfs-csi-nodeplugin
subjects:
  - kind: ServiceAccount
    name: rook-csi-cephfs-plugin-sa
    namespace: ` + namespace + `
roleRef:
  kind: ClusterRole
  name: cephfs-csi-nodeplugin
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-csi-cephfs-provisioner-sa
  namespace: ` + namespace + `
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: cephfs-external-provisioner-runner
aggregationRule:
  clusterRoleSelectors:
  - matchLabels:
      rbac.ceph.rook.io/aggregate-to-cephfs-external-provisioner-runner: "true"
rules: []
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: cephfs-external-provisioner-runner-rules
  labels:
    rbac.ceph.rook.io/aggregate-to-cephfs-external-provisioner-runner: "true"
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete", "update", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["list", "watch", "create", "update", "patch"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims/status"]
    verbs: ["update", "patch"]
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: cephfs-csi-provisioner-role
subjects:
  - kind: ServiceAccount
    name: rook-csi-cephfs-provisioner-sa
    namespace: ` + namespace + `
roleRef:
  kind: ClusterRole
  name: cephfs-external-provisioner-runner
  apiGroup: rbac.authorization.k8s.io
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  namespace: ` + namespace + `
  name: cephfs-external-provisioner-cfg
rules:
  - apiGroups: [""]
    resources: ["endpoints"]
    verbs: ["get", "watch", "list", "delete", "update", "create"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "create", "delete"]
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "watch", "list", "delete", "update", "create"]
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: cephfs-csi-provisioner-role-cfg
  namespace: ` + namespace + `
subjects:
  - kind: ServiceAccount
    name: rook-csi-cephfs-provisioner-sa
    namespace: ` + namespace + `
roleRef:
  kind: Role
  name: cephfs-external-provisioner-cfg
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: rook-privileged
spec:
  privileged: true
  allowedCapabilities:
    # required by CSI
    - SYS_ADMIN
  # fsGroup - the flexVolume agent has fsGroup capabilities and could potentially be any group
  fsGroup:
    rule: RunAsAny
  # runAsUser, supplementalGroups - Rook needs to run some pods as root
  # Ceph pods could be run as the Ceph user, but that user isn't always known ahead of time
  runAsUser:
    rule: RunAsAny
  supplementalGroups:
    rule: RunAsAny
  # seLinux - seLinux context is unknown ahead of time; set if this is well-known
  seLinux:
    rule: RunAsAny
  volumes:
    # recommended minimum set
    - configMap
    - downwardAPI
    - emptyDir
    - persistentVolumeClaim
    - secret
    - projected
    # required for Rook
    - hostPath
    - flexVolume
  # allowedHostPaths can be set to Rook's known host volume mount points when they are fully-known
  # directory-based OSDs make this hard to nail down
  # allowedHostPaths:
  #   - /run/udev      # for OSD prep
  #   - /dev           # for OSD prep
  #   - /var/lib/rook  # or whatever the dataDirHostPath value is set to
  # Ceph requires host IPC for setting up encrypted devices
  hostIPC: true
  # Ceph OSDs need to share the same PID namespace
  hostPID: true
  # hostNetwork can be set to 'false' if host networking isn't used
  hostNetwork: true
  hostPorts:
    # Ceph messenger protocol v1
    - min: 6789
      max: 6790 # <- support old default port
    # Ceph messenger protocol v2
    - min: 3300
      max: 3300
    # Ceph RADOS ports for OSDs, MDSes
    - min: 6800
      max: 7300
    # # Ceph dashboard port HTTP (not recommended)
    # - min: 7000
    #   max: 7000
    # Ceph dashboard port HTTPS
    - min: 8443
      max: 8443
    # Ceph mgr Prometheus Metrics
    - min: 9283
      max: 9283
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: 'psp:rook'
rules:
  - apiGroups:
      - policy
    resources:
      - podsecuritypolicies
    resourceNames:
      - rook-privileged
    verbs:
      - use
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: rook-ceph-system-psp
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: 'psp:rook'
subjects:
  - kind: ServiceAccount
    name: rook-ceph-system
    namespace: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: rook-csi-rbd-provisioner-sa-psp
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: 'psp:rook'
subjects:
  - kind: ServiceAccount
    name: rook-csi-rbd-provisioner-sa
    namespace: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: rook-csi-rbd-plugin-sa-psp
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: 'psp:rook'
subjects:
  - kind: ServiceAccount
    name: rook-csi-rbd-plugin-sa
    namespace: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: rook-csi-cephfs-provisioner-sa-psp
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: 'psp:rook'
subjects:
  - kind: ServiceAccount
    name: rook-csi-cephfs-provisioner-sa
    namespace: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: rook-csi-cephfs-plugin-sa-psp
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: 'psp:rook'
subjects:
  - kind: ServiceAccount
    name: rook-csi-cephfs-plugin-sa
    namespace: ` + namespace + `
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rook-ceph-operator
  namespace: ` + namespace + `
  labels:
    operator: rook
    storage-backend: ceph
spec:
  replicas: 1
  selector:
    matchLabels:
      app: rook-ceph-operator
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
        - name: ROOK_CEPH_STATUS_CHECK_INTERVAL
          value: "5s"
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
              fieldPath: metadata.namespace
        - name: ROOK_ENABLE_FLEX_DRIVER
          value: "true"
        - name: ROOK_CURRENT_NAMESPACE_ONLY
          value: "false"
---
kind: ConfigMap
apiVersion: v1
metadata:
  name: rook-ceph-operator-config
  namespace: ` + namespace + `
data:
  ROOK_CSI_ENABLE_CEPHFS: "true"
  ROOK_CSI_ENABLE_RBD: "true"
  ROOK_CSI_ENABLE_GRPC_METRICS: "true"
`
}

// GetClusterRoles returns rook-cluster manifest
func (m *CephManifestsV1_2) GetClusterRoles(namespace, systemNamespace string) string {
	return `apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-osd
  namespace: ` + namespace + `
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-mgr
  namespace: ` + namespace + `
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-osd
  namespace: ` + namespace + `
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: [ "get", "list", "watch", "create", "update", "delete" ]
- apiGroups: ["ceph.rook.io"]
  resources: ["cephclusters", "cephclusters/finalizers"]
  verbs: [ "get", "list", "create", "update", "delete" ]
---
# Aspects of ceph-mgr that operate within the cluster's namespace
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr
  namespace: ` + namespace + `
rules:
- apiGroups:
  - ""
  resources:
  - pods
  - services
  - pods/log
  verbs:
  - get
  - list
  - watch
  - delete
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
---
# Allow the ceph osd to access cluster-wide resources necessary for determining their topology location
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-osd-` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-osd
subjects:
- kind: ServiceAccount
  name: rook-ceph-osd
  namespace: ` + namespace + `
---
# Allow the ceph mgr to access cluster-wide resources necessary for the mgr modules
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr-cluster-` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-mgr-cluster
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: ` + namespace + `
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
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
  - create
  - update
  - delete
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
# Allow the osd pods in this namespace to work with configmaps
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-osd
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-osd
subjects:
- kind: ServiceAccount
  name: rook-ceph-osd
  namespace: ` + namespace + `
---
# Allow the ceph mgr to access the cluster-specific resources necessary for the mgr modules
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-mgr
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: ` + namespace + `
---
# Allow the ceph mgr to access the rook system resources necessary for the mgr modules
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr-system ` + namespace + `
  namespace: ` + systemNamespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-mgr-system
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: ` + namespace + `
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-cmd-reporter
subjects:
- kind: ServiceAccount
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rook-ceph-default-psp
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
- kind: ServiceAccount
  name: default
  namespace: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rook-ceph-osd-psp
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
- kind: ServiceAccount
  name: rook-ceph-osd
  namespace: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rook-ceph-mgr-psp
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rook-ceph-cmd-reporter-psp
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
- kind: ServiceAccount
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
`
}

// GetRookCluster returns rook-cluster manifest
func (m *CephManifestsV1_2) GetRookCluster(settings *clusterSettings) string {
	store := "# storeType not specified; Rook will use default store types"
	if settings.StoreType != "" {
		store = `storeType: "` + settings.StoreType + `"`
	}

	if settings.UsePVCs {
		return `apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: ` + settings.ClusterName + `
  namespace: ` + settings.Namespace + `
spec:
  dataDirHostPath: ` + settings.DataDirHostPath + `
  mon:
    count: ` + strconv.Itoa(settings.Mons) + `
    allowMultiplePerNode: true
    volumeClaimTemplate:
      spec:
        storageClassName: ` + settings.StorageClassName + `
        resources:
          requests:
            storage: 5Gi
  cephVersion:
    image: ` + settings.CephVersion.Image + `
    allowUnsupported: ` + strconv.FormatBool(settings.CephVersion.AllowUnsupported) + `
  skipUpgradeChecks: false
  continueUpgradeAfterChecksEvenIfNotHealthy: false
  dashboard:
    enabled: true
  rbdMirroring:
    workers: ` + strconv.Itoa(settings.RBDMirrorWorkers) + `
  network:
    hostNetwork: false
  crashCollector:
    disable: false
  storage:
    storageClassDeviceSets:
    - name: set1
      count: 1
      portable: false
      tuneSlowDeviceClass: true
      volumeClaimTemplates:
      - metadata:
          name: data
        spec:
          resources:
            requests:
              storage: 10Gi
          storageClassName: ` + settings.StorageClassName + `
          volumeMode: Block
          accessModes:
            - ReadWriteOnce
  disruptionManagement:
    managePodBudgets: false
    osdMaintenanceTimeout: 30
    manageMachineDisruptionBudgets: false
    machineDisruptionBudgetNamespace: openshift-machine-api`
	}

	return `apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: ` + settings.ClusterName + `
  namespace: ` + settings.Namespace + `
spec:
  cephVersion:
    image: ` + settings.CephVersion.Image + `
    allowUnsupported: ` + strconv.FormatBool(settings.CephVersion.AllowUnsupported) + `
  dataDirHostPath: ` + settings.DataDirHostPath + `
  network:
    hostNetwork: false
  mon:
    count: ` + strconv.Itoa(settings.Mons) + `
    allowMultiplePerNode: true
  dashboard:
    enabled: true
  skipUpgradeChecks: true
  metadataDevice:
  storage:
    useAllNodes: true
    useAllDevices: true
    deviceFilter:
    config:
      ` + store + `
      databaseSizeMB: "1024"
      journalSizeMB: "1024"
  mgr:
    modules:
    - name: pg_autoscaler
      enabled: true`
}

// GetRookToolBox returns rook-toolbox manifest
func (m *CephManifestsV1_2) GetRookToolBox(namespace string) string {
	return `apiVersion: v1
kind: Pod
metadata:
  name: rook-ceph-tools
  namespace: ` + namespace + `
spec:
  dnsPolicy: ClusterFirstWithHostNet
  containers:
  - name: rook-ceph-tools
    image: rook/ceph:` + m.imageTag + `
    imagePullPolicy: IfNotPresent
    command: ["/tini"]
    args: ["-g", "--", "/usr/local/bin/toolbox.sh"]
    env:
      - name: ROOK_ADMIN_SECRET
        valueFrom:
          secretKeyRef:
            name: rook-ceph-mon
            key: admin-secret
    volumeMounts:
      - name: mon-endpoint-volume
        mountPath: /etc/rook
  hostNetwork: false
  volumes:
    - name: mon-endpoint-volume
      configMap:
        name: rook-ceph-mon-endpoints
        items:
        - key: data
          path: mon-endpoints`
}

func (m *CephManifestsV1_2) GetBlockPoolDef(poolName string, namespace string, replicaSize string) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: ` + poolName + `
  namespace: ` + namespace + `
spec:
  replicated:
    size: ` + replicaSize + `
    targetSizeRatio: .5
    requireSafeReplicaSize: false
  compressionMode: aggressive`
}

func (m *CephManifestsV1_2) GetBlockStorageClassDef(csi bool, poolName, storageClassName, reclaimPolicy, namespace, systemNamespace string) string {
	// Create a CSI driver storage class
	if csi {
		return `
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ` + storageClassName + `
provisioner: ` + systemNamespace + `.rbd.csi.ceph.com
parameters:
  pool: ` + poolName + `
  clusterID: ` + namespace + `
  csi.storage.k8s.io/provisioner-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/provisioner-secret-namespace: ` + namespace + `
  csi.storage.k8s.io/node-stage-secret-name: rook-csi-rbd-node
  csi.storage.k8s.io/node-stage-secret-namespace: ` + namespace + `
`
	}
	// Create a FLEX driver storage class
	return `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: ` + storageClassName + `
provisioner: ceph.rook.io/block
allowVolumeExpansion: true
reclaimPolicy: ` + reclaimPolicy + `
parameters:
    blockPool: ` + poolName + `
    clusterNamespace: ` + namespace
}

func (m *CephManifestsV1_2) GetFileStorageClassDef(fsName, storageClassName, namespace string) string {
	// Create a CSI driver storage class
	csiCephFSNodeSecret := "rook-csi-cephfs-node"
	csiCephFSProvisionerSecret := "rook-csi-cephfs-provisioner"
	return `
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ` + storageClassName + `
provisioner: ` + SystemNamespace(namespace) + `.cephfs.csi.ceph.com
parameters:
  clusterID: ` + namespace + `
  fsName: ` + fsName + `
  pool: ` + fsName + `-data0
  csi.storage.k8s.io/provisioner-secret-name: ` + csiCephFSProvisionerSecret + `
  csi.storage.k8s.io/provisioner-secret-namespace: ` + namespace + `
  csi.storage.k8s.io/node-stage-secret-name: ` + csiCephFSNodeSecret + `
  csi.storage.k8s.io/node-stage-secret-namespace: ` + namespace + `
`
}

func (m *CephManifestsV1_2) GetBlockPVCDef(claimName, namespace, storageClassName, accessModes, size string) string {
	return `apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ` + claimName + `
  namespace: ` + namespace + `
  annotations:
    volume.beta.kubernetes.io/storage-class: ` + storageClassName + `
spec:
  accessModes:
    - ` + accessModes + `
  resources:
    requests:
      storage: ` + size
}

// GetFilesystem returns the manifest to create a Rook filesystem resource with the given config.
func (m *CephManifestsV1_2) GetFilesystem(namespace, name string, activeCount int) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephFilesystem
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
    activeCount: ` + strconv.Itoa(activeCount) + `
    activeStandby: true`
}

// GetFilesystem returns the manifest to create a Rook Ceph NFS resource with the given config.
func (m *CephManifestsV1_2) GetNFS(namespace, name, pool string, count int) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephNFS
metadata:
  name: ` + name + `
  namespace: ` + namespace + `
spec:
  rados:
    pool: ` + pool + `
    namespace: nfs-ns
  server:
    active: ` + strconv.Itoa(count)
}

func (m *CephManifestsV1_2) GetObjectStore(namespace, name string, replicaCount, port int) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephObjectStore
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

func (m *CephManifestsV1_2) GetObjectStoreUser(namespace, name string, displayName string, store string) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: ` + name + `
  namespace: ` + namespace + `
spec:
  displayName: ` + displayName + `
  store: ` + store
}

//GetBucketStorageClass returns the manifest to create object bucket
func (m *CephManifestsV1_2) GetBucketStorageClass(storeNameSpace string, storeName string, storageClassName string, reclaimPolicy string, region string) string {
	return `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: ` + storageClassName + `
provisioner: ceph.rook.io/bucket
reclaimPolicy: ` + reclaimPolicy + `
parameters:
    objectStoreName: ` + storeName + `
    objectStoreNamespace: ` + storeNameSpace + `
    region: ` + region
}

//GetObc returns the manifest to create object bucket claim
func (m *CephManifestsV1_2) GetObc(claimName string, storageClassName string, objectBucketName string, varBucketName bool) string {
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
  storageClassName: ` + storageClassName
}

func (m *CephManifestsV1_2) GetClient(namespace string, claimName string, caps map[string]string) string {
	clientCaps := []string{}
	for name, cap := range caps {
		str := name + ": " + cap
		clientCaps = append(clientCaps, str)
	}
	return `apiVersion: ceph.rook.io/v1
kind: CephClient
metadata:
  name: ` + claimName + `
  namespace: ` + namespace + `
spec:
  caps:
    ` + strings.Join(clientCaps, "\n    ")
}

func (m *CephManifestsV1_2) GetClusterExternalRoles(namespace, firstClusterNamespace string) string {
	return `apiVersion: v1
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
  namespace: ` + firstClusterNamespace + `
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-cmd-reporter
subjects:
- kind: ServiceAccount
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
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
  - create
  - update
  - delete`
}

func (m *CephManifestsV1_2) GetRookExternalCluster(settings *clusterExternalSettings) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: ` + settings.Namespace + `
  namespace: ` + settings.Namespace + `
spec:
  external:
    enable: true
  dataDirHostPath: ` + settings.DataDirHostPath + ``
}

func (m *CephManifestsV1_2) GetRBDMirror(namespace, name string, count int) string {
	// Not implemented in v1.2
	return ""
}
