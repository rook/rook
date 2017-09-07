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

import "strings"

//InstallData wraps rook yaml definitions
type InstallData struct {
}

//NewK8sInstallData creates new instance of InstallData struct
func NewK8sInstallData() *InstallData {
	return &InstallData{}
}

//GetRookOperator returns rook Operator  manifest
func (i *InstallData) GetRookOperator(k8sVersion string) string {

	if strings.Contains(k8sVersion, "v1.5") {
		return `apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: rook-operator
spec:
  replicas: 1
  template:
    metadata:
      labels:
        name: rook-operator
    spec:
      containers:
      - name: rook-operator
        image: rook/rook:master
        args: ["operator", "--mon-healthcheck-interval=5s", "--mon-out-timeout=1s"]
        env:
        - name: ROOK_REPO_PREFIX
          value: roo`
	}

	return `kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-operator
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
  - thirdpartyresources
  - deployments
  - daemonsets
  - replicasets
  verbs:
  - get
  - list
  - watch
  - create
  - delete
- apiGroups:
  - apiextensions.k8s.io
  resources:
  - customresourcedefinitions
  verbs:
  - get
  - list
  - watch
  - create
  - delete
- apiGroups:
  - rbac.authorization.k8s.io
  resources:
  - clusterroles
  - clusterrolebindings
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
  - rook.io
  resources:
  - "*"
  verbs:
  - "*"
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-operator
  namespace: default
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-operator
  namespace: default
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-operator
subjects:
- kind: ServiceAccount
  name: rook-operator
  namespace: default
---
apiVersion: apps/v1beta1
kind: Deployment
metadata:
  name: rook-operator
  namespace: default
spec:
  replicas: 1
  template:
    metadata:
      labels:
        app: rook-operator
    spec:
      serviceAccountName: rook-operator
      containers:
      - name: rook-operator
        image: rook/rook:master
        args: ["operator", "--mon-healthcheck-interval=5s", "--mon-out-timeout=1s"]
        env:
        - name: ROOK_REPO_PREFIX
          value: rook`

}

//GetRookCluster returns rook-cluster manifest
func (i *InstallData) GetRookCluster(namespace string) string {
	return `apiVersion: v1
kind: Namespace
metadata:
  name: ` + namespace + `
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: ` + namespace + `
  namespace: ` + namespace + `
spec:
  versionTag: master
  dataDirHostPath:
# To control where various services will be scheduled by kubernetes, use the placement configuration sections below.
# The example under 'all' would have all services scheduled on kubernetes nodes labeled with 'role=storage' and
# tolerate taints with a key of 'storage-node'.
#  placement:
#    all:
#      nodeAffinity:
#        requiredDuringSchedulingIgnoredDuringExecution:
#          nodeSelectorTerms:
#          - matchExpressions:
#            - key: role
#              operator: In
#              values:
#              - storage-node
#      tolerations:
#      - key: storage-node
#        operator: Exists
#    api:
#      nodeAffinity:
#      tolerations:
#    mds:
#      nodeAffinity:
#      tolerations:
#    mon:
#      nodeAffinity:
#      tolerations:
#    osd:
#      nodeAffinity:
#      tolerations:
#    rgw:
#      nodeAffinity:
#      tolerations:
  storage:                # cluster level storage configuration and selection
    useAllNodes: true
    useAllDevices: false
    deviceFilter:
    metadataDevice:
    location:
    storeConfig:
      storeType: filestore
      databaseSizeMB: 1024 # this value can be removed for environments with normal sized disks (100 GB or larger)
      journalSizeMB: 1024  # this value can be removed for environments with normal sized disks (20 GB or larger)
# Individual nodes and their config can be specified as well, but 'useAllNodes' above must be set to false. Then, only the named
# nodes below will be used as storage resources.  Each node's 'name' field should match their 'kubernetes.io/hostname' label.
#    nodes:
#    - name: "172.17.4.101"
#      directories:         # specific directores to use for storage can be specified for each node
#      - path: "/rook/storage-dir"
#    - name: "172.17.4.201"
#      devices:             # specific devices to use for storage can be specified for each node
#      - name: "sdb"
#      - name: "sdc"
#      storeConfig:         # configuration can be specified at the node level which overrides the cluster level config
#        storeType: bluestore
#    - name: "172.17.4.301"
#      deviceFilter: "^sd."`
}

//GetRookToolBox returns rook-toolbox manifest
func (i *InstallData) GetRookToolBox(namespace string) string {
	return `apiVersion: v1
kind: Pod
metadata:
  name: rook-tools
  namespace: ` + namespace + `
spec:
  containers:
  - name: rook-tools
    image: rook/toolbox:master
    imagePullPolicy: IfNotPresent
    args: ["sleep", "36500d"]
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
  volumes:
    - name: dev
      hostPath:
        path: /dev
    - name: sysbus
      hostPath:
        path: /sys/bus
    - name: libmodules
      hostPath:
        path: /lib/modules`
}
