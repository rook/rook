/*
Copyright 2019 The Rook Authors. All rights reserved.

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

type EdgefsManifests struct{}

func (i *EdgefsManifests) GetEdgefsCRDs() string {
	return `apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: clusters.edgefs.rook.io
spec:
  group: edgefs.rook.io
  names:
    kind: Cluster
    listKind: ClusterList
    plural: clusters
    singular: cluster
  scope: Namespaced
  version: v1
`
}

func (i *EdgefsManifests) GetEdgefsOperator(namespace string) string {
	return `apiVersion: v1
kind: Namespace
metadata:
  name: ` + namespace + `
---
# The cluster role for managing all the cluster-specific resources in a namespace
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-edgefs-cluster-mgmt
  labels:
    operator: rook
    storage-backend: edgefs
rules:
- apiGroups: [""]
  resources: ["secrets", "pods", "nodes", "services", "configmaps", "endpoints"]
  verbs: ["get", "list", "watch", "patch", "create", "update", "delete"]
- apiGroups: ["apps"]
  resources: ["statefulsets", "statefulsets/scale"]
  verbs: ["create", "delete", "deletecollection", "patch", "update"]
- apiGroups: ["apps"]
  resources: ["deployments", "daemonsets", "replicasets", "statefulsets"]
  verbs: ["get", "list", "watch", "create", "update", "delete"]
---
# The role for the operator to manage resources in the system namespace
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: Role
metadata:
  name: rook-edgefs-system
  namespace: ` + namespace + `
  labels:
    operator: rook
    storage-backend: edgefs
rules:
- apiGroups: [""]
  resources: ["pods", "nodes", "configmaps"]
  verbs: ["get", "list", "watch", "patch", "create", "update", "delete"]
- apiGroups: ["apps"]
  resources: ["daemonsets"]
  verbs: ["get", "list", "watch", "create", "update", "delete"]
---
# The cluster role for managing the Rook CRDs
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-edgefs-global
  labels:
    operator: rook
    storage-backend: edgefs
rules:
- apiGroups: [""]
  # Pod access is needed for fencing
  # Node access is needed for determining nodes where mons should run
  resources: ["pods", "nodes", "nodes/proxy"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: [""]
  # PVs and PVCs are managed by the Rook provisioner
  resources: ["events", "persistentvolumes", "persistentvolumeclaims"]
  verbs: ["get", "list", "watch", "patch", "create", "update", "delete"]
- apiGroups: ["storage.k8s.io"]
  resources: ["storageclasses"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["get", "list", "watch", "create", "update", "delete"]
- apiGroups: ["edgefs.rook.io"]
  resources: ["*"]
  verbs: ["*"]
- apiGroups: ["rook.io"]
  resources: ["*"]
  verbs: ["*"]
---
# The rook system service account used by the operator, agent, and discovery pods
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-edgefs-system
  namespace: ` + namespace + `
  labels:
    operator: rook
    storage-backend: edgefs
---
# Grant the operator, agent, and discovery agents access to resources in the rook-ceph-system namespace
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-edgefs-system
  namespace: ` + namespace + `
  labels:
    operator: rook
    storage-backend: edgefs
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-edgefs-system
subjects:
- kind: ServiceAccount
  name: rook-edgefs-system
  namespace: ` + namespace + `
---
# Grant the rook system daemons cluster-wide access to manage the Rook CRDs, PVCs, and storage classes
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-edgefs-global
  namespace: ` + namespace + `
  labels:
    operator: rook
    storage-backend: edgefs
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-edgefs-global
subjects:
- kind: ServiceAccount
  name: rook-edgefs-system
  namespace: ` + namespace + `
---
# The deployment for the rook operator
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rook-edgefs-operator
  namespace: ` + namespace + `
  labels:
    operator: rook
    storage-backend: edgefs
spec:
  replicas: 1
  selector:
    matchLabels:
      app: rook-edgefs-operator
  template:
    metadata:
      labels:
        app: rook-edgefs-operator
    spec:
      serviceAccountName: rook-edgefs-system
      containers:
      - name: rook-edgefs-operator
        image: edgefs/edgefs-operator:v1 #TODO: should be replaced by rook/edgefs:1.1 after 1.1 release
        imagePullPolicy: "Always"
        args: ["edgefs", "operator"]
        env:
        - name: ROOK_LOG_LEVEL
          value: "INFO"
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
`
}

func (i *EdgefsManifests) GetEdgefsCluster(namespace string) string {
	return `apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-edgefs-cluster
  namespace: ` + namespace + `
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-edgefs-cluster
  namespace: ` + namespace + `
rules:
- apiGroups: [""]
  resources: ["configmaps", "endpoints"]
  verbs: [ "get", "list", "watch", "create", "update", "delete" ]
- apiGroups: ["edgefs.rook.io"]
  resources: ["*"]
  verbs: ["*"]
- apiGroups: [""]
  resources: ["pods"]
  verbs: [ "get", "list" ]
---
# Allow the operator to create resources in this cluster's namespace
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-edgefs-cluster-mgmt
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-edgefs-cluster-mgmt
subjects:
- kind: ServiceAccount
  name: rook-edgefs-system
  namespace: rook-edgefs-system
---
# Allow the pods in this namespace to work with configmaps
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-edgefs-cluster
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-edgefs-cluster
subjects:
- kind: ServiceAccount
  name: rook-edgefs-cluster
  namespace: ` + namespace + `
---
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: privileged
spec:
  fsGroup:
    rule: RunAsAny
  privileged: true
  runAsUser:
    rule: RunAsAny
  seLinux:
    rule: RunAsAny
  supplementalGroups:
    rule: RunAsAny
  volumes:
  - '*'
  allowedCapabilities:
  - '*'
  hostPID: true
  hostIPC: true
  hostNetwork: false
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: privileged-psp-user
rules:
- apiGroups:
  - apps
  resources:
  - podsecuritypolicies
  resourceNames:
  - privileged
  verbs:
  - use
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: rook-edgefs-system-psp
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: privileged-psp-user
subjects:
- kind: ServiceAccount
  name: rook-edgefs-system
  namespace: rook-edgefs-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: rook-edgefs-cluster-psp
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: privileged-psp-user
subjects:
- kind: ServiceAccount
  name: rook-edgefs-cluster
  namespace: ` + namespace + `
---
apiVersion: edgefs.rook.io/v1
kind: Cluster
metadata:
  name: rook-edgefs
  namespace: ` + namespace + `
spec:
  edgefsImageName: edgefs/edgefs:latest   # specify version here, i.e. edgefs/edgefs:1.0.0 etc
  serviceAccount: rook-edgefs-cluster
  skipHostPrepare: true
  dataDirHostPath: /var/lib/edgefs
  storage: # cluster level storage configuration and selection
    useAllNodes: true
    directories:
    - path: /mnt/disks/ssd0
    - path: /mnt/disks/ssd1
    - path: /mnt/disks/ssd2
`
}
