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

	s.op, s.k8sh = StartTestCluster(s.T, s.namespace, "bluestore", false, useDevices, 1, installer.Version0_8)
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
	defer cleanupFilesystemConsumer(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)

	logger.Infof("Initializing object before the upgrade")
	objectStoreName := "upgraded-object"
	runObjectE2ETestLite(s.helper, s.k8sh, s.Suite, s.namespace, objectStoreName, 1)

	// verify that we're actually running 0.8 before the upgrade
	operatorContainer := "rook-ceph-operator"
	version, err := k8sutil.GetDeploymentVersion(s.k8sh.Clientset, systemNamespace, operatorContainer, operatorContainer)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), "v0.8.1", version)

	s.simpleWriteAndRead(podName, "pre-upgrade-file")

	// Upgrade to master
	require.Nil(s.T(), s.k8sh.SetDeploymentVersion(systemNamespace, operatorContainer, operatorContainer, installer.VersionMaster))

	// verify that the operator spec is updated
	version, err = k8sutil.GetDeploymentVersion(s.k8sh.Clientset, systemNamespace, operatorContainer, operatorContainer)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), installer.VersionMaster, version)

	// wait for the osd pods to be updated
	err = k8sutil.WaitForDeploymentVersion(s.k8sh.Clientset, s.namespace, "app=rook-ceph-osd", "rook-ceph-osd", installer.VersionMaster)
	require.Nil(s.T(), err)
	logger.Infof("Done with automatic upgrade to master")

	// test writing and reading from the mounts that were created before the upgrade
	s.simpleWriteAndRead(podName, "post-upgrade-file")
}

func (s *UpgradeSuite) simpleWriteAndRead(podName, filename string) {
	// Verify the block storage is functional
	logger.Infof("Write to block storage")
	message := "basic data written after upgrade"
	_, wtErr := s.helper.BlockClient.Write(podName, message, filename, "")
	require.Nil(s.T(), wtErr)

	logger.Infof("Read from block storage")
	data, rErr := s.helper.BlockClient.Read(podName, filename, "")
	assert.Nil(s.T(), rErr)
	require.Contains(s.T(), data, message, "make sure content of the files is unchanged")
	logger.Infof("Read from  Block storage successfully")

	// Verify the file storage is functional
	writeAndReadToFilesystem(s.helper, s.k8sh, s.Suite, s.namespace, filename)
}
