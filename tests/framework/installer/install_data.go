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
)

//InstallData wraps rook yaml definitions
type InstallData struct {
}

//NewK8sInstallData creates new instance of InstallData struct
func NewK8sInstallData() *InstallData {
	return &InstallData{}
}

func (i *InstallData) GetRookCRDs(namespace string) string {
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
  version: v1alpha1
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
  version: v1alpha1
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
  version: v1alpha1
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
  version: v1alpha1
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
func (i *InstallData) GetRookOperator(namespace string) string {

	return `kind: Namespace
apiVersion: v1
metadata:
  name: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-ceph-operator
rules:
- apiGroups:
  - ""
  resources:
  - namespaces
  - serviceaccounts
  - secrets
  - pods
  - services
  - nodes
  - nodes/proxy
  - configmaps
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
- apiGroups:
  - rbac.authorization.k8s.io
  resources:
  - clusterroles
  - clusterrolebindings
  - roles
  - rolebindings
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
  - storageclasses
  verbs:
  - get
  - list
  - watch
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
  name: rook-ceph-operator
  namespace: ` + namespace + `
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-operator
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-operator
subjects:
- kind: ServiceAccount
  name: rook-ceph-operator
  namespace: ` + namespace + `
---
apiVersion: apps/v1beta1
kind: Deployment
metadata:
  name: rook-ceph-operator
  namespace: ` + namespace + `
spec:
  replicas: 1
  template:
    metadata:
      labels:
        app: rook-ceph-operator
    spec:
      serviceAccountName: rook-ceph-operator
      containers:
      - name: rook-ceph-operator
        image: rook/ceph:master
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
func (i *InstallData) GetRookCluster(namespace, storeType, dataDirHostPath string, useAllDevices bool, mons int) string {
	return `apiVersion: ceph.rook.io/v1alpha1
kind: Cluster
metadata:
  name: ` + namespace + `
  namespace: ` + namespace + `
spec:
  dataDirHostPath: ` + dataDirHostPath + `
  network:
    hostNetwork: false
  mon:
    count: ` + strconv.Itoa(mons) + `
    allowMultiplePerNode: true
  dashboard:
    enabled: true
  metadataDevice:
  storage:
    useAllNodes: true
    useAllDevices: ` + strconv.FormatBool(useAllDevices) + `
    deviceFilter:
    location:
    config:
      storeType: "` + storeType + `"
      databaseSizeMB: "1024"
      journalSizeMB: "1024"`
}

//GetRookToolBox returns rook-toolbox manifest
func (i *InstallData) GetRookToolBox(namespace string) string {
	return `apiVersion: v1
kind: Pod
metadata:
  name: rook-ceph-tools
  namespace: ` + namespace + `
spec:
  dnsPolicy: ClusterFirstWithHostNet
  containers:
  - name: rook-ceph-tools
    image: rook/ceph-toolbox:master
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
