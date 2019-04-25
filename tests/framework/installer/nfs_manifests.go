/*
Copyright 2018 The Rook Authors. All rights reserved.

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

type NFSManifests struct {
}

// GetNFSServerCRDs returns NFSServer CRD definition
func (i *NFSManifests) GetNFSServerCRDs() string {
	return `apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: nfsservers.nfs.rook.io
spec:
  group: nfs.rook.io
  names:
    kind: NFSServer
    listKind: NFSServerList
    plural: nfsservers
    singular: nfsserver
  scope: Namespaced
  version: v1alpha1
`
}

// GetNFSServerOperator returns the NFSServer operator definition
func (i *NFSManifests) GetNFSServerOperator(namespace string) string {
	return `apiVersion: v1
kind: Namespace
metadata:
  name:  ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-nfs-operator
rules:
- apiGroups:
  - "*"
  resources:
  - namespaces
  - configmaps
  - pods
  - services
  - storageclasses
  verbs:
  - get
  - watch
  - create
- apiGroups:
  - apps
  resources:
  - statefulsets
  verbs:
  - get
  - create
- apiGroups:
  - nfs.rook.io
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
  name: rook-nfs-operator
  namespace: ` + namespace + `
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-nfs-operator
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-nfs-operator
subjects:
- kind: ServiceAccount
  name: rook-nfs-operator
  namespace: ` + namespace + `
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rook-nfs-operator
  namespace: ` + namespace + `
  labels:
    app: rook-nfs-operator
spec:
  selector:
    matchLabels:
      app: rook-nfs-operator
  replicas: 1
  template:
    metadata:
      labels:
        app: rook-nfs-operator
    spec:
      serviceAccountName: rook-nfs-operator
      containers:
      - name: rook-nfs-operator
        imagePullPolicy: IfNotPresent
        image: rook/nfs:master
        args: ["nfs", "operator"]
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-nfs-provisioner-runner
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create", "update", "patch"]
  - apiGroups: [""]
    resources: ["services", "endpoints"]
    verbs: ["get"]
  - apiGroups: ["extensions"]
    resources: ["podsecuritypolicies"]
    resourceNames: ["nfs-provisioner"]
    verbs: ["use"]
  - apiGroups: [""]
    resources: ["endpoints"]
    verbs: ["get", "list", "watch", "create", "update", "patch"]
  - apiGroups:
    - nfs.rook.io
    resources:
    - "*"
    verbs:
    - "*"
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: run-nfs-provisioner
subjects:
  - kind: ServiceAccount
    name: rook-nfs-provisioner
     # replace with namespace where provisioner is deployed
    namespace: ` + namespace + `
roleRef:
  kind: ClusterRole
  name: rook-nfs-provisioner-runner
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-nfs-provisioner
  namespace: ` + namespace + `
---
kind: Deployment
apiVersion: apps/v1
metadata:
  name: rook-nfs-provisioner
  namespace: ` + namespace + `
spec:
  selector:
    matchLabels:
      app: rook-nfs-provisioner
  replicas: 1
  strategy:
    type: Recreate 
  template:
    metadata:
      labels:
        app: rook-nfs-provisioner
    spec:
      serviceAccount: rook-nfs-provisioner
      containers:
      - name: rook-nfs-provisioner
        image: rook/nfs:master
        imagePullPolicy: IfNotPresent
        args: ["nfs", "provisioner","--provisioner=rook.io/nfs-provisioner"]
        env:
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

// GetNFSServerPV returns NFSServer PV definition
func (i *NFSManifests) GetNFSServerPV(namespace string, clusterIP string) string {
	return `apiVersion: v1
kind: PersistentVolume
metadata:
  name: nfs-pv
  namespace: ` + namespace + `
  annotations:
    volume.beta.kubernetes.io/mount-options: "vers=4.1"
spec:
  storageClassName: nfs-sc
  capacity:
    storage: 1Mi
  accessModes:
    - ReadWriteMany
  nfs:
    server: ` + clusterIP + `
    path: "/test-claim"
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: nfs-pv1
  namespace: ` + namespace + `
  annotations:
    volume.beta.kubernetes.io/mount-options: "vers=4.1"
spec:
  storageClassName: nfs-sc
  capacity:
    storage: 2Mi
  accessModes:
    - ReadWriteMany
  nfs:
    server: ` + clusterIP + `
    path: "/test-claim1"
`
}

// GetNFSServerPVC returns NFSServer PVC definition
func (i *NFSManifests) GetNFSServerPVC(namespace string) string {
	return `
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  labels:
    app: rook-nfs
  name: nfs-ns-nfs-share
parameters:
  exportName: nfs-share
  nfsServerName: ` + namespace + `
  nfsServerNamespace: ` + namespace + `
provisioner: rook.io/nfs-provisioner
reclaimPolicy: Delete
volumeBindingMode: Immediate
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  labels:
    app: rook-nfs
  name: nfs-ns-nfs-share1
parameters:
  exportName: nfs-share1
  nfsServerName: ` + namespace + `
  nfsServerNamespace: ` + namespace + `
provisioner: rook.io/nfs-provisioner
reclaimPolicy: Delete
volumeBindingMode: Immediate
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: nfs-pv-claim
spec:
  storageClassName: nfs-ns-nfs-share
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 1Mi
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: nfs-pv-claim-bigger
spec:
  storageClassName: nfs-ns-nfs-share1
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 2Mi
`
}

// GetNFSServer returns NFSServer CRD instance definition
func (i *NFSManifests) GetNFSServer(namespace string, count int, storageClassName string) string {
	return `
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: test-claim
  namespace: ` + namespace + `
spec:
  storageClassName: ` + storageClassName + `
  accessModes:
  - ReadWriteMany
  resources:
    requests:
      storage: 1Mi
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: test-claim1
  namespace: ` + namespace + `
spec:
  storageClassName: ` + storageClassName + `
  accessModes:
  - ReadWriteMany
  resources:
    requests:
      storage: 2Mi
---
apiVersion: nfs.rook.io/v1alpha1
kind: NFSServer
metadata:
  name: ` + namespace + `
  namespace: ` + namespace + `
spec:
  serviceAccountName: ` + namespace + `
  replicas: ` + strconv.Itoa(count) + `
  exports:
  - name: nfs-share
    server:
      accessMode: ReadWrite
      squash: "none"
    persistentVolumeClaim:
      claimName: test-claim
  - name: nfs-share1
    server:
      accessMode: ReadWrite
      squash: "none"
    persistentVolumeClaim:
      claimName: test-claim1
`
}
