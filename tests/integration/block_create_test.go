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
	"testing"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test K8s Block Image Creation Scenarios. These tests work when platform is set to Kubernetes

func TestBlockCreateSuite(t *testing.T) {
	s := new(BlockCreateSuite)
	defer func(s *BlockCreateSuite) {
		HandlePanics(recover(), s.op, s.T)
	}(s)
	suite.Run(t, s)
}

type BlockCreateSuite struct {
	suite.Suite
	testClient     *clients.TestClient
	kh             *utils.K8sHelper
	initBlockCount int
	namespace      string
	installer      *installer.CephInstaller
	op             *TestCluster
}

func (s *BlockCreateSuite) SetupSuite() {

	var err error
	s.namespace = "block-k8s-ns"
	mons := 1
	rbdMirrorWorkers := 1
	s.op, s.kh = StartTestCluster(s.T, s.namespace, "bluestore", false, false, mons, rbdMirrorWorkers, installer.VersionMaster, installer.MimicVersion)
	s.testClient = clients.CreateTestClient(s.kh, s.op.installer.Manifests)
	initialBlocks, err := s.testClient.BlockClient.List(s.namespace)
	assert.Nil(s.T(), err)
	s.initBlockCount = len(initialBlocks)
}

// Test case when persistentvolumeclaim is created for a storage class that doesn't exist
func (s *BlockCreateSuite) TestCreatePVCWhenNoStorageClassExists() {
	logger.Infof("Test creating PVC(block images) when storage class is not created")

	// Create PVC
	claimName := "test-no-storage-class-claim"
	poolName := "test-no-storage-class-pool"
	storageClassName := "rook-ceph-block"
	reclaimPolicy := "Delete"
	defer s.tearDownTest(claimName, poolName, storageClassName, reclaimPolicy, "ReadWriteOnce")

	err := s.testClient.BlockClient.CreatePvc(claimName, storageClassName, "ReadWriteOnce")
	require.NoError(s.T(), err)

	// check status of PVC
	pvcStatus, err := s.kh.GetPVCStatus(defaultNamespace, claimName)
	require.Nil(s.T(), err)
	assert.Contains(s.T(), pvcStatus, "Pending", "Makes sure PVC is in Pending state")

	// check block image count
	b, _ := s.testClient.BlockClient.List(s.namespace)
	require.Equal(s.T(), s.initBlockCount, len(b), "Make sure new block image is not created")
}

func (s *BlockCreateSuite) TestCreatingPVCWithVariousAccessModes() {
	s.CheckCreatingPVC("rwo", "ReadWriteOnce")
	s.CheckCreatingPVC("rwx", "ReadWriteMany")
	s.CheckCreatingPVC("rox", "ReadOnlyMany")
}

// Test case when persistentvolumeclaim is created for a valid storage class twice
func (s *BlockCreateSuite) TestCreateSamePVCTwice() {
	logger.Infof("Test creating PVC(create block images) twice")
	claimName := "test-twice-claim"
	poolName := "test-twice-pool"
	storageClassName := "rook-ceph-block"
	reclaimPolicy := "Delete"
	defer s.tearDownTest(claimName, poolName, storageClassName, reclaimPolicy, "ReadWriteOnce")
	status, _ := s.kh.GetPVCStatus(defaultNamespace, claimName)
	logger.Infof("PVC %s status: %s", claimName, status)
	s.testClient.BlockClient.List(s.namespace)

	logger.Infof("create pool and storageclass")
	err := s.testClient.PoolClient.Create(poolName, s.namespace, 1)
	require.NoError(s.T(), err)

	err = s.testClient.BlockClient.CreateStorageClass(poolName, storageClassName, reclaimPolicy, s.namespace, true)
	require.NoError(s.T(), err)

	logger.Infof("make sure storageclass is created")
	err = s.kh.IsStorageClassPresent("rook-ceph-block")
	require.Nil(s.T(), err)

	logger.Infof("create pvc")
	err = s.testClient.BlockClient.CreatePvc(claimName, storageClassName, "ReadWriteOnce")
	require.NoError(s.T(), err)

	logger.Infof("check status of PVC")
	require.True(s.T(), s.kh.WaitUntilPVCIsBound(defaultNamespace, claimName))

	b1, err := s.testClient.BlockClient.List(s.namespace)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), s.initBlockCount+1, len(b1), "Make sure new block image is created")

	logger.Infof("Create same pvc again and expect an error")
	err = s.testClient.BlockClient.CreatePvc(claimName, storageClassName, "ReadWriteOnce")
	assert.NotNil(s.T(), err)

	logger.Infof("check status of PVC")
	require.True(s.T(), s.kh.WaitUntilPVCIsBound(defaultNamespace, claimName))

	logger.Infof("check block image count")
	b2, _ := s.testClient.BlockClient.List(s.namespace)
	assert.Equal(s.T(), len(b1), len(b2), "Make sure new block image is created")

}

func (s *BlockCreateSuite) TestBlockStorageMountUnMountForStatefulSets() {
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

func (s *BlockCreateSuite) statefulSetDataCleanup(namespace, poolName, storageClassName, reclaimPolicy, statefulSetName, statefulPodsName string) {
	delOpts := metav1.DeleteOptions{}
	listOpts := metav1.ListOptions{LabelSelector: "app=" + statefulSetName}
	// Delete stateful set
	s.kh.Clientset.CoreV1().Services(namespace).Delete(statefulSetName, &delOpts)
	s.kh.Clientset.AppsV1().StatefulSets(defaultNamespace).Delete(statefulPodsName, &delOpts)
	s.kh.Clientset.CoreV1().Pods(defaultNamespace).DeleteCollection(&delOpts, listOpts)
	// Delete all PVCs
	s.kh.DeletePvcWithLabel(defaultNamespace, statefulSetName)
	// Delete storageclass and pool
	s.testClient.PoolClient.DeleteStorageClass(s.namespace, poolName, storageClassName, reclaimPolicy)
}

func (s *BlockCreateSuite) tearDownTest(claimName string, poolName string, storageClassName string, reclaimPolicy string, accessMode string) {
	s.testClient.BlockClient.DeletePvc(claimName, storageClassName, accessMode)
	s.testClient.PoolClient.Delete(poolName, s.namespace)
	s.testClient.BlockClient.DeleteStorageClass(poolName, storageClassName, reclaimPolicy, s.namespace)
}

func (s *BlockCreateSuite) TearDownSuite() {
	s.op.Teardown()
}

func (s *BlockCreateSuite) CheckCreatingPVC(pvcName, pvcAccessMode string) {
	logger.Infof("Test creating %s PVC(block images) when storage class is created", pvcAccessMode)
	claimName := fmt.Sprintf("test-with-storage-class-claim-%s", pvcName)
	poolName := fmt.Sprintf("test-with-storage-class-pool-%s", pvcName)
	storageClassName := "rook-ceph-block"
	reclaimPolicy := "Delete"
	defer s.tearDownTest(claimName, poolName, storageClassName, reclaimPolicy, pvcAccessMode)

	// create pool and storageclass
	err := s.testClient.PoolClient.Create(poolName, s.namespace, 1)
	require.NoError(s.T(), err)
	err = s.testClient.BlockClient.CreateStorageClass(poolName, storageClassName, reclaimPolicy, s.namespace, true)
	require.NoError(s.T(), err)

	// make sure storageclass is created
	err = s.kh.IsStorageClassPresent(storageClassName)
	require.Nil(s.T(), err)

	// create pvc
	err = s.testClient.BlockClient.CreatePvc(claimName, storageClassName, pvcAccessMode)
	require.NoError(s.T(), err)

	// check status of PVC
	require.True(s.T(), s.kh.WaitUntilPVCIsBound(defaultNamespace, claimName))
	accessModes, err := s.kh.GetPVCAccessModes(defaultNamespace, claimName)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), accessModes[0], v1.PersistentVolumeAccessMode(pvcAccessMode))

	// check block image count
	b, _ := s.testClient.BlockClient.List(s.namespace)
	require.Equal(s.T(), s.initBlockCount+1, len(b), "Make sure new block image is created")

}
