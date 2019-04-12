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

// CockroachDBManifests holds the funcs which return the CockroachDB manifests
type CockroachDBManifests struct {
}

// GetCockroachDBCRDs return the CockroachDB Cluster CRD
func (i *CockroachDBManifests) GetCockroachDBCRDs() string {
	return `apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: clusters.cockroachdb.rook.io
spec:
  group: cockroachdb.rook.io
  names:
    kind: Cluster
    listKind: ClusterList
    plural: clusters
    singular: cluster
  scope: Namespaced
  version: v1alpha1
`
}

// GetCockroachDBOperator return the CockroachDB operator manifest
func (i *CockroachDBManifests) GetCockroachDBOperator(namespace string) string {
	return `kind: Namespace
apiVersion: v1
metadata:
  name: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-cockroachdb-operator
rules:
- apiGroups:
  - ""
  resources:
  - pods
  verbs:
  - get
  - list
- apiGroups:
  - ""
  resources:
  - services
  verbs:
  - create
- apiGroups:
  - apps
  resources:
  - statefulsets
  verbs:
  - create
- apiGroups:
  - policy
  resources:
  - poddisruptionbudgets
  verbs:
  - create
- apiGroups:
  - cockroachdb.rook.io
  resources:
  - "*"
  verbs:
  - "*"
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-cockroachdb-operator
  namespace: ` + namespace + `
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-cockroachdb-operator
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-cockroachdb-operator
subjects:
- kind: ServiceAccount
  name: rook-cockroachdb-operator
  namespace: ` + namespace + `
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rook-cockroachdb-operator
  namespace: ` + namespace + `
  labels:
    app: rook-cockroachdb-operator
spec:
  selector:
    matchLabels:
      app: rook-cockroachdb-operator
  replicas: 1
  template:
    metadata:
      labels:
        app: rook-cockroachdb-operator
    spec:
      serviceAccountName: rook-cockroachdb-operator
      containers:
      - name: rook-cockroachdb-operator
        image: rook/cockroachdb:master
        args: ["cockroachdb", "operator"]
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

// GetCockroachDBCluster return a CockroacDB Cluster object
func (i *CockroachDBManifests) GetCockroachDBCluster(namespace string, count int) string {
	return `apiVersion: cockroachdb.rook.io/v1alpha1
kind: Cluster
metadata:
  name: ` + namespace + `
  namespace: ` + namespace + `
spec:
  scope:
    nodeCount: ` + strconv.Itoa(count) + `
    volumeClaimTemplates: []
  secure: false
  cachePercent: 25
  maxSQLMemoryPercent: 25
`
}
