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
	version1_3 = "master" // v1.3 is not yet released, so test upgrade to master until then
)

// CephManifestsV1_3 wraps rook yaml definitions for Rook-Ceph v1.3 manifests
type CephManifestsV1_3 struct {
	K8sh              *utils.K8sHelper
	Namespace         string
	SystemNamespace   string
	OperatorContainer string
	T                 func() *testing.T
}

// RookImage returns the rook image under test for v1.2
func (m *CephManifestsV1_3) RookImage() string {
	return fmt.Sprintf("rook/ceph:%s", version1_3)
}

// UpgradeToV1_3 performs the steps necessary to upgrade a Rook v1.1 cluster to v1.2. It does not
// verify the upgrade but merely starts the upgrade process.
func (m *CephManifestsV1_3) UpgradeToV1_3() {
	require.NoError(m.T(), m.K8sh.ResourceOperation("apply", m.applyManifest()))

	require.NoError(m.T(),
		m.K8sh.SetDeploymentVersion(m.SystemNamespace, m.OperatorContainer, m.OperatorContainer, version1_3))
}

func (m *CephManifestsV1_3) applyManifest() string {
	return `
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
   name: cephfs-external-provisioner-runner-rules
   labels:
      rbac.ceph.rook.io/aggregate-to-cephfs-external-provisioner-runner: "true"
rules:
   - apiGroups: [""]
     resources: ["secrets"]
     verbs: ["get", "list"]
   - apiGroups: [""]
     resources: ["persistentvolumes"]
     verbs: ["get", "list", "watch", "create", "delete", "update", "patch"]
   - apiGroups: [""]
     resources: ["persistentvolumeclaims"]
     verbs: ["get", "list", "watch", "update"]
   - apiGroups: ["storage.k8s.io"]
     resources: ["storageclasses"]
     verbs: ["get", "list", "watch"]
   - apiGroups: [""]
     resources: ["events"]
     verbs: ["list", "watch", "create", "update", "patch"]
   - apiGroups: ["storage.k8s.io"]
     resources: ["volumeattachments"]
     verbs: ["get", "list", "watch", "update", "patch"]
   - apiGroups: [""]
     resources: ["nodes"]
     verbs: ["get", "list", "watch"]
   - apiGroups: [""]
     resources: ["persistentvolumeclaims/status"]
     verbs: ["update", "patch"]
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
   name: rbd-external-provisioner-runner-rules
   labels:
      rbac.ceph.rook.io/aggregate-to-rbd-external-provisioner-runner: "true"
rules:
   - apiGroups: [""]
     resources: ["secrets"]
     verbs: ["get", "list"]
   - apiGroups: [""]
     resources: ["persistentvolumes"]
     verbs: ["get", "list", "watch", "create", "delete", "update", "patch"]
   - apiGroups: [""]
     resources: ["persistentvolumeclaims"]
     verbs: ["get", "list", "watch", "update"]
   - apiGroups: ["storage.k8s.io"]
     resources: ["volumeattachments"]
     verbs: ["get", "list", "watch", "update", "patch"]
   - apiGroups: [""]
     resources: ["nodes"]
     verbs: ["get", "list", "watch"]
   - apiGroups: ["storage.k8s.io"]
     resources: ["storageclasses"]
     verbs: ["get", "list", "watch"]
   - apiGroups: [""]
     resources: ["events"]
     verbs: ["list", "watch", "create", "update", "patch"]
   - apiGroups: ["snapshot.storage.k8s.io"]
     resources: ["volumesnapshots"]
     verbs: ["get", "list", "watch", "update"]
   - apiGroups: ["snapshot.storage.k8s.io"]
     resources: ["volumesnapshotcontents"]
     verbs: ["create", "get", "list", "watch", "update", "delete"]
   - apiGroups: ["snapshot.storage.k8s.io"]
     resources: ["volumesnapshotclasses"]
     verbs: ["get", "list", "watch"]
   - apiGroups: ["apiextensions.k8s.io"]
     resources: ["customresourcedefinitions"]
     verbs: ["create", "list", "watch", "delete", "get", "update"]
   - apiGroups: ["snapshot.storage.k8s.io"]
     resources: ["volumesnapshots/status"]
     verbs: ["update"]
   - apiGroups: [""]
     resources: ["persistentvolumeclaims/status"]
     verbs: ["update", "patch"]
`
}
