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
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	v1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
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
	blockName         = "block-claim-upgrade"
	bucketPrefix      = "generate-me" // use generated bucket name for this test
	simpleTestMessage = "my simple message"
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
	// All setup is in baseSetup()
}

func (s *UpgradeSuite) TearDownSuite() {
	s.installer.UninstallRook()
}

func (s *UpgradeSuite) baseSetup(useHelm bool, initialCephVersion v1.CephVersionSpec) {
	s.namespace = "upgrade"
	s.settings = &installer.TestCephSettings{
		ClusterName:                 s.namespace,
		Namespace:                   s.namespace,
		OperatorNamespace:           installer.SystemNamespace(s.namespace),
		UseHelm:                     useHelm,
		RetainHelmDefaultStorageCRs: true,
		UsePVC:                      false,
		Mons:                        1,
		EnableDiscovery:             true,
		SkipClusterCleanup:          true,
		RookVersion:                 installer.Version1_11,
		CephVersion:                 initialCephVersion,
	}

	s.installer, s.k8sh = StartTestCluster(s.T, s.settings)
	s.helper = clients.CreateTestClient(s.k8sh, s.installer.Manifests)
}

func (s *UpgradeSuite) TestUpgradeRook() {
	s.testUpgrade(false, installer.PacificVersion)
}

func (s *UpgradeSuite) TestUpgradeHelm() {
	s.testUpgrade(true, installer.PacificVersion)
}

func (s *UpgradeSuite) testUpgrade(useHelm bool, initialCephVersion v1.CephVersionSpec) {
	s.baseSetup(useHelm, initialCephVersion)

	objectUserID := "upgraded-user"
	preFilename := "pre-upgrade-file"
	numOSDs, rbdFilesToRead, cephfsFilesToRead := s.deployClusterforUpgrade(objectUserID, preFilename)

	clusterInfo := client.AdminTestClusterInfo(s.namespace)
	requireBlockImagesRemoved := false
	defer func() {
		blockTestDataCleanUp(s.helper, s.k8sh, &s.Suite, clusterInfo, installer.BlockPoolName, installer.BlockPoolSCName, blockName, rbdPodName, requireBlockImagesRemoved)
		cleanupFilesystemConsumer(s.helper, s.k8sh, &s.Suite, s.namespace, filePodName)
		cleanupFilesystem(s.helper, s.k8sh, &s.Suite, s.namespace, installer.FilesystemName)
		_ = s.helper.ObjectUserClient.Delete(s.namespace, objectUserID)
		_ = s.helper.BucketClient.DeleteObc(obcName, installer.ObjectStoreSCName, bucketPrefix, maxObject, false)
		_ = s.helper.BucketClient.DeleteBucketStorageClass(s.namespace, installer.ObjectStoreName, installer.ObjectStoreSCName, "Delete")
		objectStoreCleanUp(&s.Suite, s.helper, s.k8sh, s.settings.Namespace, installer.ObjectStoreName)
	}()

	// Delete Object-SC before upgrade test (https://github.com/rook/rook/issues/10153)
	_ = s.helper.BucketClient.DeleteBucketStorageClass(s.namespace, installer.ObjectStoreName, installer.ObjectStoreSCName, "Delete")

	//
	// Upgrade Rook from v1.11 to master
	//
	logger.Infof("*** UPGRADING ROOK FROM %s to master ***", installer.Version1_11)
	s.gatherLogs(s.settings.OperatorNamespace, "_before_master_upgrade")
	s.upgradeToMaster()

	s.verifyOperatorImage(installer.LocalBuildTag)
	s.verifyRookUpgrade(numOSDs)
	err := s.installer.WaitForToolbox(s.namespace)
	assert.NoError(s.T(), err)

	logger.Infof("Done with automatic upgrade from %s to master", installer.Version1_11)
	newFile := "post-upgrade-previous-to-master-file"
	s.verifyFilesAfterUpgrade(newFile, rbdFilesToRead, cephfsFilesToRead)
	rbdFilesToRead = append(rbdFilesToRead, newFile)
	cephfsFilesToRead = append(cephfsFilesToRead, newFile)

	checkCephObjectUser(&s.Suite, s.helper, s.k8sh, s.namespace, installer.ObjectStoreName, objectUserID, true, false)

	// should be Bound after upgrade to Rook master
	// do not need retry b/c the OBC controller runs parallel to Rook-Ceph orchestration
	assert.True(s.T(), s.helper.BucketClient.CheckOBC(obcName, "bound"))

	logger.Infof("Verified upgrade from %s to master", installer.Version1_11)

	// SKIP the Ceph version upgrades for the helm test
	if s.settings.UseHelm {
		return
	}

	//
	// Upgrade from pacific to quincy
	//
	logger.Infof("*** UPGRADING CEPH FROM PACIFIC TO QUINCY ***")
	s.gatherLogs(s.settings.OperatorNamespace, "_before_quincy_upgrade")
	s.upgradeCephVersion(installer.QuincyVersion.Image, numOSDs)
	// Verify reading and writing to the test clients
	newFile = "post-quincy-upgrade-file"
	s.verifyFilesAfterUpgrade(newFile, rbdFilesToRead, cephfsFilesToRead)
	logger.Infof("Verified upgrade from pacific to quincy")

	//
	// Upgrade from quincy to reef
	//
	logger.Infof("*** UPGRADING CEPH FROM QUINCY TO REEF ***")
	s.gatherLogs(s.settings.OperatorNamespace, "_before_reef_upgrade")
	s.upgradeCephVersion(installer.ReefVersion.Image, numOSDs)
	// Verify reading and writing to the test clients
	newFile = "post-reef-upgrade-file"
	s.verifyFilesAfterUpgrade(newFile, rbdFilesToRead, cephfsFilesToRead)
	logger.Infof("Verified upgrade from quincy to reef")

	checkCephObjectUser(&s.Suite, s.helper, s.k8sh, s.namespace, installer.ObjectStoreName, objectUserID, true, false)
}

func (s *UpgradeSuite) TestUpgradeCephToQuincyDevel() {
	s.baseSetup(false, installer.QuincyVersion)

	objectUserID := "upgraded-user"
	preFilename := "pre-upgrade-file"
	s.settings.CephVersion = installer.QuincyVersion
	numOSDs, rbdFilesToRead, cephfsFilesToRead := s.deployClusterforUpgrade(objectUserID, preFilename)
	clusterInfo := client.AdminTestClusterInfo(s.namespace)
	requireBlockImagesRemoved := false
	defer func() {
		blockTestDataCleanUp(s.helper, s.k8sh, &s.Suite, clusterInfo, installer.BlockPoolName, installer.BlockPoolSCName, blockName, rbdPodName, requireBlockImagesRemoved)
		cleanupFilesystemConsumer(s.helper, s.k8sh, &s.Suite, s.namespace, filePodName)
		cleanupFilesystem(s.helper, s.k8sh, &s.Suite, s.namespace, installer.FilesystemName)
		_ = s.helper.ObjectUserClient.Delete(s.namespace, objectUserID)
		_ = s.helper.BucketClient.DeleteObc(obcName, installer.ObjectStoreSCName, bucketPrefix, maxObject, false)
		_ = s.helper.BucketClient.DeleteBucketStorageClass(s.namespace, installer.ObjectStoreName, installer.ObjectStoreName, "Delete")
		objectStoreCleanUp(&s.Suite, s.helper, s.k8sh, s.settings.Namespace, installer.ObjectStoreName)
	}()

	//
	// Upgrade from quincy to quincy devel
	//
	logger.Infof("*** UPGRADING CEPH FROM QUINCY STABLE TO QUINCY DEVEL ***")
	s.gatherLogs(s.settings.OperatorNamespace, "_before_quincy_upgrade")
	s.upgradeCephVersion(installer.QuincyDevelVersion.Image, numOSDs)
	// Verify reading and writing to the test clients
	newFile := "post-quincy-upgrade-file"
	s.verifyFilesAfterUpgrade(newFile, rbdFilesToRead, cephfsFilesToRead)
	logger.Infof("Verified upgrade from quincy stable to quincy devel")

	checkCephObjectUser(&s.Suite, s.helper, s.k8sh, s.namespace, installer.ObjectStoreName, objectUserID, true, false)
}

func (s *UpgradeSuite) TestUpgradeCephToPacificDevel() {
	s.baseSetup(false, installer.PacificVersion)

	objectUserID := "upgraded-user"
	preFilename := "pre-upgrade-file"
	s.settings.CephVersion = installer.PacificVersion
	numOSDs, rbdFilesToRead, cephfsFilesToRead := s.deployClusterforUpgrade(objectUserID, preFilename)
	clusterInfo := client.AdminTestClusterInfo(s.namespace)
	requireBlockImagesRemoved := false
	defer func() {
		blockTestDataCleanUp(s.helper, s.k8sh, &s.Suite, clusterInfo, installer.BlockPoolName, installer.BlockPoolSCName, blockName, rbdPodName, requireBlockImagesRemoved)
		cleanupFilesystemConsumer(s.helper, s.k8sh, &s.Suite, s.namespace, filePodName)
		cleanupFilesystem(s.helper, s.k8sh, &s.Suite, s.namespace, installer.FilesystemName)
		_ = s.helper.ObjectUserClient.Delete(s.namespace, objectUserID)
		_ = s.helper.BucketClient.DeleteObc(obcName, installer.ObjectStoreSCName, bucketPrefix, maxObject, false)
		_ = s.helper.BucketClient.DeleteBucketStorageClass(s.namespace, installer.ObjectStoreName, installer.ObjectStoreSCName, "Delete")
		objectStoreCleanUp(&s.Suite, s.helper, s.k8sh, s.settings.Namespace, installer.ObjectStoreName)
	}()

	//
	// Upgrade from pacific to pacific devel
	//
	logger.Infof("*** UPGRADING CEPH FROM PACIFIC STABLE TO PACIFIC DEVEL ***")
	s.gatherLogs(s.settings.OperatorNamespace, "_before_pacific_upgrade")
	s.upgradeCephVersion(installer.PacificDevelVersion.Image, numOSDs)
	// Verify reading and writing to the test clients
	newFile := "post-pacific-upgrade-file"
	s.verifyFilesAfterUpgrade(newFile, rbdFilesToRead, cephfsFilesToRead)
	logger.Infof("verified upgrade from pacific stable to pacific devel")

	checkCephObjectUser(&s.Suite, s.helper, s.k8sh, s.namespace, installer.ObjectStoreName, objectUserID, true, false)
}

func (s *UpgradeSuite) deployClusterforUpgrade(objectUserID, preFilename string) (int, []string, []string) {
	//
	// Create block, object, and file storage before the upgrade
	// The helm chart already created these though.
	//
	clusterInfo := client.AdminTestClusterInfo(s.namespace)
	if !s.settings.UseHelm {
		logger.Infof("Initializing block before the upgrade")
		setupBlockLite(s.helper, s.k8sh, &s.Suite, clusterInfo, installer.BlockPoolName, installer.BlockPoolSCName, blockName)
	} else {
		createAndWaitForPVC(s.helper, s.k8sh, &s.Suite, clusterInfo, installer.BlockPoolSCName, blockName)
	}

	createPodWithBlock(s.helper, s.k8sh, &s.Suite, s.namespace, installer.BlockPoolSCName, rbdPodName, blockName)

	if !s.settings.UseHelm {
		// Create the filesystem
		logger.Infof("Initializing file before the upgrade")
		activeCount := 1
		createFilesystem(s.helper, s.k8sh, &s.Suite, s.settings, installer.FilesystemName, activeCount)
		assert.NoError(s.T(), s.helper.FSClient.CreateStorageClass(installer.FilesystemName, s.settings.OperatorNamespace, s.namespace, installer.FilesystemSCName))
	}

	// Start the file test client
	createFilesystemConsumerPod(s.helper, s.k8sh, &s.Suite, s.settings, installer.FilesystemName, installer.FilesystemSCName)

	if !s.settings.UseHelm {
		logger.Infof("Initializing object before the upgrade")
		deleteStore := false
		tls := false
		runObjectE2ETestLite(s.T(), s.helper, s.k8sh, s.installer, s.settings.Namespace, installer.ObjectStoreName, 1, deleteStore, tls)
	}

	logger.Infof("Initializing object user before the upgrade")
	createCephObjectUser(&s.Suite, s.helper, s.k8sh, s.namespace, installer.ObjectStoreName, objectUserID, false, false)

	logger.Info("Initializing object bucket claim before the upgrade")
	cobErr := s.helper.BucketClient.CreateBucketStorageClass(s.namespace, installer.ObjectStoreName, installer.ObjectStoreName, "Delete")
	require.Nil(s.T(), cobErr)
	cobcErr := s.helper.BucketClient.CreateObc(obcName, installer.ObjectStoreName, bucketPrefix, maxObject, false)
	require.Nil(s.T(), cobcErr)

	created := utils.Retry(12, 2*time.Second, "OBC is created", func() bool {
		// do not check if bound here b/c this fails in Rook v1.4
		return s.helper.BucketClient.CheckOBC(obcName, "created")
	})
	require.True(s.T(), created)

	// verify that we're actually running the right pre-upgrade image
	s.verifyOperatorImage(installer.Version1_11)

	assert.NoError(s.T(), s.k8sh.WriteToPod("", rbdPodName, preFilename, simpleTestMessage))
	assert.NoError(s.T(), s.k8sh.ReadFromPod("", rbdPodName, preFilename, simpleTestMessage))

	// we will keep appending to this to continue verifying old files through the upgrades
	rbdFilesToRead := []string{preFilename}
	cephfsFilesToRead := []string{}

	// Get some info about the currently deployed OSDs to determine later if they are all updated
	osdDepList, err := k8sutil.GetDeployments(context.TODO(), s.k8sh.Clientset, s.namespace, "app=rook-ceph-osd")
	require.NoError(s.T(), err)
	osdDeps := osdDepList.Items
	numOSDs := len(osdDeps) // there should be this many upgraded OSDs
	require.NotEqual(s.T(), 0, numOSDs)

	return numOSDs, rbdFilesToRead, cephfsFilesToRead
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
	osdDepList, err := k8sutil.GetDeployments(context.TODO(), s.k8sh.Clientset, s.namespace, "app=rook-ceph-osd")
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
	version, err := k8sutil.GetDeploymentImage(context.TODO(), s.k8sh.Clientset, systemNamespace, operatorContainer, operatorContainer)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "rook/ceph:"+expectedImage, version)
}

func (s *UpgradeSuite) verifyRookUpgrade(numOSDs int) {
	// Get some info about the currently deployed mons to determine later if they are all updated
	monDepList, err := k8sutil.GetDeployments(context.TODO(), s.k8sh.Clientset, s.namespace, "app=rook-ceph-mon")
	require.NoError(s.T(), err)
	require.Equal(s.T(), s.settings.Mons, len(monDepList.Items), monDepList.Items)

	// Get some info about the currently deployed mgr to determine later if it is updated
	mgrDepList, err := k8sutil.GetDeployments(context.TODO(), s.k8sh.Clientset, s.namespace, "app=rook-ceph-mgr")
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, len(mgrDepList.Items))

	// Get some info about the currently deployed OSDs to determine later if they are all updated
	osdDepList, err := k8sutil.GetDeployments(context.TODO(), s.k8sh.Clientset, s.namespace, "app=rook-ceph-osd")
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

func (s *UpgradeSuite) verifyFilesAfterUpgrade(newFileToWrite string, rbdFilesToRead, cephFSFilesToRead []string) {
	retryCount := 5

	for _, file := range rbdFilesToRead {
		// test reading preexisting files in the pod with rbd mounted
		// There is some unreliability right after the upgrade when there is only one osd, so we will retry if needed
		assert.NoError(s.T(), s.k8sh.ReadFromPodRetry("", rbdPodName, file, simpleTestMessage, retryCount))
	}

	// test writing and reading a new file in the pod with rbd mounted
	assert.NoError(s.T(), s.k8sh.WriteToPodRetry("", rbdPodName, newFileToWrite, simpleTestMessage, retryCount))
	assert.NoError(s.T(), s.k8sh.ReadFromPodRetry("", rbdPodName, newFileToWrite, simpleTestMessage, retryCount))

	// wait for filesystem to be active
	clusterInfo := client.AdminTestClusterInfo(s.namespace)
	err := waitForFilesystemActive(s.k8sh, clusterInfo, installer.FilesystemName)
	require.NoError(s.T(), err)

	// test reading preexisting files in the pod with cephfs mounted
	for _, file := range cephFSFilesToRead {
		assert.NoError(s.T(), s.k8sh.ReadFromPodRetry(s.namespace, filePodName, file, simpleTestMessage, retryCount))
	}

	// test writing and reading a new file in the pod with cephfs mounted
	assert.NoError(s.T(), s.k8sh.WriteToPodRetry(s.namespace, filePodName, newFileToWrite, simpleTestMessage, retryCount))
	assert.NoError(s.T(), s.k8sh.ReadFromPodRetry(s.namespace, filePodName, newFileToWrite, simpleTestMessage, retryCount))
}

// UpgradeToMaster performs the steps necessary to upgrade a Rook v1.4 cluster to master. It does not
// verify the upgrade but merely starts the upgrade process.
func (s *UpgradeSuite) upgradeToMaster() {
	// Apply the CRDs for the latest master
	s.settings.RookVersion = installer.LocalBuildTag
	s.installer.Manifests = installer.NewCephManifests(s.settings)

	if s.settings.UseHelm {
		logger.Info("Requiring msgr2 during helm upgrade to test the port conversion from 6789 to 3300")
		s.settings.RequireMsgr2 = true

		// Upgrade the operator chart
		err := s.installer.UpgradeRookOperatorViaHelm()
		require.NoError(s.T(), err, "failed to upgrade the operator chart")

		err = s.installer.UpgradeRookCephClusterViaHelm()
		require.NoError(s.T(), err, "failed to upgrade the cluster chart")
		return
	}

	require.NoError(s.T(), s.k8sh.ResourceOperation("apply", s.installer.Manifests.GetCRDs(s.k8sh)))

	require.NoError(s.T(), s.k8sh.ResourceOperation("apply", s.installer.Manifests.GetCommon()))

	require.NoError(s.T(),
		s.k8sh.SetDeploymentVersion(s.settings.OperatorNamespace, operatorContainer, operatorContainer, installer.LocalBuildTag))

	require.NoError(s.T(), s.k8sh.ResourceOperation("apply", s.installer.Manifests.GetToolbox()))
}
