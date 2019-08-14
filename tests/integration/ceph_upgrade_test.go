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
	"testing"
	"time"

	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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
func TestCephUpgradeSuite(t *testing.T) {
	if installer.SkipTestSuite(installer.CephTestSuite) {
		t.Skip()
	}

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
	s.op, s.k8sh = StartTestCluster(s.T, s.namespace, "bluestore", false, useDevices, mons, rbdMirrorWorkers, installer.Version1_0, installer.MimicVersion)
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
	setupBlockLite(s.helper, s.k8sh, s.Suite, s.namespace, poolName, storageClassName, blockName, podName, s.op.installer.CephVersion)
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
	assert.Equal(s.T(), "rook/ceph:"+installer.Version1_0, version)

	message := "my simple message"
	preFilename := "pre-upgrade-file"
	assert.Nil(s.T(), s.k8sh.WriteToPod("", podName, preFilename, message))
	assert.Nil(s.T(), s.k8sh.ReadFromPod("", podName, preFilename, message))
	assert.Nil(s.T(), s.k8sh.WriteToPod(s.namespace, filePodName, preFilename, message))
	assert.Nil(s.T(), s.k8sh.ReadFromPod(s.namespace, filePodName, preFilename, message))

	// Update to the next version of cluster roles before the operator is restarted
	err = s.updateClusterRoles()
	require.Nil(s.T(), err)

	// Upgrade to master
	require.Nil(s.T(), s.k8sh.SetDeploymentVersion(systemNamespace, operatorContainer, operatorContainer, installer.VersionMaster))

	// verify that the operator spec is updated
	version, err = k8sutil.GetDeploymentImage(s.k8sh.Clientset, systemNamespace, operatorContainer, operatorContainer)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), "rook/ceph:"+installer.VersionMaster, version)

	// wait for the mon pods to be running. we can no longer check a version in the pod spec, so we just wait a bit.
	time.Sleep(15 * time.Second)

	err = s.k8sh.WaitForLabeledPodsToRun("app=rook-ceph-mon", s.namespace)
	require.Nil(s.T(), err)

	// wait for the osd pods to be updated
	err = k8sutil.WaitForDeploymentImage(s.k8sh.Clientset, s.namespace, "app=rook-ceph-osd", opspec.ConfigInitContainerName, true, "rook/ceph:master")
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
	logger.Infof("Placeholder: create the new resources that have been added since 1.0")
	namespace := s.namespace
	newResources := `
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
rules:
- apiGroups:
  - ""
  resources:
  - pods
  - configmaps
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - delete
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-cmd-reporter
subjects:
- kind: ServiceAccount
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rook-ceph-cmd-reporter-psp
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
- kind: ServiceAccount
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `

---
`
	logger.Infof("creating the new resources that have been added since 1.0")
	return s.k8sh.ResourceOperation("apply", newResources)
}
