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

	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	rbdPodName        = "test-pod-upgrade"
	operatorContainer = "rook-ceph-operator"
	poolName          = "upgradepool"
	storageClassName  = "block-upgrade"
	blockName         = "block-claim-upgrade"
	bucketPrefix      = "generate-me" // use generated bucket name for this test
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
		HandlePanics(recover(), s.TearDownSuite, s.T)
	}(s)
	suite.Run(t, s)
}

type UpgradeSuite struct {
	suite.Suite
	helper    *clients.TestClient
	k8sh      *utils.K8sHelper
	settings  *installer.TestCephSettings
	installer *installer.CephInstaller
	namespace string
}

func (s *UpgradeSuite) SetupSuite() {
	s.namespace = "upgrade-ns"
	s.settings = &installer.TestCephSettings{
		ClusterName:       s.namespace,
		Namespace:         s.namespace,
		OperatorNamespace: installer.SystemNamespace(s.namespace),
		StorageClassName:  "",
		UseHelm:           false,
		UsePVC:            false,
		Mons:              1,
		SkipOSDCreation:   false,
		RookVersion:       installer.Version1_6,
		CephVersion:       installer.NautilusPartitionVersion,
	}

	s.installer, s.k8sh = StartTestCluster(s.T, s.settings)
	s.helper = clients.CreateTestClient(s.k8sh, s.installer.Manifests)
}

func (s *UpgradeSuite) TearDownSuite() {
	s.installer.UninstallRook()
}

func (s *UpgradeSuite) TestUpgradeRookToMaster() {
	message := "my simple message"
	objectStoreName := "upgraded-object"
	objectUserID := "upgraded-user"
	preFilename := "pre-upgrade-file"
	numOSDs, filesystemName, rbdFilesToRead, cephfsFilesToRead := s.deployClusterforUpgrade(objectStoreName, objectUserID, message, preFilename)
	s.settings.CephVersion = installer.NautilusVersion

	clusterInfo := client.AdminClusterInfo(s.namespace)
	requireBlockImagesRemoved := false
	defer func() {
		blockTestDataCleanUp(s.helper, s.k8sh, s.Suite, clusterInfo, poolName, storageClassName, blockName, rbdPodName, requireBlockImagesRemoved)
		cleanupFilesystemConsumer(s.helper, s.k8sh, s.Suite, s.namespace, filePodName)
		cleanupFilesystem(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)
		_ = s.helper.ObjectUserClient.Delete(s.namespace, objectUserID)
		_ = s.helper.BucketClient.DeleteObc(obcName, bucketStorageClassName, bucketPrefix, maxObject, false)
		_ = s.helper.BucketClient.DeleteBucketStorageClass(s.namespace, objectStoreName, bucketStorageClassName, "Delete", region)
		objectStoreCleanUp(s.Suite, s.helper, s.k8sh, s.settings.Namespace, objectStoreName)
	}()

	//
	// Upgrade Rook from v1.6 to master
	//
	logger.Infof("*** UPGRADING ROOK FROM %s to master ***", installer.Version1_6)
	s.gatherLogs(s.settings.OperatorNamespace, "_before_master_upgrade")
	s.upgradeToMaster()

	s.verifyOperatorImage(installer.LocalBuildTag)
	s.verifyRookUpgrade(numOSDs)
	err := s.installer.WaitForToolbox(s.namespace)
	assert.NoError(s.T(), err)

	logger.Infof("Done with automatic upgrade from %s to master", installer.Version1_6)
	newFile := "post-upgrade-1_6-to-master-file"
	s.verifyFilesAfterUpgrade(filesystemName, newFile, message, rbdFilesToRead, cephfsFilesToRead)
	rbdFilesToRead = append(rbdFilesToRead, newFile)
	cephfsFilesToRead = append(cephfsFilesToRead, newFile)

	checkCephObjectUser(s.Suite, s.helper, s.k8sh, s.namespace, objectStoreName, objectUserID, true, false)

	// should be Bound after upgrade to Rook master
	// do not need retry b/c the OBC controller runs parallel to Rook-Ceph orchestration
	assert.True(s.T(), s.helper.BucketClient.CheckOBC(obcName, "bound"))

	logger.Infof("Verified upgrade from %s to master", installer.Version1_6)

	//
	// Upgrade from nautilus to octopus
	//
	logger.Infof("*** UPGRADING CEPH FROM Nautilus TO Octopus ***")
	s.gatherLogs(s.settings.OperatorNamespace, "_before_octopus_upgrade")
	s.upgradeCephVersion(installer.OctopusVersion.Image, numOSDs)
	// Verify reading and writing to the test clients
	newFile = "post-octopus-upgrade-file"
	s.verifyFilesAfterUpgrade(filesystemName, newFile, message, rbdFilesToRead, cephfsFilesToRead)
	logger.Infof("Verified upgrade from nautilus to octopus")

	checkCephObjectUser(s.Suite, s.helper, s.k8sh, s.namespace, objectStoreName, objectUserID, true, false)

	//
	// Upgrade from octopus to pacific
	//
	logger.Infof("*** UPGRADING CEPH FROM OCTOPUS TO PACIFIC ***")
	s.gatherLogs(s.settings.OperatorNamespace, "_before_pacific_upgrade")
	s.upgradeCephVersion(installer.PacificVersion.Image, numOSDs)
	// Verify reading and writing to the test clients
	newFile = "post-pacific-upgrade-file"
	s.verifyFilesAfterUpgrade(filesystemName, newFile, message, rbdFilesToRead, cephfsFilesToRead)
	logger.Infof("Verified upgrade from octopus to pacific")

	checkCephObjectUser(s.Suite, s.helper, s.k8sh, s.namespace, objectStoreName, objectUserID, true, false)
}

func (s *UpgradeSuite) TestUpgradeCephToOctopusDevel() {
	message := "my simple message"
	objectStoreName := "upgraded-object"
	objectUserID := "upgraded-user"
	preFilename := "pre-upgrade-file"
	s.settings.CephVersion = installer.OctopusVersion
	numOSDs, filesystemName, rbdFilesToRead, cephfsFilesToRead := s.deployClusterforUpgrade(objectStoreName, objectUserID, message, preFilename)
	clusterInfo := client.AdminClusterInfo(s.namespace)
	requireBlockImagesRemoved := false
	defer func() {
		blockTestDataCleanUp(s.helper, s.k8sh, s.Suite, clusterInfo, poolName, storageClassName, blockName, rbdPodName, requireBlockImagesRemoved)
		cleanupFilesystemConsumer(s.helper, s.k8sh, s.Suite, s.namespace, filePodName)
		cleanupFilesystem(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)
		_ = s.helper.ObjectUserClient.Delete(s.namespace, objectUserID)
		_ = s.helper.BucketClient.DeleteObc(obcName, bucketStorageClassName, bucketPrefix, maxObject, false)
		_ = s.helper.BucketClient.DeleteBucketStorageClass(s.namespace, objectStoreName, bucketStorageClassName, "Delete", region)
		objectStoreCleanUp(s.Suite, s.helper, s.k8sh, s.settings.Namespace, objectStoreName)
	}()

	//
	// Upgrade from octopus to octopus
	//
	logger.Infof("*** UPGRADING CEPH FROM OCTOPUS STABLE TO OCTOPUS DEVEL ***")
	s.gatherLogs(s.settings.OperatorNamespace, "_before_pacific_upgrade")
	s.upgradeCephVersion(installer.OctopusDevelVersion.Image, numOSDs)
	// Verify reading and writing to the test clients
	newFile := "post-octopus-upgrade-file"
	s.verifyFilesAfterUpgrade(filesystemName, newFile, message, rbdFilesToRead, cephfsFilesToRead)
	logger.Infof("Verified upgrade from octopus stable to octopus devel")

	checkCephObjectUser(s.Suite, s.helper, s.k8sh, s.namespace, objectStoreName, objectUserID, true, false)
}

func (s *UpgradeSuite) TestUpgradeCephToPacificDevel() {
	message := "my simple message"
	objectStoreName := "upgraded-object"
	objectUserID := "upgraded-user"
	preFilename := "pre-upgrade-file"
	s.settings.CephVersion = installer.PacificVersion
	numOSDs, filesystemName, rbdFilesToRead, cephfsFilesToRead := s.deployClusterforUpgrade(objectStoreName, objectUserID, message, preFilename)
	clusterInfo := client.AdminClusterInfo(s.namespace)
	requireBlockImagesRemoved := false
	defer func() {
		blockTestDataCleanUp(s.helper, s.k8sh, s.Suite, clusterInfo, poolName, storageClassName, blockName, rbdPodName, requireBlockImagesRemoved)
		cleanupFilesystemConsumer(s.helper, s.k8sh, s.Suite, s.namespace, filePodName)
		cleanupFilesystem(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)
		_ = s.helper.ObjectUserClient.Delete(s.namespace, objectUserID)
		_ = s.helper.BucketClient.DeleteObc(obcName, bucketStorageClassName, bucketPrefix, maxObject, false)
		_ = s.helper.BucketClient.DeleteBucketStorageClass(s.namespace, objectStoreName, bucketStorageClassName, "Delete", region)
		objectStoreCleanUp(s.Suite, s.helper, s.k8sh, s.settings.Namespace, objectStoreName)
	}()

	//
	// Upgrade from octopus to pacific
	//
	logger.Infof("*** UPGRADING CEPH FROM PACIFIC STABLE TO PACIFIC DEVEL ***")
	s.gatherLogs(s.settings.OperatorNamespace, "_before_pacific_upgrade")
	s.upgradeCephVersion(installer.PacificDevelVersion.Image, numOSDs)
	// Verify reading and writing to the test clients
	newFile := "post-pacific-upgrade-file"
	s.verifyFilesAfterUpgrade(filesystemName, newFile, message, rbdFilesToRead, cephfsFilesToRead)
	logger.Infof("Verified upgrade from pacific stable to pacific devel")

	checkCephObjectUser(s.Suite, s.helper, s.k8sh, s.namespace, objectStoreName, objectUserID, true, false)
}

func (s *UpgradeSuite) deployClusterforUpgrade(objectStoreName, objectUserID, message, preFilename string) (int, string, []string, []string) {
	//
	// Create block, object, and file storage before the upgrade
	//
	logger.Infof("Initializing block before the upgrade")
	clusterInfo := client.AdminClusterInfo(s.namespace)
	setupBlockLite(s.helper, s.k8sh, s.Suite, clusterInfo, poolName, storageClassName, blockName, rbdPodName)

	createPodWithBlock(s.helper, s.k8sh, s.Suite, s.namespace, storageClassName, rbdPodName, blockName)

	// Create the filesystem
	logger.Infof("Initializing file before the upgrade")
	filesystemName := "upgrade-test-fs"
	activeCount := 1
	createFilesystem(s.helper, s.k8sh, s.Suite, s.settings, filesystemName, activeCount)

	// Start the file test client
	fsStorageClass := "file-upgrade"
	assert.NoError(s.T(), s.helper.FSClient.CreateStorageClass(filesystemName, s.settings.OperatorNamespace, s.namespace, fsStorageClass))
	createFilesystemConsumerPod(s.helper, s.k8sh, s.Suite, s.settings, filesystemName, fsStorageClass)

	logger.Infof("Initializing object before the upgrade")
	deleteStore := false
	tls := false
	runObjectE2ETestLite(s.T(), s.helper, s.k8sh, s.settings.Namespace, objectStoreName, 1, deleteStore, tls)

	logger.Infof("Initializing object user before the upgrade")
	createCephObjectUser(s.Suite, s.helper, s.k8sh, s.namespace, objectStoreName, objectUserID, false, false)

	logger.Info("Initializing object bucket claim before the upgrade")
	cobErr := s.helper.BucketClient.CreateBucketStorageClass(s.namespace, objectStoreName, bucketStorageClassName, "Delete", region)
	require.Nil(s.T(), cobErr)
	cobcErr := s.helper.BucketClient.CreateObc(obcName, bucketStorageClassName, bucketPrefix, maxObject, false)
	require.Nil(s.T(), cobcErr)

	created := utils.Retry(12, 2*time.Second, "OBC is created", func() bool {
		// do not check if bound here b/c this fails in Rook v1.4
		return s.helper.BucketClient.CheckOBC(obcName, "created")
	})
	require.True(s.T(), created)

	// verify that we're actually running the right pre-upgrade image
	s.verifyOperatorImage(installer.Version1_6)

	assert.NoError(s.T(), s.k8sh.WriteToPod("", rbdPodName, preFilename, message))
	assert.NoError(s.T(), s.k8sh.ReadFromPod("", rbdPodName, preFilename, message))

	// we will keep appending to this to continue verifying old files through the upgrades
	rbdFilesToRead := []string{preFilename}
	cephfsFilesToRead := []string{}

	// Get some info about the currently deployed OSDs to determine later if they are all updated
	osdDepList, err := k8sutil.GetDeployments(s.k8sh.Clientset, s.namespace, "app=rook-ceph-osd")
	require.NoError(s.T(), err)
	osdDeps := osdDepList.Items
	numOSDs := len(osdDeps) // there should be this many upgraded OSDs
	require.NotEqual(s.T(), 0, numOSDs)

	return numOSDs, filesystemName, rbdFilesToRead, cephfsFilesToRead
}

func (s *UpgradeSuite) gatherLogs(systemNamespace, testSuffix string) {
	// Gather logs before Ceph upgrade to help with debugging
	if installer.TestLogCollectionLevel() == "all" {
		s.k8sh.PrintPodDescribe(s.namespace)
	}
	n := strings.Replace(s.T().Name(), "/", "_", -1) + testSuffix
	s.installer.GatherAllRookLogs(n, systemNamespace, s.namespace)
}

func (s *UpgradeSuite) upgradeCephVersion(newCephImage string, numOSDs int) {
	osdDepList, err := k8sutil.GetDeployments(s.k8sh.Clientset, s.namespace, "app=rook-ceph-osd")
	require.NoError(s.T(), err)
	oldCephVersion := osdDepList.Items[0].Labels["ceph-version"] // upgraded OSDs should not have this version label

	_, err = s.k8sh.Kubectl("-n", s.namespace, "patch", "CephCluster", s.namespace, "--type=merge",
		"-p", fmt.Sprintf(`{"spec": {"cephVersion": {"image": "%s"}}}`, newCephImage))

	assert.NoError(s.T(), err)
	s.waitForUpgradedDaemons(oldCephVersion, "ceph-version", numOSDs, false)
}

func (s *UpgradeSuite) verifyOperatorImage(expectedImage string) {
	systemNamespace := installer.SystemNamespace(s.namespace)

	// verify that the operator spec is updated
	version, err := k8sutil.GetDeploymentImage(s.k8sh.Clientset, systemNamespace, operatorContainer, operatorContainer)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "rook/ceph:"+expectedImage, version)
}

func (s *UpgradeSuite) verifyRookUpgrade(numOSDs int) {
	// Get some info about the currently deployed mons to determine later if they are all updated
	monDepList, err := k8sutil.GetDeployments(s.k8sh.Clientset, s.namespace, "app=rook-ceph-mon")
	require.NoError(s.T(), err)
	require.Equal(s.T(), s.settings.Mons, len(monDepList.Items), monDepList.Items)

	// Get some info about the currently deployed mgr to determine later if it is updated
	mgrDepList, err := k8sutil.GetDeployments(s.k8sh.Clientset, s.namespace, "app=rook-ceph-mgr")
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, len(mgrDepList.Items))

	// Get some info about the currently deployed OSDs to determine later if they are all updated
	osdDepList, err := k8sutil.GetDeployments(s.k8sh.Clientset, s.namespace, "app=rook-ceph-osd")
	require.NoError(s.T(), err)
	require.NotZero(s.T(), len(osdDepList.Items))
	require.Equal(s.T(), numOSDs, len(osdDepList.Items), osdDepList.Items)

	d := osdDepList.Items[0]
	oldRookVersion := d.Labels["rook-version"] // upgraded OSDs should not have this version label

	s.waitForUpgradedDaemons(oldRookVersion, "rook-version", numOSDs, true)
}

func (s *UpgradeSuite) waitForUpgradedDaemons(previousVersion, versionLabel string, numOSDs int, waitForMDS bool) {
	// wait for the mon(s) to be updated
	monsNotOldVersion := fmt.Sprintf("app=rook-ceph-mon,%s!=%s", versionLabel, previousVersion)
	err := s.k8sh.WaitForDeploymentCount(monsNotOldVersion, s.namespace, s.settings.Mons)
	require.NoError(s.T(), err, "mon(s) didn't update")
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(monsNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	// wait for the mgr to be updated
	mgrNotOldVersion := fmt.Sprintf("app=rook-ceph-mgr,%s!=%s", versionLabel, previousVersion)
	err = s.k8sh.WaitForDeploymentCount(mgrNotOldVersion, s.namespace, 1)
	require.NoError(s.T(), err, "mgr didn't update")
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(mgrNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	// wait for the osd pods to be updated
	osdsNotOldVersion := fmt.Sprintf("app=rook-ceph-osd,%s!=%s", versionLabel, previousVersion)
	err = s.k8sh.WaitForDeploymentCount(osdsNotOldVersion, s.namespace, numOSDs)
	require.NoError(s.T(), err, "osd(s) didn't update")
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(osdsNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	// wait for the mds pods to be updated
	// FIX: In v1.2 there was a race condition that can cause the MDS to not be updated, so we skip
	// the check for MDS upgrade in case it's just a ceph upgrade (no operator restart)
	if waitForMDS {
		mdsesNotOldVersion := fmt.Sprintf("app=rook-ceph-mds,%s!=%s", versionLabel, previousVersion)
		err = s.k8sh.WaitForDeploymentCount(mdsesNotOldVersion, s.namespace, 2 /* always expect 2 mdses */)
		require.NoError(s.T(), err)
		err = s.k8sh.WaitForLabeledDeploymentsToBeReady(mdsesNotOldVersion, s.namespace)
		require.NoError(s.T(), err)
	}

	rgwsNotOldVersion := fmt.Sprintf("app=rook-ceph-rgw,%s!=%s", versionLabel, previousVersion)
	err = s.k8sh.WaitForDeploymentCount(rgwsNotOldVersion, s.namespace, 1 /* always expect 1 rgw */)
	require.NoError(s.T(), err)
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(rgwsNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	// Give a few seconds for the daemons to settle down after the upgrade
	time.Sleep(5 * time.Second)
}

func (s *UpgradeSuite) verifyFilesAfterUpgrade(fsName, newFileToWrite, messageForAllFiles string, rbdFilesToRead, cephFSFilesToRead []string) {
	retryCount := 5

	for _, file := range rbdFilesToRead {
		// test reading preexisting files in the pod with rbd mounted
		// There is some unreliability right after the upgrade when there is only one osd, so we will retry if needed
		assert.NoError(s.T(), s.k8sh.ReadFromPodRetry("", rbdPodName, file, messageForAllFiles, retryCount))
	}

	// test writing and reading a new file in the pod with rbd mounted
	assert.NoError(s.T(), s.k8sh.WriteToPodRetry("", rbdPodName, newFileToWrite, messageForAllFiles, retryCount))
	assert.NoError(s.T(), s.k8sh.ReadFromPodRetry("", rbdPodName, newFileToWrite, messageForAllFiles, retryCount))

	if fsName != "" {
		// wait for filesystem to be active
		clusterInfo := client.AdminClusterInfo(s.namespace)
		err := waitForFilesystemActive(s.k8sh, clusterInfo, fsName)
		require.NoError(s.T(), err)

		// test reading preexisting files in the pod with cephfs mounted
		for _, file := range cephFSFilesToRead {
			assert.NoError(s.T(), s.k8sh.ReadFromPodRetry(s.namespace, filePodName, file, messageForAllFiles, retryCount))
		}

		// test writing and reading a new file in the pod with cephfs mounted
		assert.NoError(s.T(), s.k8sh.WriteToPodRetry(s.namespace, filePodName, newFileToWrite, messageForAllFiles, retryCount))
		assert.NoError(s.T(), s.k8sh.ReadFromPodRetry(s.namespace, filePodName, newFileToWrite, messageForAllFiles, retryCount))
	}
}

// UpgradeToMaster performs the steps necessary to upgrade a Rook v1.4 cluster to master. It does not
// verify the upgrade but merely starts the upgrade process.
func (s *UpgradeSuite) upgradeToMaster() {
	// Apply the CRDs for the latest master
	s.settings.RookVersion = installer.LocalBuildTag
	m := installer.NewCephManifests(s.settings)
	require.NoError(s.T(), s.k8sh.ResourceOperation("apply", m.GetCRDs(s.k8sh)))

	require.NoError(s.T(), s.k8sh.ResourceOperation("apply", m.GetCommon()))

	require.NoError(s.T(),
		s.k8sh.SetDeploymentVersion(s.settings.OperatorNamespace, operatorContainer, operatorContainer, installer.LocalBuildTag))

	require.NoError(s.T(),
		s.k8sh.SetDeploymentVersion(s.settings.Namespace, "rook-ceph-tools", "rook-ceph-tools", installer.LocalBuildTag))
}
