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
	"fmt"
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
)

type CassandraManifests struct{}

func (i *CassandraManifests) GetCassandraCRDs() string {
	return `apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: clusters.cassandra.rook.io
spec:
  group: cassandra.rook.io
  names:
    kind: Cluster
    listKind: ClusterList
    plural: clusters
    singular: cluster
  scope: Namespaced
  version: v1alpha1`
}

func (i *CassandraManifests) GetCassandraOperator(namespace string) string {

	return fmt.Sprintf(`
# Namespace where Cassandra Operator will live
apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s

---
# ClusterRole for cassandra-operator.
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: rook-cassandra-operator
rules:
  - apiGroups:
    - ""
    resources:
      - pods
    verbs:
      - get
      - list
      - watch
      - delete
  - apiGroups:
      - ""
    resources:
      - services
      - serviceaccounts
    verbs:
      - get
      - list
      - watch
      - create
  - apiGroups:
      - ""
    resources:
      - persistentvolumes
      - persistentvolumeclaims
    verbs:
      - get
      - delete
  - apiGroups:
      - ""
    resources:
      - nodes
    verbs:
      - get
  - apiGroups:
      - apps
    resources:
      - statefulsets
    verbs:
      - "*"
  - apiGroups:
      - policy
    resources:
      - poddisruptionbudgets
    verbs:
      - create
  - apiGroups:
      - cassandra.rook.io
    resources:
      - "*"
    verbs:
      - "*"
  - apiGroups:
      - ""
    resources:
      - events
    verbs:
      - create
      - update
      - patch
---
# ServiceAccount for cassandra-operator. Serves as its authorization identity.
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-cassandra-operator
  namespace: %[1]s
---
# Bind cassandra-operator ServiceAccount with ClusterRole.
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-cassandra-operator
  namespace: %[1]s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-cassandra-operator
subjects:
- kind: ServiceAccount
  name: rook-cassandra-operator
  namespace: %[1]s
---
# cassandra-operator StatefulSet.
 apiVersion: apps/v1
 kind: StatefulSet
 metadata:
   name: rook-cassandra-operator
   namespace: %[1]s
 spec:
   replicas: 1
   serviceName: "non-existent-service"
   selector:
     matchLabels:
       app: rook-cassandra-operator
   template:
     metadata:
       labels:
         app: rook-cassandra-operator
     spec:
       serviceAccountName: rook-cassandra-operator
       containers:
       - name: rook-cassandra-operator
         image: rook/cassandra:master
         imagePullPolicy: "IfNotPresent"
         args: ["cassandra", "operator"]
         env:
         - name: POD_NAME
           valueFrom:
             fieldRef:
               fieldPath: metadata.name
         - name: POD_NAMESPACE
           valueFrom:
             fieldRef:
               fieldPath: metadata.namespace
`, namespace)
}

func (i *CassandraManifests) GetCassandraCluster(namespace string, count int, mode cassandrav1alpha1.ClusterMode) string {

	var version string
	if mode == cassandrav1alpha1.ClusterModeScylla {
		version = "2.3.0"
	} else {
		version = "3.11.1"
	}
	return fmt.Sprintf(`
# Namespace for cassandra cluster
apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s

---

# Role for cassandra members.
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: %[1]s-member
  namespace: %[1]s
rules:
  - apiGroups:
      - ""
    resources:
      - pods
    verbs:
      - get
  - apiGroups:
      - ""
    resources:
      - services
    verbs:
      - get
      - list
  - apiGroups:
      - cassandra.rook.io
    resources:
      - clusters
    verbs:
      - get

---

# ServiceAccount for cassandra members.
apiVersion: v1
kind: ServiceAccount
metadata:
  name: %[1]s-member
  namespace: %[1]s

---

# RoleBinding for cassandra members.
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: %[1]s-member
  namespace: %[1]s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: %[1]s-member
subjects:
- kind: ServiceAccount
  name: %[1]s-member
  namespace: %[1]s

---

# Cassandra Cluster
apiVersion: cassandra.rook.io/v1alpha1
kind: Cluster
metadata:
  name: %[1]s
  namespace: %[1]s
spec:
  version: %[4]s
  mode: %[3]s
  datacenter:
    name: "us-east-1"
    racks:
      - name: "us-east-1a"
        members: %[2]d
        storage:
          volumeClaimTemplates:
                - metadata:
                    name: %[1]s-data
                  spec:
                    resources:
                      requests:
                        storage: 5Gi
        resources:
          requests:
            cpu: 1
            memory: 2Gi
          limits:
            cpu: 1
            memory: 2Gi
`, namespace, count, mode, version)
}
