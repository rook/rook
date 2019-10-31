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
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
// The error is "the server does not allow this method on the requested resource (post cephclusters.ceph.rook.io)".
// Everything appears to have been cleaned up successfully in this test, so it is still unclear what is causing the issue between tests.
func TestCephBlockSuite(t *testing.T) {
	if installer.SkipTestSuite(installer.CephTestSuite) {
		t.Skip()
	}

	s := new(CephBlockSuite)
	defer func(s *CephBlockSuite) {
		HandlePanics(recover(), s.op, s.T)
	}(s)
	suite.Run(t, s)
}

type CephBlockSuite struct {
	suite.Suite
	testClient *clients.TestClient
	bc         *clients.BlockOperation
	kh         *utils.K8sHelper
	namespace  string
	pvcNameRWO string
	pvcNameRWX string
	installer  *installer.CephInstaller
	op         *TestCluster
}

func (s *CephBlockSuite) SetupSuite() {

	s.namespace = "block-test-ns"
	s.pvcNameRWO = "block-persistent-rwo"
	s.pvcNameRWX = "block-persistent-rwx"
	useHelm := false
	useDevices := true
	mons := 1
	rbdMirrorWorkers := 1
	s.op, s.kh = StartTestCluster(s.T, blockMinimalTestVersion, s.namespace, "filestore", useHelm, useDevices, mons, rbdMirrorWorkers, installer.VersionMaster, installer.NautilusVersion)
	s.testClient = clients.CreateTestClient(s.kh, s.op.installer.Manifests)
	s.bc = s.testClient.BlockClient
}

func (s *CephBlockSuite) AfterTest(suiteName, testName string) {
	s.op.installer.CollectOperatorLog(suiteName, testName, installer.SystemNamespace(s.namespace))
}

func (s *CephBlockSuite) TestBlockStorageMountUnMountForStatefulSets() {
	poolName := "stspool"
	storageClassName := "stssc"
	reclaimPolicy := "Delete"
	statefulSetName := "block-stateful-set"
	statefulPodsName := "ststest"

	defer s.statefulSetDataCleanup(defaultNamespace, poolName, storageClassName, reclaimPolicy, statefulSetName, statefulPodsName)
	logger.Infof("Test case when block persistent volumes are scaled up and down along with StatefulSet")
	logger.Info("Step 1: Create pool and storageClass")

	err := s.testClient.PoolClient.CreateStorageClass(s.namespace, poolName, storageClassName, reclaimPolicy)
	assert.Nil(s.T(), err)
	logger.Info("Step 2 : Deploy statefulSet with 1X replication")
	service, statefulset := getBlockStatefulSetAndServiceDefinition(defaultNamespace, statefulSetName, statefulPodsName, storageClassName)
	_, err = s.kh.Clientset.CoreV1().Services(defaultNamespace).Create(service)
	assert.Nil(s.T(), err)
	_, err = s.kh.Clientset.AppsV1().StatefulSets(defaultNamespace).Create(statefulset)
	assert.Nil(s.T(), err)
	require.True(s.T(), s.kh.CheckPodCountAndState(statefulSetName, defaultNamespace, 1, "Running"))
	require.True(s.T(), s.kh.CheckPvcCountAndStatus(statefulSetName, defaultNamespace, 1, "Bound"))

	logger.Info("Step 3 : Scale up replication on statefulSet")
	scaleerr := s.kh.ScaleStatefulSet(statefulPodsName, defaultNamespace, 2)
	assert.NoError(s.T(), scaleerr, "make sure scale up is successful")
	require.True(s.T(), s.kh.CheckPodCountAndState(statefulSetName, defaultNamespace, 2, "Running"))
	require.True(s.T(), s.kh.CheckPvcCountAndStatus(statefulSetName, defaultNamespace, 2, "Bound"))

	logger.Info("Step 4 : Scale down replication on statefulSet")
	scaleerr = s.kh.ScaleStatefulSet(statefulPodsName, defaultNamespace, 1)
	assert.NoError(s.T(), scaleerr, "make sure scale down is successful")
	require.True(s.T(), s.kh.CheckPodCountAndState(statefulSetName, defaultNamespace, 1, "Running"))
	require.True(s.T(), s.kh.CheckPvcCountAndStatus(statefulSetName, defaultNamespace, 2, "Bound"))

	logger.Info("Step 5 : Delete statefulSet")
	delOpts := metav1.DeleteOptions{}
	listOpts := metav1.ListOptions{LabelSelector: "app=" + statefulSetName}
	err = s.kh.Clientset.CoreV1().Services(defaultNamespace).Delete(statefulSetName, &delOpts)
	assert.Nil(s.T(), err)
	err = s.kh.Clientset.AppsV1().StatefulSets(defaultNamespace).Delete(statefulPodsName, &delOpts)
	assert.Nil(s.T(), err)
	err = s.kh.Clientset.CoreV1().Pods(defaultNamespace).DeleteCollection(&delOpts, listOpts)
	assert.Nil(s.T(), err)
	require.True(s.T(), s.kh.WaitUntilPodWithLabelDeleted(fmt.Sprintf("app=%s", statefulSetName), defaultNamespace))
	require.True(s.T(), s.kh.CheckPvcCountAndStatus(statefulSetName, defaultNamespace, 2, "Bound"))
}

func (s *CephBlockSuite) statefulSetDataCleanup(namespace, poolName, storageClassName, reclaimPolicy, statefulSetName, statefulPodsName string) {
	delOpts := metav1.DeleteOptions{}
	listOpts := metav1.ListOptions{LabelSelector: "app=" + statefulSetName}
	// Delete stateful set
	s.kh.Clientset.CoreV1().Services(namespace).Delete(statefulSetName, &delOpts)
	s.kh.Clientset.AppsV1().StatefulSets(defaultNamespace).Delete(statefulPodsName, &delOpts)
	s.kh.Clientset.CoreV1().Pods(defaultNamespace).DeleteCollection(&delOpts, listOpts)
	// Delete all PVCs
	s.kh.DeletePvcWithLabel(defaultNamespace, statefulSetName)
	// Delete storageclass and pool
	s.testClient.PoolClient.DeletePool(s.testClient.BlockClient, s.namespace, poolName)
	s.testClient.PoolClient.DeleteStorageClass(storageClassName)
}

func (s *CephBlockSuite) setupPVCs() {
	logger.Infof("creating the test PVCs")
	poolNameRWO := "block-pool-rwo"
	storageClassNameRWO := "rook-ceph-block-rwo"

	// Create PVCs
	cbErr := s.testClient.PoolClient.CreateStorageClassAndPvc(s.namespace, poolNameRWO, storageClassNameRWO, "Delete", s.pvcNameRWO, "ReadWriteOnce")
	require.Nil(s.T(), cbErr)
	require.True(s.T(), s.kh.WaitUntilPVCIsBound(defaultNamespace, s.pvcNameRWO), "Make sure PVC is Bound")

	cbErr2 := s.testClient.BlockClient.CreatePvc(s.pvcNameRWX, storageClassNameRWO, "ReadWriteMany", "1M")
	require.Nil(s.T(), cbErr2)
	require.True(s.T(), s.kh.WaitUntilPVCIsBound(defaultNamespace, s.pvcNameRWX), "Make sure PVC is Bound")

	// Mount PVC on a pod and write some data.
	_, mtErr := s.bc.BlockMap(getBlockPodDefinition("setup-block-rwo", s.pvcNameRWO, false))
	require.Nil(s.T(), mtErr)
	crdName, err := s.kh.GetVolumeResourceName(defaultNamespace, s.pvcNameRWO)
	require.Nil(s.T(), err)
	rwoVolumePresent := s.kh.IsVolumeResourcePresent(installer.SystemNamespace(s.namespace), crdName)
	if !rwoVolumePresent {
		s.kh.PrintPodDescribe(defaultNamespace, "setup-block-rwo")
		s.kh.PrintPodStatus(s.namespace)
		s.kh.PrintPodStatus(installer.SystemNamespace(s.namespace))
	}
	require.True(s.T(), rwoVolumePresent, fmt.Sprintf("make sure rwo Volume %s is created", crdName))

	_, mtErr1 := s.bc.BlockMap(getBlockPodDefinition("setup-block-rwx", s.pvcNameRWX, false))
	require.Nil(s.T(), mtErr1)
	crdName1, err1 := s.kh.GetVolumeResourceName(defaultNamespace, s.pvcNameRWX)
	require.Nil(s.T(), err1)
	require.True(s.T(), s.kh.IsVolumeResourcePresent(installer.SystemNamespace(s.namespace), crdName1), fmt.Sprintf("make sure rwx Volume %s is created", crdName))
	require.True(s.T(), s.kh.IsPodRunning("setup-block-rwo", defaultNamespace), "make sure setup-block-rwo pod is in running state")
	require.True(s.T(), s.kh.IsPodRunning("setup-block-rwx", defaultNamespace), "make sure setup-block-rwx pod is in running state")

	// Write Data to Pod
	message := "Persisted message one"
	filename := "bsFile1"
	err = s.kh.WriteToPod("", "setup-block-rwo", filename, message)
	require.Nil(s.T(), err)
	err = s.kh.WriteToPod("", "setup-block-rwx", filename, message)
	require.Nil(s.T(), err)

	// Unmount pod
	_, err = s.kh.DeletePods("setup-block-rwo", "setup-block-rwx")
	require.Nil(s.T(), err)
	require.True(s.T(), s.kh.IsPodTerminated("setup-block-rwo", defaultNamespace), "make sure setup-block-rwo pod is terminated")
	require.True(s.T(), s.kh.IsPodTerminated("setup-block-rwx", defaultNamespace), "make sure setup-block-rwx pod is terminated")
}

func (s *CephBlockSuite) TearDownSuite() {
	logger.Infof("Cleaning up block storage")

	s.kh.DeletePods(
		"setup-block-rwo", "setup-block-rwx", "rwo-block-rw-one", "rwo-block-rw-two", "rwo-block-ro-one",
		"rwo-block-ro-two", "rwx-block-rw-one", "rwx-block-rw-two", "rwx-block-ro-one", "rwx-block-ro-two")

	s.testClient.PoolClient.DeletePvc(s.namespace, s.pvcNameRWO)
	s.testClient.PoolClient.DeletePvc(s.namespace, s.pvcNameRWX)
	s.testClient.PoolClient.DeleteStorageClass("rook-ceph-block-rwo")
	s.testClient.PoolClient.DeleteStorageClass("rook-ceph-block-rwx")
	s.testClient.PoolClient.DeletePool(s.testClient.BlockClient, s.namespace, "block-pool-rwo")
	s.testClient.PoolClient.DeletePool(s.testClient.BlockClient, s.namespace, "block-pool-rwx")
	s.op.Teardown()
}

func (s *CephBlockSuite) TestBlockStorageMountUnMountForDifferentAccessModes() {
	s.setupPVCs()

	logger.Infof("Test case when existing RWO PVC is mounted and unmounted on pods with various accessModes")
	logger.Infof("Step 1.1: Mount existing ReadWriteOnce and ReadWriteMany PVC on a Pod with RW access")
	// mount PVC with RWO access on a pod with readonly set to false
	_, err := s.bc.BlockMap(getBlockPodDefinition("rwo-block-rw-one", s.pvcNameRWO, false))
	require.Nil(s.T(), err)
	// mount PVC with RWX access on a pod with readonly set to false
	_, err = s.bc.BlockMap(getBlockPodDefinition("rwx-block-rw-one", s.pvcNameRWX, false))
	require.Nil(s.T(), err)
	crdName1, err1 := s.kh.GetVolumeResourceName(defaultNamespace, s.pvcNameRWO)
	crdName2, err2 := s.kh.GetVolumeResourceName(defaultNamespace, s.pvcNameRWX)
	assert.Nil(s.T(), err1)
	assert.Nil(s.T(), err2)

	assert.True(s.T(), s.kh.IsVolumeResourcePresent(installer.SystemNamespace(s.namespace), crdName1), fmt.Sprintf("make sure Volume %s is created", crdName1))
	assert.True(s.T(), s.kh.IsPodRunning("rwo-block-rw-one", defaultNamespace), "make sure block-rw-one pod is in running state")
	assert.True(s.T(), s.kh.IsVolumeResourcePresent(installer.SystemNamespace(s.namespace), crdName2), fmt.Sprintf("make sure Volume %s is created", crdName2))
	assert.True(s.T(), s.kh.IsPodRunning("rwx-block-rw-one", defaultNamespace), "make sure rwx-block-rw-one pod is in running state")

	logger.Infof("Step 2: Check if previously persisted data is readable from ReadWriteOnce and ReadWriteMany PVC")
	// Read data on RWO PVC Mounted on pod with RW Access
	filename1 := "bsFile1"
	message1 := "Persisted message one"
	err = s.kh.ReadFromPod("", "rwo-block-rw-one", filename1, message1)
	assert.Nil(s.T(), err)

	// Read data on RWX PVC Mounted on pod with RW Access
	err = s.kh.ReadFromPod("", "rwx-block-rw-one", filename1, message1)
	assert.Nil(s.T(), err)

	logger.Infof("Step 3: Check if read/write works on ReadWriteOnce and ReadWriteMany PVC")
	// Write data on RWO PVC Mounted on pod with RW Access
	filename2 := "bsFile2"
	message2 := "Persisted message two"
	assert.Nil(s.T(), s.kh.WriteToPod("", "rwo-block-rw-one", filename2, message2))

	// Read data on RWO PVC Mounted on pod with RW Access
	assert.Nil(s.T(), s.kh.ReadFromPod("", "rwo-block-rw-one", filename2, message2))

	// Write data on RWX PVC Mounted on pod with RW Access
	assert.Nil(s.T(), s.kh.WriteToPod("", "rwx-block-rw-one", filename2, message2))

	// Read data on RWX PVC Mounted on pod with RW Access
	assert.Nil(s.T(), s.kh.ReadFromPod("", "rwx-block-rw-one", filename2, message2))

	// Mount another Pod with RW access on same PVC
	logger.Infof("Step 4: Mount existing ReadWriteOnce and ReadWriteMany PVC on a new Pod with RW access")
	// Mount RWO PVC on a new pod with ReadOnly set to false
	_, err = s.bc.BlockMap(getBlockPodDefinition("rwo-block-rw-two", s.pvcNameRWO, false))
	assert.Nil(s.T(), err)
	// Mount RWX PVC on a new pod with ReadOnly set to false
	_, err = s.bc.BlockMap(getBlockPodDefinition("rwx-block-rw-two", s.pvcNameRWX, false))
	assert.Nil(s.T(), err)
	assert.True(s.T(), s.kh.IsPodInError("rwo-block-rw-two", defaultNamespace, "FailedMount", "Volume is already attached by pod"), "make sure rwo-block-rw-two pod errors out while mounting the volume")
	assert.True(s.T(), s.kh.IsPodInError("rwx-block-rw-two", defaultNamespace, "FailedMount", "Volume is already attached by pod"), "make sure rwx-block-rw-two pod errors out while mounting the volume")
	_, err = s.kh.DeletePods("rwo-block-rw-two", "rwx-block-rw-two")
	assert.Nil(s.T(), err)
	assert.True(s.T(), s.kh.IsPodTerminated("rwo-block-rw-two", defaultNamespace), "make sure rwo-block-rw-two pod is terminated")
	assert.True(s.T(), s.kh.IsPodTerminated("rwx-block-rw-two", defaultNamespace), "make sure rwx-block-rw-two pod is terminated")

	logger.Infof("Step 5: Mount existing ReadWriteOnce and ReadWriteMany PVC on a new Pod with RO access")
	// Mount RWO PVC on a new pod with ReadOnly set to true
	_, err = s.bc.BlockMap(getBlockPodDefinition("rwo-block-ro-one", s.pvcNameRWO, true))
	assert.Nil(s.T(), err)
	// Mount RWX PVC on a new pod with ReadOnly set to true
	_, err = s.bc.BlockMap(getBlockPodDefinition("rwx-block-ro-one", s.pvcNameRWX, true))
	assert.Nil(s.T(), err)
	assert.True(s.T(), s.kh.IsPodInError("rwo-block-ro-one", defaultNamespace, "FailedMount", "Volume is already attached by pod"), "make sure rwo-block-ro-one pod errors out while mounting the volume")
	assert.True(s.T(), s.kh.IsPodInError("rwx-block-ro-one", defaultNamespace, "FailedMount", "Volume is already attached by pod"), "make sure rwx-block-ro-one pod errors out while mounting the volume")
	_, err = s.kh.DeletePods("rwo-block-ro-one", "rwx-block-ro-one")
	assert.Nil(s.T(), err)
	assert.True(s.T(), s.kh.IsPodTerminated("rwo-block-ro-one", defaultNamespace), "make sure rwo-block-ro-one pod is terminated")
	assert.True(s.T(), s.kh.IsPodTerminated("rwx-block-ro-one", defaultNamespace), "make sure rwx-block-ro-one pod is terminated")

	logger.Infof("Step 6: UnMount Pod with RWX and RWO access")
	_, err = s.kh.DeletePods("rwo-block-rw-one", "rwx-block-rw-one")
	assert.Nil(s.T(), err)
	assert.True(s.T(), s.kh.IsPodTerminated("rwo-block-rw-one", defaultNamespace), "make sure rwo-block-rw-one pod is terminated")
	assert.True(s.T(), s.kh.IsPodTerminated("rwx-block-rw-one", defaultNamespace), "make sure rwx-lock-rw-one pod is terminated")

	logger.Infof("Step 7: Mount ReadWriteOnce and ReadWriteMany PVC on two different pods with ReadOnlyMany with Readonly Access")
	// Mount RWO PVC on 2 pods with ReadOnly set to True
	_, err1 = s.bc.BlockMap(getBlockPodDefinition("rwo-block-ro-one", s.pvcNameRWO, true))
	_, err2 = s.bc.BlockMap(getBlockPodDefinition("rwo-block-ro-two", s.pvcNameRWO, true))
	assert.Nil(s.T(), err1)
	assert.Nil(s.T(), err2)
	// Mount RWX PVC on 2 pods with ReadOnly set to True
	_, err1 = s.bc.BlockMap(getBlockPodDefinition("rwx-block-ro-one", s.pvcNameRWX, true))
	_, err2 = s.bc.BlockMap(getBlockPodDefinition("rwx-block-ro-two", s.pvcNameRWX, true))
	assert.Nil(s.T(), err1)
	assert.Nil(s.T(), err2)
	assert.True(s.T(), s.kh.IsPodRunning("rwo-block-ro-one", defaultNamespace), "make sure rwo-block-ro-one pod is in running state")
	assert.True(s.T(), s.kh.IsPodRunning("rwo-block-ro-two", defaultNamespace), "make sure rwo-block-ro-two pod is in running state")
	assert.True(s.T(), s.kh.IsPodRunning("rwx-block-ro-one", defaultNamespace), "make sure rwx-block-ro-one pod is in running state")
	assert.True(s.T(), s.kh.IsPodRunning("rwx-block-ro-two", defaultNamespace), "make sure rwx-block-ro-two pod is in running state")

	logger.Infof("Step 8: Read Data from both ReadyOnlyMany and ReadWriteOnce pods with ReadOnly Access")
	// Read data from RWO PVC via both ReadOnly pods
	assert.Nil(s.T(), s.kh.ReadFromPod("", "rwo-block-ro-one", filename1, message1))
	assert.Nil(s.T(), s.kh.ReadFromPod("", "rwo-block-ro-two", filename1, message1))

	// Read data from RWX PVC via both ReadOnly pods
	assert.Nil(s.T(), s.kh.ReadFromPod("", "rwx-block-ro-one", filename1, message1))
	assert.Nil(s.T(), s.kh.ReadFromPod("", "rwx-block-ro-two", filename1, message1))

	logger.Infof("Step 9: Write Data to Pod with ReadOnlyMany and ReadWriteOnce PVC  mounted with ReadOnly access")
	// Write data to RWO PVC via pod with ReadOnly Set to true
	message3 := "Persisted message three"
	filename3 := "bsFile3"
	err = s.kh.WriteToPod("", "rwo-block-ro-one", filename3, message3)
	assert.Contains(s.T(), err.Error(), "failed to write file")

	// Write data to RWx PVC via pod with ReadOnly Set to true
	err = s.kh.WriteToPod("", "rwx-block-ro-one", filename3, message3)
	assert.Contains(s.T(), err.Error(), "failed to write file")

	logger.Infof("Step 10: UnMount Pod with ReadOnlyMany and ReadWriteOnce PVCs")
	// UnMount RWO PVC from both ReadOnly Pods
	_, err = s.kh.DeletePods("rwo-block-ro-one", "rwo-block-ro-two", "rwx-block-ro-one", "rwx-block-ro-two")
	assert.Nil(s.T(), err)
	assert.True(s.T(), s.kh.IsPodTerminated("rwo-block-ro-one", defaultNamespace), "make sure rwo-block-ro-one pod is terminated")
	assert.True(s.T(), s.kh.IsPodTerminated("rwo-block-ro-two", defaultNamespace), "make sure rwo-block-ro-two pod is terminated")
	assert.True(s.T(), s.kh.IsPodTerminated("rwx-block-ro-one", defaultNamespace), "make sure rwx-lock-ro-one pod is terminated")
	assert.True(s.T(), s.kh.IsPodTerminated("rwx-block-ro-two", defaultNamespace), "make sure rwx-block-ro-two pod is terminated")
}
