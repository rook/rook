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

import (
	"strconv"
)

type MinioManifests struct {
}

func (i *MinioManifests) GetMinioCRDs() string {
	return `apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: objectstores.minio.rook.io
spec:
  group: minio.rook.io
  names:
    kind: ObjectStore
    listKind: ObjectStoreList
    plural: objectstores
    singular: objectstore
  scope: Namespaced
  version: v1alpha1
`
}

func (i *MinioManifests) GetMinioOperator(namespace string) string {
	return `apiVersion: v1
kind: Namespace
metadata:
  name: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-minio-operator
rules:
- apiGroups:
  - ""
  resources:
  - namespaces
  - secrets
  - pods
  - services
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
  - minio.rook.io
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
  name: rook-minio-operator
  namespace: ` + namespace + `
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-minio-operator
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-minio-operator
subjects:
- kind: ServiceAccount
  name: rook-minio-operator
  namespace: ` + namespace + `
---
apiVersion: apps/v1beta1
kind: Deployment
metadata:
  name: rook-minio-operator
  namespace: ` + namespace + `
spec:
  replicas: 1
  template:
    metadata:
      labels:
        app: rook-minio-operator
    spec:
      serviceAccountName: rook-minio-operator
      containers:
      - name: rook-minio-operator
        image: rook/minio:master
        args: ["minio", "operator"]
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

func (i *MinioManifests) GetMinioCluster(namespace string, count int) string {
	return `apiVersion: v1
kind: Secret
metadata:
  name: access-keys
  namespace: ` + namespace + `
type: Opaque
data:
  # Base64 encoded string: "TEMP_DEMO_ACCESS_KEY"
  username: VEVNUF9ERU1PX0FDQ0VTU19LRVk=
  # Base64 encoded string: "TEMP_DEMO_SECRET_KEY"
  password: VEVNUF9ERU1PX1NFQ1JFVF9LRVk=
---
apiVersion: minio.rook.io/v1alpha1
kind: ObjectStore
metadata:
  name: my-store
  namespace: ` + namespace + `
spec:
  scope:
    nodeCount: ` + strconv.Itoa(count) + `
  port: 9000
  credentials:
    name: access-keys
    namespace: ` + namespace + `
  storageAmount: "10G"
---
apiVersion: v1
kind: Service
metadata:
  name: minio-my-store
  namespace: ` + namespace + `
spec:
  type: NodePort
  ports:
    - port: 9000
  selector:
    app: minio
`
}
