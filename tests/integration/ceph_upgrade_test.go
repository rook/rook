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
	mons := 1
	rbdMirrorWorkers := 0
	s.op, s.k8sh = StartTestCluster(s.T,
		upgradeMinimalTestVersion,
		s.namespace,
		"",
		false,
		false,
		"",
		mons,
		rbdMirrorWorkers,
		installer.Version1_1,
		installer.MimicVersion,
	)
	s.helper = clients.CreateTestClient(s.k8sh, s.op.installer.Manifests)
}

func (s *UpgradeSuite) TearDownSuite() {
	s.op.Teardown()
}

func (s *UpgradeSuite) TestUpgradeToMaster() {
	checkSkipCSITest(s.Suite, s.k8sh)

	systemNamespace := installer.SystemNamespace(s.namespace)

	//
	// Create block, object, and file storage before the upgrade
	//
	poolName := "upgradepool"
	storageClassName := "block-upgrade"
	blockName := "block-claim-upgrade"
	logger.Infof("Initializing block before the upgrade")
	setupBlockLite(s.helper, s.k8sh, s.Suite, s.namespace, systemNamespace, poolName, storageClassName, blockName, rbdPodName, s.op.installer.CephVersion)

	createPodWithBlock(s.helper, s.k8sh, s.Suite, s.namespace, storageClassName, rbdPodName, blockName)

	// FIX: We should require block images to be removed. See tracking issue:
	// <INSERT ISSUE>
	requireBlockImagesRemoved := false
	defer blockTestDataCleanUp(s.helper, s.k8sh, s.Suite, s.namespace, poolName, storageClassName, blockName, rbdPodName, requireBlockImagesRemoved)

	// Create the filesystem, but wait to create the file test client until we upgrade to nautilus
	logger.Infof("Initializing file before the upgrade")
	filesystemName := "upgrade-test-fs"
	activeCount := 1
	createFilesystem(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName, activeCount)

	logger.Infof("Initializing object before the upgrade")
	objectStoreName := "upgraded-object"
	runObjectE2ETestLite(s.helper, s.k8sh, s.Suite, s.namespace, objectStoreName, 1)

	// verify that we're actually running the right pre-upgrade image
	s.verifyOperatorImage(installer.Version1_1)

	message := "my simple message"
	preFilename := "pre-upgrade-file"
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

	//
	// Upgrade Rook from v1.1 to v1.2
	//
	logger.Infof("*** UPGRADING ROOK FROM v1.1 to v1.2 ***")
	s.gatherLogs(systemNamespace, "_before_1.2_upgrade")
	s.upgradeToV1_2()

	s.verifyOperatorImage(installer.Version1_2)
	s.verifyRookUpgrade(numOSDs)
	logger.Infof("Done with automatic upgrade from v1.1 to v1.2")
	newFile := "post-upgrade-1_1-to-1_2-file"
	s.verifyFilesAfterUpgrade("", newFile, message, rbdFilesToRead, cephfsFilesToRead)
	rbdFilesToRead = append(rbdFilesToRead, newFile)
	logger.Infof("Verified upgrade from v1.1 to v1.2")

	//
	// Upgrade from mimic to nautilus
	//
	logger.Infof("*** UPGRADING CEPH FROM Mimic TO Nautilus ***")
	s.gatherLogs(systemNamespace, "_before_ceph_upgrade")
	s.upgradeCephVersion(installer.NautilusVersion.Image, numOSDs)

	// Start the file test client now that the CSI driver is supported on nautilus
	fsStorageClass := "file-upgrade"
	assert.NoError(s.T(), s.helper.FSClient.CreateStorageClass(filesystemName, s.namespace, fsStorageClass))
	useCSI := true
	createFilesystemConsumerPod(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName, fsStorageClass, useCSI)
	defer func() {
		cleanupFilesystemConsumer(s.helper, s.k8sh, s.Suite, s.namespace, filePodName)
		cleanupFilesystem(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)
	}()

	// Verify reading and writing to the test clients
	newFile = "post-ceph-upgrade-file"
	s.verifyFilesAfterUpgrade(filesystemName, newFile, message, rbdFilesToRead, cephfsFilesToRead)
	rbdFilesToRead = append(rbdFilesToRead, newFile)
	cephfsFilesToRead = append(cephfsFilesToRead, newFile)
	logger.Infof("Verified upgrade from mimic to nautilus")

	//
	// Upgrade Rook from v1.2 to master
	//
	logger.Infof("*** UPGRADING ROOK FROM v1.2 to master ***")
	s.gatherLogs(systemNamespace, "_before_master_upgrade")
	s.upgradeToMaster()

	s.verifyOperatorImage(installer.VersionMaster)
	s.verifyRookUpgrade(numOSDs)
	logger.Infof("Done with automatic upgrade from v1.2 to master")
	newFile = "post-upgrade-1_2-to-master-file"
	s.verifyFilesAfterUpgrade(filesystemName, newFile, message, rbdFilesToRead, cephfsFilesToRead)
	rbdFilesToRead = append(rbdFilesToRead, newFile)
	cephfsFilesToRead = append(cephfsFilesToRead, newFile)
	logger.Infof("Verified upgrade from v1.2 to master")
}

func (s *UpgradeSuite) gatherLogs(systemNamespace, testSuffix string) {
	// Gather logs before Ceph upgrade to help with debugging
	if installer.Env.Logs == "all" {
		s.k8sh.PrintPodDescribe(s.namespace)
	}
	n := strings.Replace(s.T().Name(), "/", "_", -1) + testSuffix
	s.op.installer.GatherAllRookLogs(n, systemNamespace, s.namespace)
}

func (s *UpgradeSuite) upgradeCephVersion(newCephImage string, numOSDs int) {
	osdDepList, err := k8sutil.GetDeployments(s.k8sh.Clientset, s.namespace, "app=rook-ceph-osd")
	require.NoError(s.T(), err)
	oldCephVersion := osdDepList.Items[0].Labels["ceph-version"] // upgraded OSDs should not have this version label

	//
	// Upgrade Ceph version, for example from Mimic to Nautilus.
	//
	s.k8sh.Kubectl("-n", s.namespace, "patch", "CephCluster", s.namespace, "--type=merge",
		"-p", fmt.Sprintf(`{"spec": {"cephVersion": {"image": "%s"}}}`, newCephImage))

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
	require.Equal(s.T(), s.op.mons, len(monDepList.Items), monDepList.Items)

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
	err := s.k8sh.WaitForDeploymentCount(monsNotOldVersion, s.namespace, s.op.mons)
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
		err := waitForFilesystemActive(s.k8sh, s.namespace, fsName)
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

// UpgradeToV1_2 performs the steps necessary to upgrade a Rook v1.1 cluster to v1.2. It does not
// verify the upgrade but merely starts the upgrade process.
func (s *UpgradeSuite) upgradeToV1_2() {
	require.NoError(s.T(), s.k8sh.ResourceOperation("apply", upgradeManifestTo1_2(s.namespace)))

	require.NoError(s.T(),
		s.k8sh.SetDeploymentVersion(installer.SystemNamespace(s.namespace), operatorContainer, operatorContainer, installer.Version1_2))
}

func upgradeManifestTo1_2(namespace string) string {
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
  namespace: ` + namespace + `
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
  namespace: ` + namespace + `
`
}

// UpgradeToMaster performs the steps necessary to upgrade a Rook v1.2 cluster to master. It does not
// verify the upgrade but merely starts the upgrade process.
func (s *UpgradeSuite) upgradeToMaster() {
	require.NoError(s.T(), s.k8sh.ResourceOperation("apply", upgradeManifestToMaster(s.namespace)))

	require.NoError(s.T(),
		s.k8sh.SetDeploymentVersion(installer.SystemNamespace(s.namespace), operatorContainer, operatorContainer, installer.VersionMaster))
}

func upgradeManifestToMaster(namespace string) string {
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
