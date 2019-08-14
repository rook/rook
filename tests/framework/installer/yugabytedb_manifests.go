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

import "strconv"

type YugabyteDBManifests struct {
}

func (_ *YugabyteDBManifests) GetYugabyteDBOperatorSpecs(namespace string) string {
	return `apiVersion: v1
kind: Namespace
metadata:
  name: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-yugabytedb-operator
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
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - apps
  resources:
  - statefulsets
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - yugabytedb.rook.io
  resources:
  - "*"
  verbs:
  - "*"
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-yugabytedb-operator
  namespace: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRoleBinding
metadata:
  name: rook-yugabytedb-operator
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-yugabytedb-operator
subjects:
- kind: ServiceAccount
  name: rook-yugabytedb-operator
  namespace: ` + namespace + `
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rook-yugabytedb-operator
  namespace: ` + namespace + `
  labels:
    app: rook-yugabytedb-operator
spec:
  selector:
    matchLabels:
      app: rook-yugabytedb-operator
  replicas: 1
  template:
    metadata:
      labels:
        app: rook-yugabytedb-operator
    spec:
      serviceAccountName: rook-yugabytedb-operator
      containers:
      - name: rook-yugabytedb-operator
        image: samkulkarni20/rook-yugabytedb:latest
        args: ["yugabytedb", "operator"]
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace`
}

func (_ *YugabyteDBManifests) GetYugabyteDBCRDSpecs() string {
	return `apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: ybclusters.yugabytedb.rook.io
spec:
  group: yugabytedb.rook.io
  names:
    kind: YBCluster
    listKind: YBClusterList
    singular: ybcluster
    plural: ybclusters
  scope: Namespaced
  version: v1alpha1`
}

func (_ *YugabyteDBManifests) GetYugabyteDBClusterSpecs(namespace string, replicaCount int) string {
	return `apiVersion: yugabytedb.rook.io/v1alpha1
kind: YBCluster
metadata:
  name: rook-yugabytedb
  namespace: ` + namespace + `
spec:
  master:
    replicas: ` + strconv.Itoa(replicaCount) + `
    network:
      ports:
        - name: yb-master-ui
          port: 7000
        - name: yb-master-rpc
          port: 7100
    volumeClaimTemplate:
      metadata:
        name: datadir
      spec:
        accessModes: [ "ReadWriteOnce" ]
        resources:
          requests:
            storage: 10Mi
  tserver:
    replicas: ` + strconv.Itoa(replicaCount) + `
    network:
      ports:
        - name: yb-tserver-ui
          port: 9000
        - name: yb-tserver-rpc
          port: 9100
        - name: ycql
          port: 9042
        - name: yedis
          port: 6379
        - name: ysql
          port: 5433
    volumeClaimTemplate:
      metadata:
        name: datadir
      spec:
        accessModes: [ "ReadWriteOnce" ]
        resources:
          requests:
            storage: 10Mi`
}
