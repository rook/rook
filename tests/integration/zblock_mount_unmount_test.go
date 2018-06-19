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

	"fmt"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ******************************************************
// *** Scenarios tested by the BlockMountUnMountSuite ***
// Setup
// - via the cluster CRD
// - set up Block PVC - With ReadWriteOnce
// - set up Block PVC - with ReadWriteMany
// - Mount Volume on a pod and write some data
// - UnMount Volume
// Monitors
// - one mons in the cluster
// OSDs
// - Bluestore running on directory
// Block Mount & Unmount scenarios - repeat for each PVC
// 1. ReadWriteOnce
// 	  a. Mount Volume on a new pod - make sure persisted data is present and write new data
//    b. Mount volume on two pods with  - mount should be successful only on first pod
// 2. ReadOnlyMany
//   a. Mount Multiple pods with same volume - All pods should be able to read data
//   b. Mount Multiple pods with same volume - All pods should not be able to write data
// 3. Run StatefulSet with PVC
//	a. Scale up pods
//  b. Scale down pods
//  c. Failover pods
//  d. Delete StatefulSet
// ******************************************************

// NOTE: This suite needs to be last.
// There is an issue on k8s 1.7 where the CRD controller will frequently fail to create a cluster after this suite is run.
// The error is "the server does not allow this method on the requested resource (post clusters.ceph.rook.io)".
// Everything appears to have been cleaned up successfully in this test, so it is still unclear what is causing the issue between tests.
func TestBlockMountUnMountSuite(t *testing.T) {
	s := new(BlockMountUnMountSuite)
	defer func(s *BlockMountUnMountSuite) {
		HandlePanics(recover(), s.op, s.T)
	}(s)
	suite.Run(t, s)
}

type BlockMountUnMountSuite struct {
	suite.Suite
	testClient *clients.TestClient
	bc         *clients.BlockOperation
	kh         *utils.K8sHelper
	namespace  string
	pvcNameRWO string
	pvcNameRWX string
	installer  *installer.InstallHelper
	op         contracts.Setup
}

func (s *BlockMountUnMountSuite) SetupSuite() {

	s.namespace = "block-test-ns"
	s.pvcNameRWO = "block-persistent-rwo"
	s.pvcNameRWX = "block-persistent-rwx"
	useHelm := false
	useDevices := true
	s.op, s.kh = StartBaseTestOperations(s.T, s.namespace, "filestore", useHelm, useDevices, 1)
	s.testClient = GetTestClient(s.kh, s.namespace, s.op, s.T)
	s.bc = s.testClient.BlockClient
}

func (s *BlockMountUnMountSuite) setupPVCs() {
	logger.Infof("creating the test PVCs")
	poolNameRWO := "block-pool-rwo"
	storageClassNameRWO := "rook-ceph-block-rwo"

	// Create PVCs
	_, cbErr := installer.BlockResourceOperation(s.kh, installer.GetBlockPoolStorageClassAndPvcDef(s.namespace, poolNameRWO, storageClassNameRWO, s.pvcNameRWO, "ReadWriteOnce"), "create")
	require.Nil(s.T(), cbErr)
	require.True(s.T(), s.kh.WaitUntilPVCIsBound(defaultNamespace, s.pvcNameRWO), "Make sure PVC is Bound")

	_, cbErr2 := installer.BlockResourceOperation(s.kh, installer.GetBlockPvcDef(s.pvcNameRWX, storageClassNameRWO, "ReadWriteMany"), "create")
	require.Nil(s.T(), cbErr2)
	require.True(s.T(), s.kh.WaitUntilPVCIsBound(defaultNamespace, s.pvcNameRWX), "Make sure PVC is Bound")

	// Mount PVC on a pod and write some data.
	_, mtErr := s.bc.BlockMap(getBlockPodDefintion("setup-block-rwo", s.pvcNameRWO, false), blockMountPath)
	require.Nil(s.T(), mtErr)
	crdName, err := s.kh.GetVolumeResourceName(defaultNamespace, s.pvcNameRWO)
	require.Nil(s.T(), err)
	rwoVolumePresent := s.kh.IsVolumeResourcePresent(installer.SystemNamespace(s.namespace), crdName)
	if !rwoVolumePresent {
		s.kh.PrintPodDescribe("setup-block-rwo", defaultNamespace)
		s.kh.PrintPodStatus(s.namespace)
		s.kh.PrintPodStatus(installer.SystemNamespace(s.namespace))
	}
	require.True(s.T(), rwoVolumePresent, fmt.Sprintf("make sure rwo Volume %s is created", crdName))

	_, mtErr1 := s.bc.BlockMap(getBlockPodDefintion("setup-block-rwx", s.pvcNameRWX, false), blockMountPath)
	require.Nil(s.T(), mtErr1)
	crdName1, err1 := s.kh.GetVolumeResourceName(defaultNamespace, s.pvcNameRWX)
	require.Nil(s.T(), err1)
	require.True(s.T(), s.kh.IsVolumeResourcePresent(installer.SystemNamespace(s.namespace), crdName1), fmt.Sprintf("make sure rwx Volume %s is created", crdName))
	require.True(s.T(), s.kh.IsPodRunning("setup-block-rwo", defaultNamespace), "make sure setup-block-rwo pod is in running state")
	require.True(s.T(), s.kh.IsPodRunning("setup-block-rwx", defaultNamespace), "make sure setup-block-rwx pod is in running state")

	// Write Data to Pod
	_, wtErr1 := s.bc.Write("setup-block-rwo", blockMountPath, "Persisted message one", "bsFile1", "")
	require.Nil(s.T(), wtErr1)
	_, wtErr2 := s.bc.Write("setup-block-rwx", blockMountPath, "Persisted message one", "bsFile1", "")
	require.Nil(s.T(), wtErr2)

	// Unmount pod
	_, unmtErr1 := s.bc.BlockUnmap(getBlockPodDefintion("setup-block-rwo", s.pvcNameRWO, false), blockMountPath)
	_, unmtErr2 := s.bc.BlockUnmap(getBlockPodDefintion("setup-block-rwx", s.pvcNameRWO, false), blockMountPath)
	require.Nil(s.T(), unmtErr1)
	require.Nil(s.T(), unmtErr2)
	require.True(s.T(), s.kh.IsPodTerminated("setup-block-rwo", defaultNamespace), "make sure setup-block-rwo pod is terminated")
	require.True(s.T(), s.kh.IsPodTerminated("setup-block-rwx", defaultNamespace), "make sure setup-block-rwx pod is terminated")
}

func (s *BlockMountUnMountSuite) TearDownSuite() {
	logger.Infof("Cleaning up block storage")
	s.testClient.BlockClient.BlockUnmap(getBlockPodDefintion("setup-block-rwo", s.pvcNameRWO, false), blockMountPath)
	s.testClient.BlockClient.BlockUnmap(getBlockPodDefintion("setup-block-rwx", s.pvcNameRWX, false), blockMountPath)
	s.testClient.BlockClient.BlockUnmap(getBlockPodDefintion("rwo-block-rw-one", s.pvcNameRWO, false), blockMountPath)
	s.testClient.BlockClient.BlockUnmap(getBlockPodDefintion("rwo-block-rw-two", s.pvcNameRWO, false), blockMountPath)
	s.testClient.BlockClient.BlockUnmap(getBlockPodDefintion("rwo-block-ro-one", s.pvcNameRWO, true), blockMountPath)
	s.testClient.BlockClient.BlockUnmap(getBlockPodDefintion("rwo-block-ro-two", s.pvcNameRWO, true), blockMountPath)
	s.testClient.BlockClient.BlockUnmap(getBlockPodDefintion("rwx-block-rw-one", s.pvcNameRWX, false), blockMountPath)
	s.testClient.BlockClient.BlockUnmap(getBlockPodDefintion("rwx-block-rw-two", s.pvcNameRWX, false), blockMountPath)
	s.testClient.BlockClient.BlockUnmap(getBlockPodDefintion("rwx-block-ro-one", s.pvcNameRWX, true), blockMountPath)
	s.testClient.BlockClient.BlockUnmap(getBlockPodDefintion("rwx-block-ro-two", s.pvcNameRWX, true), blockMountPath)
	installer.BlockResourceOperation(s.kh, installer.GetBlockPoolStorageClassAndPvcDef(s.namespace, "block-pool-rwo", "rook-ceph-block-rwo", s.pvcNameRWO, "ReadWriteOnce"), "delete")
	installer.BlockResourceOperation(s.kh, installer.GetBlockPoolStorageClassAndPvcDef(s.namespace, "block-pool-rwx", "rook-ceph-block-rwx", s.pvcNameRWX, "ReadWriteMany"), "delete")

	cleanupDynamicBlockStorage(s.testClient, s.namespace)
	s.op.TearDown()
}

func (s *BlockMountUnMountSuite) TestBlockStorageMountUnMountForDifferentAccessModes() {
	s.setupPVCs()

	logger.Infof("Test case when existing RWO PVC is mounted and unmounted on pods with various accessModes")
	logger.Infof("Step 1.1: Mount existing ReadWriteOnce and ReadWriteMany PVC on a Pod with RW access")
	//mount PVC with RWO access on a pod with readonly set to false
	_, mtErr1 := s.bc.BlockMap(getBlockPodDefintion("rwo-block-rw-one", s.pvcNameRWO, false), blockMountPath)
	require.Nil(s.T(), mtErr1)
	//mount PVC with RWX access on a pod with readonly set to false
	_, mtErr2 := s.bc.BlockMap(getBlockPodDefintion("rwx-block-rw-one", s.pvcNameRWX, false), blockMountPath)
	require.Nil(s.T(), mtErr2)
	crdName1, err1 := s.kh.GetVolumeResourceName(defaultNamespace, s.pvcNameRWO)
	crdName2, err2 := s.kh.GetVolumeResourceName(defaultNamespace, s.pvcNameRWX)
	assert.Nil(s.T(), err1)
	assert.Nil(s.T(), err2)

	assert.True(s.T(), s.kh.IsVolumeResourcePresent(installer.SystemNamespace(s.namespace), crdName1), fmt.Sprintf("make sure Volume %s is created", crdName1))
	assert.True(s.T(), s.kh.IsPodRunning("rwo-block-rw-one", defaultNamespace), "make sure block-rw-one pod is in running state")
	assert.True(s.T(), s.kh.IsVolumeResourcePresent(installer.SystemNamespace(s.namespace), crdName2), fmt.Sprintf("make sure Volume %s is created", crdName2))
	assert.True(s.T(), s.kh.IsPodRunning("rwx-block-rw-one", defaultNamespace), "make sure rwx-block-rw-one pod is in running state")

	logger.Infof("Step 2: Check if previously persisted data is readable from ReadWriteOnce and ReadWriteMany PVC")
	//Read data on RWO PVC Mounted on pod with RW Access
	read1, rErr1 := s.bc.Read("rwo-block-rw-one", blockMountPath, "bsFile1", "")
	assert.Nil(s.T(), rErr1)
	assert.Contains(s.T(), read1, "Persisted message one", "make sure previously persisted data is readable")
	//Read data on RWX PVC Mounted on pod with RW Access
	read2, rErr2 := s.bc.Read("rwx-block-rw-one", blockMountPath, "bsFile1", "")
	assert.Nil(s.T(), rErr2)
	assert.Contains(s.T(), read2, "Persisted message one", "make sure previously persisted data is readable")

	logger.Infof("Step 3: Check if read/write works on ReadWriteOnce and ReadWriteMany PVC")
	//Write data on RWO PVC Mounted on pod with RW Access
	_, wtErr1 := s.bc.Write("rwo-block-rw-one", blockMountPath, "Persisted message two", "bsFile2", "")
	assert.Nil(s.T(), wtErr1)
	//Read data on RWO PVC Mounted on pod with RW Access
	read1, rErr1 = s.bc.Read("rwo-block-rw-one", blockMountPath, "bsFile2", "")
	assert.Nil(s.T(), rErr1)
	assert.Contains(s.T(), read1, "Persisted message two", "make sure new persisted data is readable")
	//Write data on RWX PVC Mounted on pod with RW Access
	_, wtErr2 := s.bc.Write("rwx-block-rw-one", blockMountPath, "Persisted message two", "bsFile2", "")
	assert.Nil(s.T(), wtErr2)
	//Read data on RWX PVC Mounted on pod with RW Access
	read2, rErr2 = s.bc.Read("rwx-block-rw-one", blockMountPath, "bsFile2", "")
	assert.Nil(s.T(), rErr2)
	assert.Contains(s.T(), read2, "Persisted message two", "make sure new persisted data is readable")

	//Mount another Pod with RW access on same PVC
	logger.Infof("Step 4: Mount existing ReadWriteOnce and ReadWriteMany PVC on a new Pod with RW access")
	//Mount RWO PVC on a new pod with ReadOnly set to false
	_, mtErr1 = s.bc.BlockMap(getBlockPodDefintion("rwo-block-rw-two", s.pvcNameRWO, false), blockMountPath)
	assert.Nil(s.T(), mtErr1)
	//Mount RWX PVC on a new pod with ReadOnly set to false
	_, mtErr2 = s.bc.BlockMap(getBlockPodDefintion("rwx-block-rw-two", s.pvcNameRWX, false), blockMountPath)
	assert.Nil(s.T(), mtErr2)
	assert.True(s.T(), s.kh.IsPodInError("rwo-block-rw-two", defaultNamespace, "FailedMount", "Volume is already attached by pod"), "make sure rwo-block-rw-two pod errors out while mounting the volume")
	assert.True(s.T(), s.kh.IsPodInError("rwx-block-rw-two", defaultNamespace, "FailedMount", "Volume is already attached by pod"), "make sure rwx-block-rw-two pod errors out while mounting the volume")
	_, unmtErr1 := s.bc.BlockUnmap(getBlockPodDefintion("rwo-block-rw-two", s.pvcNameRWO, false), blockMountPath)
	assert.Nil(s.T(), unmtErr1)

	_, unmtErr2 := s.bc.BlockUnmap(getBlockPodDefintion("rwx-block-rw-two", s.pvcNameRWX, false), blockMountPath)
	assert.Nil(s.T(), unmtErr2)
	assert.True(s.T(), s.kh.IsPodTerminated("rwo-block-rw-two", defaultNamespace), "make sure rwo-block-rw-two pod is terminated")
	assert.True(s.T(), s.kh.IsPodTerminated("rwx-block-rw-two", defaultNamespace), "make sure rwx-block-rw-two pod is terminated")

	logger.Infof("Step 5: Mount existing ReadWriteOnce and ReadWriteMany PVC on a new Pod with RO access")
	//Mount RWO PVC on a new pod with ReadOnly set to true
	_, mtErr1 = s.bc.BlockMap(getBlockPodDefintion("rwo-block-ro-one", s.pvcNameRWO, true), blockMountPath)
	assert.Nil(s.T(), mtErr1)
	//Mount RWX PVC on a new pod with ReadOnly set to true
	_, mtErr2 = s.bc.BlockMap(getBlockPodDefintion("rwx-block-ro-one", s.pvcNameRWX, true), blockMountPath)
	assert.Nil(s.T(), mtErr2)
	assert.True(s.T(), s.kh.IsPodInError("rwo-block-ro-one", defaultNamespace, "FailedMount", "Volume is already attached by pod"), "make sure rwo-block-ro-one pod errors out while mounting the volume")
	assert.True(s.T(), s.kh.IsPodInError("rwx-block-ro-one", defaultNamespace, "FailedMount", "Volume is already attached by pod"), "make sure rwx-block-ro-one pod errors out while mounting the volume")
	_, unmtErr1 = s.bc.BlockUnmap(getBlockPodDefintion("rwo-block-ro-one", s.pvcNameRWO, true), blockMountPath)
	assert.Nil(s.T(), unmtErr1)
	_, unmtErr2 = s.bc.BlockUnmap(getBlockPodDefintion("rwx-block-ro-one", s.pvcNameRWX, true), blockMountPath)
	assert.Nil(s.T(), unmtErr2)
	assert.True(s.T(), s.kh.IsPodTerminated("rwo-block-ro-one", defaultNamespace), "make sure rwo-block-ro-one pod is terminated")
	assert.True(s.T(), s.kh.IsPodTerminated("rwx-block-ro-one", defaultNamespace), "make sure rwx-block-ro-one pod is terminated")

	logger.Infof("Step 6: UnMount Pod with RWX and RWO access")
	//UnMount RWO PVC
	_, unmtErr1 = s.bc.BlockUnmap(getBlockPodDefintion("rwo-block-rw-one", s.pvcNameRWO, false), blockMountPath)
	assert.Nil(s.T(), unmtErr1)
	//UnMount RWX PVC
	_, unmtErr2 = s.bc.BlockUnmap(getBlockPodDefintion("rwx-block-rw-one", s.pvcNameRWX, false), blockMountPath)
	assert.Nil(s.T(), unmtErr2)
	assert.True(s.T(), s.kh.IsPodTerminated("rwo-block-rw-one", defaultNamespace), "make sure rwo-block-rw-one pod is terminated")
	assert.True(s.T(), s.kh.IsPodTerminated("rwx-block-rw-one", defaultNamespace), "make sure rwx-lock-rw-one pod is terminated")

	logger.Infof("Step 7: Mount ReadWriteOnce and ReadWriteMany PVC on two different pods with ReadOnlyMany with Readonly Access")
	//Mount RWO PVC on 2 pods with ReadOnly set to True
	_, mtErr1_1 := s.bc.BlockMap(getBlockPodDefintion("rwo-block-ro-one", s.pvcNameRWO, true), blockMountPath)
	_, mtErr2_1 := s.bc.BlockMap(getBlockPodDefintion("rwo-block-ro-two", s.pvcNameRWO, true), blockMountPath)
	assert.Nil(s.T(), mtErr1_1)
	assert.Nil(s.T(), mtErr2_1)
	//Mount RWX PVC on 2 pods with ReadOnly set to True
	_, mtErr1_2 := s.bc.BlockMap(getBlockPodDefintion("rwx-block-ro-one", s.pvcNameRWX, true), blockMountPath)
	_, mtErr2_2 := s.bc.BlockMap(getBlockPodDefintion("rwx-block-ro-two", s.pvcNameRWX, true), blockMountPath)
	assert.Nil(s.T(), mtErr1_2)
	assert.Nil(s.T(), mtErr2_2)
	assert.True(s.T(), s.kh.IsPodRunning("rwo-block-ro-one", defaultNamespace), "make sure rwo-block-ro-one pod is in running state")
	assert.True(s.T(), s.kh.IsPodRunning("rwo-block-ro-two", defaultNamespace), "make sure rwo-block-ro-two pod is in running state")
	assert.True(s.T(), s.kh.IsPodRunning("rwx-block-ro-one", defaultNamespace), "make sure rwx-block-ro-one pod is in running state")
	assert.True(s.T(), s.kh.IsPodRunning("rwx-block-ro-two", defaultNamespace), "make sure rwx-block-ro-two pod is in running state")

	logger.Infof("Step 8: Read Data from both ReadyOnlyMany and ReadWriteOnce pods with ReadOnly Access")
	//Read data from RWO PVC via both ReadOnly pods
	read1_1, rErr1_1 := s.bc.Read("rwo-block-ro-one", blockMountPath, "bsFile1", "")
	read2_1, rErr2_1 := s.bc.Read("rwo-block-ro-two", blockMountPath, "bsFile1", "")
	assert.Nil(s.T(), rErr1_1)
	assert.Nil(s.T(), rErr2_1)
	assert.Contains(s.T(), read1_1, "Persisted message one", "make sure previously persisted data is readable from ReadOnlyMany access Pod")
	assert.Contains(s.T(), read2_1, "Persisted message one", "make sure previously persisted data is readable from ReadOnlyMany access Pod")
	//Read data from RWX PVC via both ReadOnly pods
	read1_2, rErr1_2 := s.bc.Read("rwx-block-ro-one", blockMountPath, "bsFile1", "")
	read2_2, rErr2_2 := s.bc.Read("rwx-block-ro-two", blockMountPath, "bsFile1", "")
	assert.Nil(s.T(), rErr1_2)
	assert.Nil(s.T(), rErr2_2)
	assert.Contains(s.T(), read1_2, "Persisted message one", "make sure previously persisted data is readable from ReadOnlyMany access Pod")
	assert.Contains(s.T(), read2_2, "Persisted message one", "make sure previously persisted data is readable from ReadOnlyMany access Pod")

	logger.Infof("Step 9: Write Data to Pod with ReadOnlyMany and ReadWriteOnce PVC  mounted with ReadOnly access")
	//Write data to RWO PVC via pod with ReadOnly Set to true
	_, wtErr1 = s.bc.Write("rwo-block-ro-one", blockMountPath, "Persisted message three", "bsFile3", "")
	assert.Contains(s.T(), wtErr1.Error(), "Unable to write data to pod")
	//Write data to RWx PVC via pod with ReadOnly Set to true
	_, wtErr2 = s.bc.Write("rwx-block-ro-one", blockMountPath, "Persisted message three", "bsFile3", "")
	assert.Contains(s.T(), wtErr2.Error(), "Unable to write data to pod")

	logger.Infof("Step 10: UnMount Pod with ReadOnlyMany and ReadWriteOnce PVCs")
	//UnMount RWO PVC from both ReadOnly Pods
	_, unmtErr1_1 := s.bc.BlockUnmap(getBlockPodDefintion("rwo-block-ro-one", s.pvcNameRWO, true), blockMountPath)
	_, unmtErr2_1 := s.bc.BlockUnmap(getBlockPodDefintion("rwo-block-ro-two", s.pvcNameRWO, true), blockMountPath)
	assert.Nil(s.T(), unmtErr1_1)
	assert.Nil(s.T(), unmtErr2_1)
	//UnMount RWX PVC from both ReadOnly Pods
	_, unmtErr1_2 := s.bc.BlockUnmap(getBlockPodDefintion("rwx-block-ro-one", s.pvcNameRWX, true), blockMountPath)
	_, unmtErr2_2 := s.bc.BlockUnmap(getBlockPodDefintion("rwx-block-ro-two", s.pvcNameRWX, true), blockMountPath)
	assert.Nil(s.T(), unmtErr1_2)
	assert.Nil(s.T(), unmtErr2_2)
	assert.True(s.T(), s.kh.IsPodTerminated("rwo-block-ro-one", defaultNamespace), "make sure rwo-block-ro-one pod is terminated")
	assert.True(s.T(), s.kh.IsPodTerminated("rwo-block-ro-two", defaultNamespace), "make sure rwo-block-ro-two pod is terminated")
	assert.True(s.T(), s.kh.IsPodTerminated("rwx-block-ro-one", defaultNamespace), "make sure rwx-lock-ro-one pod is terminated")
	assert.True(s.T(), s.kh.IsPodTerminated("rwx-block-ro-two", defaultNamespace), "make sure rwx-block-ro-two pod is terminated")

}
