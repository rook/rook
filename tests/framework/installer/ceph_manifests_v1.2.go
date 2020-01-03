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

import (
	"fmt"
	"testing"

	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
)

const (
	version1_2 = "v1.2.1"
)

// CephManifestsV1_2 wraps rook yaml definitions for Rook-Ceph v1.2 manifests
type CephManifestsV1_2 struct {
	K8sh              *utils.K8sHelper
	Namespace         string
	SystemNamespace   string
	OperatorContainer string
	T                 func() *testing.T
}

// UpgradeToV1_2 performs the steps necessary to upgrade a Rook v1.1 cluster to v1.2. It does not
// verify the upgrade but merely starts the upgrade process.
func (m *CephManifestsV1_2) UpgradeToV1_2() {
	require.NoError(m.T(), m.K8sh.ResourceOperation("apply", m.applyManifest()))

	require.NoError(m.T(),
		m.K8sh.SetDeploymentVersion(m.SystemNamespace, m.OperatorContainer, m.OperatorContainer, version1_2))
}

// RookImage returns the rook image under test for v1.2
func (m *CephManifestsV1_2) RookImage() string {
	return fmt.Sprintf("rook/ceph:%s", version1_2)
}

func (m *CephManifestsV1_2) applyManifest() string {
	return `
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-ceph-global-rules
  labels:
    operator: rook
    storage-backend: ceph
    rbac.ceph.rook.io/aggregate-to-rook-ceph-global: "true"
rules:
- apiGroups:
  - ""
  resources:
  # Pod access is needed for fencing
  - pods
  # Node access is needed for determining nodes where mons should run
  - nodes
  - nodes/proxy
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - events
    # PVs and PVCs are managed by the Rook provisioner
  - persistentvolumes
  - persistentvolumeclaims
  - endpoints
  verbs:
  - get
  - list
  - watch
  - patch
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
- apiGroups:
  - batch
  resources:
  - jobs
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - delete
- apiGroups:
  - ceph.rook.io
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
- apiGroups:
  - policy
  - apps
  resources:
  # This is for the clusterdisruption controller
  - poddisruptionbudgets
  # This is for both clusterdisruption and nodedrain controllers
  - deployments
  - replicasets
  verbs:
  - "*"
- apiGroups:
  - healthchecking.openshift.io
  resources:
  - machinedisruptionbudgets
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - delete
- apiGroups:
  - machine.openshift.io
  resources:
  - machines
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
  - csidrivers
  verbs:
  - create
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephclients.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephClient
    listKind: CephClientList
    plural: cephclients
    singular: cephclient
  scope: Namespaced
  version: v1
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            caps:
              type: object
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-osd
  namespace: ` + m.Namespace + `
rules:
- apiGroups:
  - ""
  resources:
  - nodes
  verbs:
  - get
  - list
---
# Allow the ceph osd to access cluster-wide resources necessary for determining their topology location
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-osd
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-osd
subjects:
- kind: ServiceAccount
  name: rook-ceph-osd
  namespace: ` + m.Namespace + `
`
}
