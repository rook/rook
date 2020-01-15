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

	rookcephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
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
	cephVersion := rookcephv1.CephVersionSpec{
		// Ceph v13.2.3 got ceph-volume features needed for provisioning
		// test that when Rook upgrades, it can still run non-ceph-volume osds
		Image: "ceph/ceph:v13.2.0-20190410",
	}
	s.op, s.k8sh = StartTestCluster(s.T,
		upgradeMinimalTestVersion,
		s.namespace,
		"",
		false,
		useDevices,
		mons,
		rbdMirrorWorkers,
		installer.Version1_0,
		cephVersion,
	)
	s.helper = clients.CreateTestClient(s.k8sh, s.op.installer.Manifests)
}

func (s *UpgradeSuite) TearDownSuite() {
	s.op.Teardown()
}

func (s *UpgradeSuite) TestUpgradeToMaster() {
	systemNamespace := installer.SystemNamespace(s.namespace)

	//
	// Create block, object, and file storage on 1.0 before the upgrade
	//
	poolName := "upgradepool"
	storageClassName := "block-upgrade"
	blockName := "block-claim-upgrade"
	logger.Infof("Initializing block before the upgrade")
	setupBlockLite(s.helper, s.k8sh, s.Suite, s.namespace, poolName, storageClassName, blockName, rbdPodName, s.op.installer.CephVersion)
	createPodWithBlock(s.helper, s.k8sh, s.Suite, s.namespace, blockName, rbdPodName)
	defer blockTestDataCleanUp(s.helper, s.k8sh, s.Suite, s.namespace, poolName, storageClassName, blockName, rbdPodName)

	logger.Infof("Initializing file before the upgrade")
	filesystemName := "upgrade-test-fs"
	createFilesystem(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)
	createFilesystemConsumerPod(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)
	defer func() {
		cleanupFilesystemConsumer(s.k8sh, s.Suite, s.namespace, filePodName)
		cleanupFilesystem(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)
	}()

	logger.Infof("Initializing object before the upgrade")
	objectStoreName := "upgraded-object"
	runObjectE2ETestLite(s.helper, s.k8sh, s.Suite, s.namespace, objectStoreName, 1)

	// verify that we're actually running the right pre-upgrade image
	s.VerifyOperatorImage("rook/ceph:" + installer.Version1_0)

	message := "my simple message"
	preFilename := "pre-upgrade-file"
	assert.NoError(s.T(), s.k8sh.WriteToPod("", rbdPodName, preFilename, message))
	assert.NoError(s.T(), s.k8sh.ReadFromPod("", rbdPodName, preFilename, message))
	assert.NoError(s.T(), s.k8sh.WriteToPod(s.namespace, filePodName, preFilename, message))
	assert.NoError(s.T(), s.k8sh.ReadFromPod(s.namespace, filePodName, preFilename, message))

	// we will keep appending to this to continue verifying old files through the upgrades
	oldFilesToRead := []string{preFilename}

	// Gather logs before Ceph upgrade to help with debugging
	if installer.Env.Logs == "all" {
		s.k8sh.PrintPodDescribe(s.namespace)
	}
	n := strings.Replace(s.T().Name(), "/", "_", -1) + "_before_ceph_upgrade"
	s.op.installer.GatherAllRookLogs(n, systemNamespace, s.namespace)

	// Get some info about the currently deployed mons to determine later if they are all updated
	monDepList, err := k8sutil.GetDeployments(s.k8sh.Clientset, s.namespace, "app=rook-ceph-mon")
	require.NoError(s.T(), err)
	numMons := len(monDepList.Items)

	// Get some info about the currently deployed OSDs to determine later if they are all updated
	osdDepList, err := k8sutil.GetDeployments(s.k8sh.Clientset, s.namespace, "app=rook-ceph-osd")
	require.NoError(s.T(), err)
	osdDeps := osdDepList.Items
	numOSDs := len(osdDeps) // there should be this many upgraded OSDs
	require.True(s.T(), numOSDs > 0)
	d := osdDeps[0]
	oldCephVersion := d.Labels["ceph-version"] // upgraded OSDs should not have this version label

	//
	// Upgrade Ceph version from Mimic to Nautilus before upgrading to Rook v1.1
	//
	s.k8sh.Kubectl("-n", s.namespace, "patch", "CephCluster", s.namespace, "--type=merge",
		"-p", fmt.Sprintf(`{"spec": {"cephVersion": {"image": "%s"}}}`, installer.NautilusVersion.Image))

	// we need to make sure Ceph is fully updated (including RGWs and MDSes) before proceeding to
	// upgrade rook; we do not support upgrading Ceph simultaneously with Rook upgrade
	monsNotOldVersion := fmt.Sprintf("app=rook-ceph-mon,ceph-version!=%s", oldCephVersion)
	err = s.k8sh.WaitForDeploymentCount(monsNotOldVersion, s.namespace, numMons)
	require.NoError(s.T(), err)
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(monsNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	osdsNotOldVersion := fmt.Sprintf("app=rook-ceph-osd,ceph-version!=%s", oldCephVersion)
	// It can take a LONG time to update the mgr modules, so wait an extra long time here
	err = s.k8sh.WaitForDeploymentCountWithRetries(osdsNotOldVersion, s.namespace, numOSDs, utils.RetryLoop*2)
	require.NoError(s.T(), err)
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(osdsNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	mdsesNotOldVersion := fmt.Sprintf("app=rook-ceph-mds,ceph-version!=%s", oldCephVersion)
	err = s.k8sh.WaitForDeploymentCount(mdsesNotOldVersion, s.namespace, 4 /* always expect 4 mdses */)
	require.NoError(s.T(), err)
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(mdsesNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	rgwsNotOldVersion := fmt.Sprintf("app=rook-ceph-rgw,ceph-version!=%s", oldCephVersion)
	err = s.k8sh.WaitForDeploymentCount(rgwsNotOldVersion, s.namespace, 1 /* always expect 1 rgw */)
	require.NoError(s.T(), err)
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(rgwsNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	// Gather logs after Ceph upgrade to help with debugging
	if installer.Env.Logs == "all" {
		s.k8sh.PrintPodDescribe(s.namespace)
	}
	n = strings.Replace(s.T().Name(), "/", "_", -1) + "_after_ceph_upgrade"
	s.op.installer.GatherAllRookLogs(n, systemNamespace, s.namespace)

	//
	// Upgrade Rook from v1.0 to v1.1
	//
	m1 := installer.CephManifestsV1_1{
		K8sh:              s.k8sh,
		Namespace:         s.namespace,
		SystemNamespace:   systemNamespace,
		OperatorContainer: operatorContainer,
		T:                 s.T,
	}
	m1.UpgradeToV1_1()

	s.VerifyOperatorImage(m1.RookImage())
	s.VerifyRookUpgrade(numMons, numOSDs)
	logger.Infof("Done with automatic upgrade from v1.0 to v1.1")
	newFile := "post-upgrade-1_0-to-1_1-file"
	s.VerifyFilesAfterUpgrade(filesystemName, newFile, message, oldFilesToRead)
	oldFilesToRead = append(oldFilesToRead, newFile)
	logger.Infof("Verified upgrade from v1.0 to v1.1")

	//
	// Upgrade Rook from v1.1 to v1.2
	//
	m2 := installer.CephManifestsV1_2{
		K8sh:              s.k8sh,
		Namespace:         s.namespace,
		SystemNamespace:   systemNamespace,
		OperatorContainer: operatorContainer,
		T:                 s.T,
	}
	m2.UpgradeToV1_2()

	s.VerifyOperatorImage(m2.RookImage())
	s.VerifyRookUpgrade(numMons, numOSDs)
	logger.Infof("Done with automatic upgrade from v1.1 to v1.2")
	newFile = "post-upgrade-1_1-to-1_2-file"
	s.VerifyFilesAfterUpgrade(filesystemName, newFile, message, oldFilesToRead)
	oldFilesToRead = append(oldFilesToRead, newFile)
	logger.Infof("Verified upgrade from v1.1 to v1.2")

	//
	// Upgrade Rook from v1.2 to v1.3 (master)
	//
	m3 := installer.CephManifestsV1_3{
		K8sh:              s.k8sh,
		Namespace:         s.namespace,
		SystemNamespace:   systemNamespace,
		OperatorContainer: operatorContainer,
		T:                 s.T,
	}
	m3.UpgradeToV1_3()

	s.VerifyOperatorImage(m3.RookImage())
	s.VerifyRookUpgrade(numMons, numOSDs)
	logger.Infof("Done with automatic upgrade from v1.2 to v1.3")
	newFile = "post-upgrade-1_2-to-1_3-file"
	s.VerifyFilesAfterUpgrade(filesystemName, newFile, message, oldFilesToRead)
	oldFilesToRead = append(oldFilesToRead, newFile)
	logger.Infof("Verified upgrade from v1.2 to v1.3")
}

func (s *UpgradeSuite) VerifyOperatorImage(expectedImage string) {
	systemNamespace := installer.SystemNamespace(s.namespace)

	// verify that the operator spec is updated
	version, err := k8sutil.GetDeploymentImage(s.k8sh.Clientset, systemNamespace, operatorContainer, operatorContainer)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), expectedImage, version)
}

func (s *UpgradeSuite) VerifyRookUpgrade(numMons, numOSDs int) {
	// Get some info about the currently deployed mons to determine later if they are all updated
	monDepList, err := k8sutil.GetDeployments(s.k8sh.Clientset, s.namespace, "app=rook-ceph-mon")
	require.NoError(s.T(), err)
	require.Equal(s.T(), numMons, len(monDepList.Items))

	// Get some info about the currently deployed OSDs to determine later if they are all updated
	osdDepList, err := k8sutil.GetDeployments(s.k8sh.Clientset, s.namespace, "app=rook-ceph-osd")
	require.NoError(s.T(), err)
	require.NotZero(s.T(), len(osdDepList.Items))
	require.Equal(s.T(), numOSDs, len(osdDepList.Items))

	d := osdDepList.Items[0]
	oldRookVersion := d.Labels["rook-version"] // upgraded OSDs should not have this version label

	monsNotOldVersion := fmt.Sprintf("app=rook-ceph-mon,rook-version!=%s", oldRookVersion)
	err = s.k8sh.WaitForDeploymentCount(monsNotOldVersion, s.namespace, numMons)
	require.NoError(s.T(), err)
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(monsNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	// wait for the osd pods to be updated
	osdsNotOldVersion := fmt.Sprintf("app=rook-ceph-osd,rook-version!=%s", oldRookVersion)
	err = s.k8sh.WaitForDeploymentCount(osdsNotOldVersion, s.namespace, numOSDs)
	require.NoError(s.T(), err)
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(osdsNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	mdsesNotOldVersion := fmt.Sprintf("app=rook-ceph-mds,rook-version!=%s", oldRookVersion)
	err = s.k8sh.WaitForDeploymentCount(mdsesNotOldVersion, s.namespace, 4 /* always expect 4 mdses */)
	require.NoError(s.T(), err)
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(mdsesNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	// rgwsNotOldVersion
	// There is an unlikely corner case with RGWs where the upgrade will fail on upgrades regarding
	// a pool added for erasure coding. This hasn't been observed by users, so we ignore this
	// currently.

	// Give a few seconds for the daemons to settle down after the upgrade
	time.Sleep(5 * time.Second)
}

func (s *UpgradeSuite) VerifyFilesAfterUpgrade(fsName, newFileToWrite, messageForAllFiles string, oldFilesToRead []string) {
	retryCount := 5

	// wait for filesystem to be active
	err := waitForFilesystemActive(s.k8sh, s.namespace, fsName)
	require.NoError(s.T(), err)

	for _, file := range oldFilesToRead {
		// test reading preexisting files in the pod with cephfs mounted
		assert.NoError(s.T(), s.k8sh.ReadFromPodRetry(s.namespace, filePodName, file, messageForAllFiles, retryCount))

		// test reading preexisting files in the pod with rbd mounted
		// There is some unreliability right after the upgrade when there is only one osd, so we will retry if needed
		assert.NoError(s.T(), s.k8sh.ReadFromPodRetry("", rbdPodName, file, messageForAllFiles, retryCount))
	}

	// test writing and reading a new file in the pod with cephfs mounted
	assert.NoError(s.T(), s.k8sh.WriteToPodRetry(s.namespace, filePodName, newFileToWrite, messageForAllFiles, retryCount))
	assert.NoError(s.T(), s.k8sh.ReadFromPodRetry(s.namespace, filePodName, newFileToWrite, messageForAllFiles, retryCount))

	// test writing and reading a new file in the pod with rbd mounted
	assert.NoError(s.T(), s.k8sh.WriteToPodRetry("", rbdPodName, newFileToWrite, messageForAllFiles, retryCount))
	assert.NoError(s.T(), s.k8sh.ReadFromPodRetry("", rbdPodName, newFileToWrite, messageForAllFiles, retryCount))
}
