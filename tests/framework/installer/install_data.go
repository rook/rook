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

//InstallData wraps rook yaml definitions
type InstallData struct {
}

//NewK8sInstallData creates new instance of InstallData struct
func NewK8sInstallData() *InstallData {
	return &InstallData{}
}

//GetRookOperator returns rook Operator  manifest
func (i *InstallData) GetRookOperator(k8sVersion, namespace string) string {

	return `kind: Namespace
apiVersion: v1
metadata:
  name: ` + namespace + `
---
kind: ClusterRole
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
  namespace: ` + namespace + `
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-operator
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-operator
subjects:
- kind: ServiceAccount
  name: rook-operator
  namespace: ` + namespace + `
---
apiVersion: apps/v1beta1
kind: Deployment
metadata:
  name: rook-operator
  namespace: ` + namespace + `
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
          value: rook
        - name: ROOK_LOG_LEVEL
          value: INFO
        # The interval to check if every mon is in the quorum.
        - name: ROOK_MON_HEALTHCHECK_INTERVAL
          value: "20s"
        # The duration to wait before trying to failover or remove/replace the
        # current mon with a new mon (useful for compensating flapping network).
        - name: ROOK_MON_OUT_TIMEOUT
          value: "300s"
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: ROOK_OPERATOR_SERVICE_ACCOUNT
          valueFrom:
            fieldRef:
              fieldPath: spec.serviceAccountName
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
func (i *InstallData) GetRookCluster(namespace, storeType string) string {
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
  hostNetwork: false
  storage:
    useAllNodes: true
    useAllDevices: false
    deviceFilter:
    metadataDevice:
    location:
    storeConfig:
      storeType: ` + storeType + `
      databaseSizeMB: 1024
      journalSizeMB: 1024
`
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
