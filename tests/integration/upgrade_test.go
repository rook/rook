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

package integration

import (
	"fmt"
	"strings"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ************************************************
// *** Major scenarios tested by the UpgradeSuite ***
// Setup
// - Initially create a cluster from the previous minor release
// - Upgrade to the current build of Rook to verify functionality after upgrade
// - Test basic usage of block, object, and file after upgrade
// Monitors
// - One mon in the cluster
// ************************************************
func TestUpgradeSuite(t *testing.T) {
	s := new(UpgradeSuite)
	defer func(s *UpgradeSuite) {
		HandlePanics(recover(), s.op, s.T)
	}(s)
	suite.Run(t, s)
}

type UpgradeSuite struct {
	suite.Suite
	helper    *clients.TestClient
	op        *TestCluster
	k8sh      *utils.K8sHelper
	namespace string
}

func (s *UpgradeSuite) SetupSuite() {
	s.namespace = "upgrade-ns"
	useDevices := true
	mons := 3
	rbdMirrorWorkers := 0
	s.op, s.k8sh = StartTestCluster(s.T, s.namespace, "bluestore", false, useDevices, mons, rbdMirrorWorkers, installer.Version0_8, installer.LuminousVersion)
	s.helper = clients.CreateTestClient(s.k8sh, s.op.installer.Manifests)
}

func (s *UpgradeSuite) TearDownSuite() {
	s.op.Teardown()
}

func (s *UpgradeSuite) TestUpgradeToMaster() {
	systemNamespace := installer.SystemNamespace(s.namespace)

	// Create block, object, and file storage on 0.8 before the upgrade
	poolName := "upgradepool"
	storageClassName := "block-upgrade"
	blockName := "block-claim-upgrade"
	podName := "test-pod-upgrade"
	logger.Infof("Initializing block before the upgrade")
	setupBlockLite(s.helper, s.k8sh, s.Suite, s.namespace, poolName, storageClassName, blockName, podName)
	createPodWithBlock(s.helper, s.k8sh, s.Suite, s.namespace, blockName, podName)
	defer blockTestDataCleanUp(s.helper, s.k8sh, s.namespace, poolName, storageClassName, blockName, podName)

	logger.Infof("Initializing file before the upgrade")
	filesystemName := "upgrade-test-fs"
	createFilesystem(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)
	createFilesystemConsumerPod(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)
	defer func() {
		cleanupFilesystemConsumer(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName, filePodName)
		cleanupFilesystem(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)
	}()

	logger.Infof("Initializing object before the upgrade")
	objectStoreName := "upgraded-object"
	runObjectE2ETestLite(s.helper, s.k8sh, s.Suite, s.namespace, objectStoreName, 1)

	// verify that we're actually running 0.8 before the upgrade
	operatorContainer := "rook-ceph-operator"
	version, err := k8sutil.GetDeploymentImage(s.k8sh.Clientset, systemNamespace, operatorContainer, operatorContainer)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), "rook/ceph:v0.8.3", version)

	message := "my simple message"
	preFilename := "pre-upgrade-file"
	assert.Nil(s.T(), s.k8sh.WriteToPod("", podName, preFilename, message))
	assert.Nil(s.T(), s.k8sh.ReadFromPod("", podName, preFilename, message))
	assert.Nil(s.T(), s.k8sh.WriteToPod(s.namespace, filePodName, preFilename, message))
	assert.Nil(s.T(), s.k8sh.ReadFromPod(s.namespace, filePodName, preFilename, message))

	// Update the 0.9 ClusterRole before the operator is restarted
	err = s.updateClusterRoles()
	require.Nil(s.T(), err)

	// Upgrade to master
	require.Nil(s.T(), s.k8sh.SetDeploymentVersion(systemNamespace, operatorContainer, operatorContainer, installer.VersionMaster))

	// verify that the operator spec is updated
	version, err = k8sutil.GetDeploymentImage(s.k8sh.Clientset, systemNamespace, operatorContainer, operatorContainer)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), "rook/ceph:"+installer.VersionMaster, version)

	// wait for the legacy mon replicasets to be deleted
	err = s.waitForLegacyMonReplicaSetDeletion()
	require.Nil(s.T(), err)

	// wait for the mon pods to be running
	err = k8sutil.WaitForDeploymentImage(s.k8sh.Clientset, s.namespace, "app=rook-ceph-mon", "mon", cephv1.DefaultLuminousImage)
	require.Nil(s.T(), err)

	err = s.k8sh.WaitForLabeledPodsToRun("app=rook-ceph-mon", s.namespace)
	require.Nil(s.T(), err)

	// wait for the osd pods to be updated
	err = k8sutil.WaitForDeploymentImage(s.k8sh.Clientset, s.namespace, "app=rook-ceph-osd", "osd", cephv1.DefaultLuminousImage)
	require.Nil(s.T(), err)

	err = s.k8sh.WaitForLabeledPodsToRun("app=rook-ceph-osd", s.namespace)
	require.Nil(s.T(), err)
	logger.Infof("Done with automatic upgrade to master")

	// Give a few seconds for the daemons to settle down after the upgrade
	time.Sleep(5 * time.Second)

	// Test writing and reading from the pod with cephfs mounted that was created before the upgrade.
	postFilename := "post-upgrade-file"
	assert.Nil(s.T(), s.k8sh.ReadFromPodRetry(s.namespace, filePodName, preFilename, message, 5))
	assert.Nil(s.T(), s.k8sh.WriteToPodRetry(s.namespace, filePodName, postFilename, message, 5))
	assert.Nil(s.T(), s.k8sh.ReadFromPodRetry(s.namespace, filePodName, postFilename, message, 5))

	// Test writing and reading from the pod with rbd mounted that was created before the upgrade.
	// There is some unreliability right after the upgrade when there is only one osd, so we will retry if needed
	assert.Nil(s.T(), s.k8sh.ReadFromPodRetry("", podName, preFilename, message, 5))
	assert.Nil(s.T(), s.k8sh.WriteToPodRetry("", podName, postFilename, message, 5))
	assert.Nil(s.T(), s.k8sh.ReadFromPodRetry("", podName, postFilename, message, 5))
}

// Update the clusterroles that have been modified in master from the previous release
func (s *UpgradeSuite) updateClusterRoles() error {
	if _, err := s.k8sh.DeleteResource("ClusterRole", "rook-ceph-cluster-mgmt"); err != nil {
		return err
	}
	if _, err := s.k8sh.DeleteResource("-n", s.namespace, "serviceaccount", "rook-ceph-cluster"); err != nil {
		return err
	}
	if _, err := s.k8sh.DeleteResource("-n", s.namespace, "rolebinding", "rook-ceph-cluster"); err != nil {
		return err
	}
	if _, err := s.k8sh.DeleteResource("-n", s.namespace, "role", "rook-ceph-cluster"); err != nil {
		return err
	}

	// create the v1 crds and other new resources suchas service accounts, cluster roles, etc needed for 0.9
	newResources := `
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephclusters.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephCluster
    listKind: CephClusterList
    plural: cephclusters
    singular: cephcluster
  scope: Namespaced
  version: v1
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            cephVersion:
              properties:
                allowUnsupported:
                  type: boolean
                image:
                  type: string
                name:
                  pattern: ^(luminous|mimic|nautilus)$
                  type: string
            dashboard:
              properties:
                enabled:
                  type: boolean
                urlPrefix:
                  type: string
            dataDirHostPath:
              pattern: ^/(\S+)
              type: string
            mon:
              properties:
                allowMultiplePerNode:
                  type: boolean
                count:
                  maximum: 9
                  minimum: 1
                  type: integer
              required:
              - count
            network:
              properties:
                hostNetwork:
                  type: boolean
            storage:
              properties:
                nodes:
                  items: {}
                  type: array
                useAllDevices: {}
                useAllNodes:
                  type: boolean
          required:
          - mon
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephfilesystems.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephFilesystem
    listKind: CephFilesystemList
    plural: cephfilesystems
    singular: cephfilesystem
  scope: Namespaced
  version: v1
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephobjectstores.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephObjectStore
    listKind: CephObjectStoreList
    plural: cephobjectstores
    singular: cephobjectstore
  scope: Namespaced
  version: v1
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephobjectstoreusers.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephObjectStoreUser
    listKind: CephObjectStoreUserList
    plural: cephobjectstoreusers
    singular: cephobjectstoreuser
  scope: Namespaced
  version: v1
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephblockpools.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephBlockPool
    listKind: CephBlockPoolList
    plural: cephblockpools
    singular: cephblockpool
  scope: Namespaced
  version: v1
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: objectstoreusers.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: ObjectStoreUser
    listKind: ObjectStoreUserList
    plural: objectstoreusers
    singular: objectstoreuser
  scope: Namespaced
  version: v1
---
# The cluster role for managing all the cluster-specific resources in a namespace
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-ceph-cluster-mgmt
  labels:
    operator: rook
    storage-backend: ceph
rules:
- apiGroups:
  - ""
  resources:
  - secrets
  - pods
  - pods/log
  - services
  - configmaps
  verbs:
  - get
  - list
  - watch
  - patch
  - create
  - update
  - delete
- apiGroups:
  - extensions
  resources:
  - deployments
  - daemonsets
  - replicasets
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - delete
---
# Aspects of ceph-mgr that require cluster-wide access
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr-cluster
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
  - nodes
  - nodes/proxy
  verbs:
  - get
  - list
  - watch
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-osd
  namespace: ` + s.namespace + `
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-mgr
  namespace: ` + s.namespace + `
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-osd
  namespace: ` + s.namespace + `
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: [ "get", "list", "watch", "create", "update", "delete" ]
---
# Aspects of ceph-mgr that require access to the system namespace
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr-system
  namespace: ` + s.namespace + `
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
  verbs:
  - get
  - list
  - watch
---
# Aspects of ceph-mgr that operate within the cluster's namespace
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr
  namespace: ` + s.namespace + `
rules:
- apiGroups:
  - ""
  resources:
  - pods
  - services
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
---
# Allow the osd pods in this namespace to work with configmaps
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-osd
  namespace: ` + s.namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-osd
subjects:
- kind: ServiceAccount
  name: rook-ceph-osd
  namespace: ` + s.namespace + `
---
# Allow the ceph mgr to access the cluster-specific resources necessary for the mgr modules
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr
  namespace: ` + s.namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-mgr
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: ` + s.namespace + `
---
# Allow the ceph mgr to access the rook system resources necessary for the mgr modules
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr-system
  namespace: ` + installer.SystemNamespace(s.namespace) + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-mgr-system
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: ` + s.namespace + `
---
# Allow the ceph mgr to access cluster-wide resources necessary for the mgr modules
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr-cluster
  namespace: ` + s.namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-mgr-cluster
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: ` + s.namespace + `
`

	logger.Infof("creating the new resources that have been added since 0.8")
	_, err := s.k8sh.ResourceOperation("create", newResources)
	return err
}

func (s *UpgradeSuite) waitForLegacyMonReplicaSetDeletion() error {
	// Wait for the legacy mon replicasets to be deleted during the upgrade
	sleepTime := 3
	attempts := 30
	for i := 0; i < attempts; i++ {
		rs, err := s.k8sh.Clientset.ExtensionsV1beta1().ReplicaSets(s.namespace).List(metav1.ListOptions{LabelSelector: "app=rook-ceph-mon"})
		if err != nil {
			return fmt.Errorf("failed to list mon replicasets. %v", err)
		}

		matches := 0
		for _, r := range rs.Items {
			// a legacy mon replicaset will have two dashes (rook-ceph-mon0) and a new mon replicaset will have four (rook-ceph-mon -a-66d5468994)
			if strings.Count(r.Name, "-") == 2 {
				matches++
			}
		}

		if matches == 0 {
			logger.Infof("all %d replicasets were deleted", len(rs.Items))
			return nil
		}

		logger.Infof("%d legacy mon replicasets still exist", matches)
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}

	return fmt.Errorf("legacy replicasets were not deleted")
}
