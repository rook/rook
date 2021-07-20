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
	"strings"

	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
)

type CassandraManifests struct{}

func (i *CassandraManifests) GetCassandraCRDs() string {
	manifest := readManifest("cassandra", "crds.yaml")
	return manifest
}

func (i *CassandraManifests) GetCassandraOperator(namespace string) string {
	manifest := readManifest("cassandra", "operator.yaml")
	manifest = strings.ReplaceAll(manifest, "rook-cassandra-system # namespace:operator", namespace)

	return manifest
}

func (i *CassandraManifests) GetCassandraCluster(namespace string, count int, mode cassandrav1alpha1.ClusterMode) string {

	var version string
	if mode == cassandrav1alpha1.ClusterModeScylla {
		version = "2.3.0"
	} else {
		version = "3.11.6"
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
      - patch
      - watch
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
