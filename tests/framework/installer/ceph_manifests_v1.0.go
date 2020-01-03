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

const (
	// Version tag for Rook v1.0
	Version1_0 = "v1.0.6"
)

// CephManifestsV1_0 wraps rook yaml definitions for Rook-Ceph v1.0 manifests
type CephManifestsV1_0 struct {
	imageTag string
}

// RookImage returns the rook image under test for v1.0
func (m *CephManifestsV1_0) RookImage() string {
	return fmt.Sprintf("rook/ceph:%s", Version1_0)
}

func (m *CephManifestsV1_0) GetRookCRDs() string {
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
            cephVersion:
              properties:
                allowUnsupported:
                  type: boolean
                image:
                  type: string
                name:
                  pattern: ^(luminous|mimic|nautilus)$
                  type: string
            dashboard:
              properties:
                enabled:
                  type: boolean
                urlPrefix:
                  type: string
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
              required:
              - count
            network:
              properties:
                hostNetwork:
                  type: boolean
            storage:
              properties:
                nodes:
                  items: {}
                  type: array
                useAllDevices: {}
                useAllNodes:
                  type: boolean
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
  additionalPrinterColumns:
    - name: MdsCount
      type: string
      description: Number of MDSs
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
  version: v1alpha2`
}

// GetRookOperator returns rook Operator manifest
func (m *CephManifestsV1_0) GetRookOperator(namespace string) string {
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
  - services
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
  resources:
  - daemonsets
  - statefulsets
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
  namespace: ` + namespace + `
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
  namespace: ` + namespace + `
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
# Allow the ceph mgr to access cluster-wide resources necessary for the mgr modules
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr-cluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-mgr-cluster
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: ` + namespace + `
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: csi-rbd-config
  namespace: ` + namespace + `
data:
  csi-rbdplugin-provisioner.yaml: |-` + rbdProvisionerTemplate + `
  csi-rbdplugin.yaml: |-` + rbdPluginTemplate + `
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: csi-cephfs-config
  namespace: ` + namespace + `
data:
  csi-cephfsplugin-provisioner.yaml: |` + cephfsProvisionerTemplate + `
  csi-cephfsplugin.yaml: |` + cephfsPluginTemplate + `
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
    verbs: ["get", "list", "watch", "create", "delete", "update"]
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
    resources: ["endpoints"]
    verbs: ["get", "create", "update"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshots"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "create", "delete"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotcontents"]
    verbs: ["create", "get", "list", "watch", "update", "delete"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apiextensions.k8s.io"]
    resources: ["customresourcedefinitions"]
    verbs: ["create"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list", "watch", "update"]

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
    verbs: ["get", "list", "watch", "create", "delete", "update"]
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
    resources: ["configmaps"]
    verbs: ["get", "list", "create", "delete"]

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
        # CSI enablement
        - name: ROOK_CSI_ENABLE_CEPHFS
          value: "true"
        - name: ROOK_CSI_CEPHFS_IMAGE
          value: "quay.io/cephcsi/cephfsplugin:v1.0.0"
        - name: ROOK_CSI_ENABLE_RBD
          value: "true"
        - name: ROOK_CSI_RBD_IMAGE
          value: "quay.io/cephcsi/rbdplugin:v1.0.0"
        - name: ROOK_CSI_REGISTRAR_IMAGE
          value: "quay.io/k8scsi/csi-node-driver-registrar:v1.0.2"
        - name: ROOK_CSI_PROVISIONER_IMAGE
          value: "quay.io/k8scsi/csi-provisioner:v1.0.1"
        - name: ROOK_CSI_SNAPSHOTTER_IMAGE
          value: "quay.io/k8scsi/csi-snapshotter:v1.0.1"
        - name: ROOK_CSI_ATTACHER_IMAGE
          value: "quay.io/k8scsi/csi-attacher:v1.0.1"
        volumeMounts:
        - mountPath: /etc/ceph-csi/rbd
          name: csi-rbd-config
        - mountPath: /etc/ceph-csi/cephfs
          name: csi-cephfs-config
      volumes:
      - name: csi-rbd-config
        configMap:
          name: csi-rbd-config
      - name: csi-cephfs-config
        configMap:
          name: csi-cephfs-config
`
}

// GetClusterRoles returns rook-cluster manifest
func (m *CephManifestsV1_0) GetClusterRoles(namespace, systemNamespace string) string {
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
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-osd
  namespace: ` + namespace + `
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: [ "get", "list", "watch", "create", "update", "delete" ]
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
`
}

// GetRookCluster returns rook-cluster manifest
func (m *CephManifestsV1_0) GetRookCluster(settings *ClusterSettings) string {
	store := "# storeType not specified; Rook will use default store types"
	if settings.StoreType != "" {
		store = `storeType: "` + settings.StoreType + `"`
	}

	// disable dashboard of the upgrade test, because upgrading Ceph from Mimic to Nautilus with Rook
	// v1.0 can stall for a LONG time on updating the mgr module
	return `apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: ` + settings.Namespace + `
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
    enabled: false
  rbdMirroring:
    workers: ` + strconv.Itoa(settings.RBDMirrorWorkers) + `
  metadataDevice:
  storage:
    useAllNodes: true
    useAllDevices: ` + strconv.FormatBool(settings.UseAllDevices) + `
    directories:
    - path: ` + settings.DataDirHostPath + /* simulate legacy fallback osd behavior so existing tests still work */ `
    deviceFilter:
    location:
    config:
      ` + store + `
      databaseSizeMB: "1024"
      journalSizeMB: "1024"`
}

// GetRookToolBox returns rook-toolbox manifest
func (m *CephManifestsV1_0) GetRookToolBox(namespace string) string {
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

// GetCleanupPod gets a cleanup Pod manifest
func (m *CephManifestsV1_0) GetCleanupPod(node, removalDir string) string {
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

func (m *CephManifestsV1_0) GetBlockPoolDef(poolName string, namespace string, replicaSize string) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: ` + poolName + `
  namespace: ` + namespace + `
spec:
  replicated:
    size: ` + replicaSize
}

func (m *CephManifestsV1_0) GetBlockStorageClassDef(poolName string, storageClassName string, reclaimPolicy string, namespace string, varClusterName bool) string {
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
    blockPool: ` + poolName + `
    ` + namespaceParameter + `: ` + namespace
}

func (m *CephManifestsV1_0) GetBlockPvcDef(claimName, storageClassName, accessModes, size string) string {
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
      storage: ` + size
}

func (m *CephManifestsV1_0) GetBlockPoolStorageClassAndPvcDef(namespace string, poolName string, storageClassName string, reclaimPolicy string, blockName string, accessMode string) string {
	return concatYaml(m.GetBlockPoolDef(poolName, namespace, "1"),
		concatYaml(m.GetBlockStorageClassDef(poolName, storageClassName, reclaimPolicy, namespace, false), m.GetBlockPvcDef(blockName, storageClassName, accessMode, "1M")))
}

func (m *CephManifestsV1_0) GetBlockPoolStorageClass(namespace string, poolName string, storageClassName string, reclaimPolicy string) string {
	return concatYaml(m.GetBlockPoolDef(poolName, namespace, "1"), m.GetBlockStorageClassDef(poolName, storageClassName, reclaimPolicy, namespace, false))
}

// GetFilesystem returns the manifest to create a Rook filesystem resource with the given config.
func (m *CephManifestsV1_0) GetFilesystem(namespace, name string, activeCount int) string {
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
func (m *CephManifestsV1_0) GetNFS(namespace, name, pool string, count int) string {
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

func (m *CephManifestsV1_0) GetObjectStore(namespace, name string, replicaCount, port int) string {
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

func (m *CephManifestsV1_0) GetObjectStoreUser(namespace, name string, displayName string, store string) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: ` + name + `
  namespace: ` + namespace + `
spec:
  displayName: ` + displayName + `
  store: ` + store
}

func (m *CephManifestsV1_0) GetBucketStorageClass(namespace string, storeName string, storageClassName string, reclaimPolicy string, region string) string {
	panic("upgrade test not supported for object bucket claim")
}

func (m *CephManifestsV1_0) GetObc(claimName string, storageClassName string, objectBucketName string, varBucketName bool) string {
	panic("upgrade test not supported for object bucket claim")
}

func (m *CephManifestsV1_0) GetClient(claimName string, namespace string, caps map[string]string) string {
	panic("upgrade test not supported for client")
}
