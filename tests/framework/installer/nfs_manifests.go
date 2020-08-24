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
  additionalPrinterColumns:
  - JSONPath: .status.state
    description: NFS Server instance state
    name: State
    type: string
  group: nfs.rook.io
  names:
    kind: NFSServer
    listKind: NFSServerList
    plural: nfsservers
    singular: nfsserver
  scope: Namespaced
  subresources:
    status: {}
  validation:
    openAPIV3Schema:
      description: NFSServer is the Schema for the nfsservers API
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          description: NFSServerSpec defines the desired state of NFSServer
          properties:
            annotations:
              additionalProperties:
                type: string
              description: The annotations-related configuration to add/set on each
                Pod related object.
              type: object
            exports:
              description: The parameters to configure the NFS export
              items:
                description: ExportsSpec represents the spec of NFS exports
                properties:
                  name:
                    description: Name of the export
                    type: string
                  persistentVolumeClaim:
                    description: PVC from which the NFS daemon gets storage for sharing
                    properties:
                      claimName:
                        description: 'ClaimName is the name of a PersistentVolumeClaim
                          in the same namespace as the pod using this volume. More
                          info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims'
                        type: string
                      readOnly:
                        description: Will force the ReadOnly setting in VolumeMounts.
                          Default false.
                        type: boolean
                    required:
                    - claimName
                    type: object
                  server:
                    description: The NFS server configuration
                    properties:
                      accessMode:
                        description: Reading and Writing permissions on the export
                          Valid values are "ReadOnly", "ReadWrite" and "none"
                        enum:
                        - ReadOnly
                        - ReadWrite
                        - none
                        type: string
                      allowedClients:
                        description: The clients allowed to access the NFS export
                        items:
                          description: AllowedClientsSpec represents the client specs
                            for accessing the NFS export
                          properties:
                            accessMode:
                              description: Reading and Writing permissions for the
                                client to access the NFS export Valid values are "ReadOnly",
                                "ReadWrite" and "none" Gets overridden when ServerSpec.accessMode
                                is specified
                              enum:
                              - ReadOnly
                              - ReadWrite
                              - none
                              type: string
                            clients:
                              description: The clients that can access the share Values
                                can be hostname, ip address, netgroup, CIDR network
                                address, or all
                              items:
                                type: string
                              type: array
                            name:
                              description: Name of the clients group
                              type: string
                            squash:
                              description: Squash options for clients Valid values
                                are "none", "rootid", "root", and "all" Gets overridden
                                when ServerSpec.squash is specified
                              enum:
                              - none
                              - rootid
                              - root
                              - all
                              type: string
                          type: object
                        type: array
                      squash:
                        description: This prevents the root users connected remotely
                          from having root privileges Valid values are "none", "rootid",
                          "root", and "all"
                        enum:
                        - none
                        - rootid
                        - root
                        - all
                        type: string
                    type: object
                type: object
              type: array
            replicas:
              description: Replicas of the NFS daemon
              type: integer
          type: object
        status:
          description: NFSServerStatus defines the observed state of NFSServer
          properties:
            message:
              type: string
            reason:
              type: string
            state:
              type: string
          type: object
      type: object
  version: v1alpha1
  versions:
  - name: v1alpha1
    served: true
    storage: true
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`
}

// GetNFSServerOperator returns the NFSServer operator definition
func (i *NFSManifests) GetNFSServerOperator(namespace string) string {
	return `apiVersion: v1
kind: Namespace
metadata:
  name:  ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: rook-nfs-operator
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
  verbs:
  - create
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - create
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - pods
  verbs:
  - list
  - get
  - watch
  - create
- apiGroups:
  - ""
  resources:
  - services
  verbs:
  - create
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apps
  resources:
  - statefulsets
  verbs:
  - create
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - nfs.rook.io
  resources:
  - nfsservers
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - nfs.rook.io
  resources:
  - nfsservers/status
  verbs:
  - get
  - patch
  - update
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-nfs-operator
  namespace: ` + namespace + `
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-nfs-operator
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
provisioner: nfs.rook.io/` + namespace + `-provisioner
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
provisioner: nfs.rook.io/` + namespace + `-provisioner
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
kind: ServiceAccount
metadata:
  name: rook-nfs-server
  namespace: ` + namespace + `
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
    name: rook-nfs-server
    namespace: ` + namespace + `
roleRef:
  kind: ClusterRole
  name: rook-nfs-provisioner-runner
  apiGroup: rbac.authorization.k8s.io
---
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
