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

	"github.com/rook/rook/pkg/daemon/ceph/model"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/contracts"
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
	installer      *installer.InstallHelper
	op             contracts.Setup
}

func (s *BlockCreateSuite) SetupSuite() {

	var err error
	s.namespace = "block-k8s-ns"
	s.op, s.kh = StartBaseTestOperations(s.T, s.namespace, "bluestore", false, false, 1)
	s.testClient = GetTestClient(s.kh, s.namespace, s.op, s.T)
	initialBlocks, err := s.testClient.BlockClient.List(s.namespace)
	assert.Nil(s.T(), err)
	s.initBlockCount = len(initialBlocks)
}

// Test case when persistentvolumeclaim is created for a storage class that doesn't exist
func (s *BlockCreateSuite) TestCreatePVCWhenNoStorageClassExists() {
	logger.Infof("Test creating PVC(block images) when storage class is not created")

	//Create PVC
	claimName := "test-no-storage-class-claim"
	poolName := "test-no-storage-class-pool"
	storageClassName := "rook-ceph-block"
	defer s.tearDownTest(claimName, poolName, storageClassName, "ReadWriteOnce")

	result, err := installer.BlockResourceOperation(s.kh, installer.GetBlockPvcDef(claimName, storageClassName, "ReadWriteOnce"), "create")
	assert.Contains(s.T(), result, fmt.Sprintf("persistentvolumeclaim \"%s\" created", claimName), "Make sure pvc is created. "+result)
	require.NoError(s.T(), err)

	//check status of PVC
	pvcStatus, err := s.kh.GetPVCStatus(defaultNamespace, claimName)
	require.Nil(s.T(), err)
	assert.Contains(s.T(), pvcStatus, "Pending", "Makes sure PVC is in Pending state")

	//check block image count
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
	defer s.tearDownTest(claimName, poolName, storageClassName, "ReadWriteOnce")
	status, _ := s.kh.GetPVCStatus(defaultNamespace, claimName)
	logger.Infof("PVC %s status: %s", claimName, status)
	s.testClient.BlockClient.List(s.namespace)

	logger.Infof("create pool and storageclass")
	pool := model.Pool{Name: poolName, ReplicatedConfig: model.ReplicatedPoolConfig{Size: 1}}
	result0, err0 := s.testClient.PoolClient.Create(pool, s.namespace)
	assert.Contains(s.T(), result0, fmt.Sprintf("\"%s\" created", poolName), "Make sure test pool is created")
	require.NoError(s.T(), err0)

	result1, err1 := installer.BlockResourceOperation(s.kh, installer.GetBlockStorageClassDef(poolName, storageClassName, s.namespace, true), "create")
	assert.Contains(s.T(), result1, fmt.Sprintf("\"%s\" created", storageClassName), "Make sure storageclass is created")
	require.NoError(s.T(), err1)

	logger.Infof("make sure storageclass is created")
	err := s.kh.IsStorageClassPresent("rook-ceph-block")
	require.Nil(s.T(), err)

	logger.Infof("create pvc")
	result2, err2 := installer.BlockResourceOperation(s.kh, installer.GetBlockPvcDef(claimName, storageClassName, "ReadWriteOnce"), "create")
	assert.Contains(s.T(), result2, fmt.Sprintf("\"%s\" created", claimName), "Make sure pvc is created. "+result2)
	require.NoError(s.T(), err2)

	logger.Infof("check status of PVC")
	require.True(s.T(), s.kh.WaitUntilPVCIsBound(defaultNamespace, claimName))

	b1, err := s.testClient.BlockClient.List(s.namespace)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), s.initBlockCount+1, len(b1), "Make sure new block image is created")

	logger.Infof("Create same pvc again")
	result3, err3 := installer.BlockResourceOperation(s.kh, installer.GetBlockPvcDef(claimName, storageClassName, "ReadWriteOnce"), "create")
	assert.Contains(s.T(), err3.Error(), fmt.Sprintf("\"%s\" already exists", claimName), "make sure PVC is not created again. "+result3)

	logger.Infof("check status of PVC")
	require.True(s.T(), s.kh.WaitUntilPVCIsBound(defaultNamespace, claimName))

	logger.Infof("check block image count")
	b2, _ := s.testClient.BlockClient.List(s.namespace)
	assert.Equal(s.T(), len(b1), len(b2), "Make sure new block image is created")

}

func (s *BlockCreateSuite) TestBlockStorageMountUnMountForStatefulSets() {
	poolName := "stspool"
	storageClassName := "stssc"
	statefulSetName := "block-stateful-set"
	statefulPodsName := "ststest"

	defer s.statefulSetDataCleanup(defaultNamespace, poolName, storageClassName, statefulSetName, statefulPodsName)
	logger.Infof("Test case when block persistent volumes are scaled up and down along with StatefulSet")
	logger.Info("Step 1: Create pool and storageClass")
	_, cbErr := installer.BlockResourceOperation(s.kh, installer.GetBlockPoolStorageClass(s.namespace, poolName, storageClassName), "create")
	assert.Nil(s.T(), cbErr)
	logger.Info("Step 2 : Deploy statefulSet with 1X replication")
	service, statefulset := getBlockStatefulSetAndServiceDefinition(defaultNamespace, statefulSetName, statefulPodsName, storageClassName)
	_, err1 := s.kh.Clientset.CoreV1().Services(defaultNamespace).Create(service)
	_, err2 := s.kh.Clientset.AppsV1beta1().StatefulSets(defaultNamespace).Create(statefulset)
	assert.Nil(s.T(), err1)
	assert.Nil(s.T(), err2)
	assert.True(s.T(), s.kh.CheckPodCountAndState(statefulSetName, defaultNamespace, 1, "Running"))
	assert.True(s.T(), s.kh.CheckPvcCountAndStatus(statefulSetName, defaultNamespace, 1, "Bound"))

	logger.Info("Step 3 : Scale up replication on statefulSet")
	scaleerr := s.kh.ScaleStatefulSet(statefulPodsName, defaultNamespace, 2)
	assert.NoError(s.T(), scaleerr, "make sure scale up is successful")
	assert.True(s.T(), s.kh.CheckPodCountAndState(statefulSetName, defaultNamespace, 2, "Running"))
	assert.True(s.T(), s.kh.CheckPvcCountAndStatus(statefulSetName, defaultNamespace, 2, "Bound"))

	logger.Info("Step 4 : Scale down replication on statefulSet")
	scaleerr = s.kh.ScaleStatefulSet(statefulPodsName, defaultNamespace, 1)
	assert.NoError(s.T(), scaleerr, "make sure scale down is successful")
	assert.True(s.T(), s.kh.CheckPodCountAndState(statefulSetName, defaultNamespace, 1, "Running"))
	assert.True(s.T(), s.kh.CheckPvcCountAndStatus(statefulSetName, defaultNamespace, 2, "Bound"))

	logger.Info("Step 5 : Delete statefulSet")
	delOpts := metav1.DeleteOptions{}
	listOpts := metav1.ListOptions{LabelSelector: "app=" + statefulSetName}
	err1 = s.kh.Clientset.CoreV1().Services(defaultNamespace).Delete(statefulSetName, &delOpts)
	err2 = s.kh.Clientset.AppsV1beta1().StatefulSets(defaultNamespace).Delete(statefulPodsName, &delOpts)
	err3 := s.kh.Clientset.CoreV1().Pods(defaultNamespace).DeleteCollection(&delOpts, listOpts)
	assert.Nil(s.T(), err1)
	assert.Nil(s.T(), err2)
	assert.Nil(s.T(), err3)
	assert.True(s.T(), s.kh.WaitUntilPodWithLabelDeleted(fmt.Sprintf("app=%s", statefulSetName), defaultNamespace))
	assert.True(s.T(), s.kh.CheckPvcCountAndStatus(statefulSetName, defaultNamespace, 2, "Bound"))
}

func (s *BlockCreateSuite) statefulSetDataCleanup(namespace, poolName, storageClassName, statefulSetName, statefulPodsName string) {
	delOpts := metav1.DeleteOptions{}
	listOpts := metav1.ListOptions{LabelSelector: "app=" + statefulSetName}
	//Delete stateful set
	s.kh.Clientset.CoreV1().Services(namespace).Delete(statefulSetName, &delOpts)
	s.kh.Clientset.AppsV1beta1().StatefulSets(defaultNamespace).Delete(statefulPodsName, &delOpts)
	s.kh.Clientset.CoreV1().Pods(defaultNamespace).DeleteCollection(&delOpts, listOpts)
	//Delete all PVCs
	s.kh.DeletePvcWithLabel(defaultNamespace, statefulSetName)
	//Delete storageclass and pool
	installer.BlockResourceOperation(s.kh, installer.GetBlockPoolStorageClass(s.namespace, poolName, storageClassName), "delete")

}

func (s *BlockCreateSuite) tearDownTest(claimName string, poolName string, storageClassName string, accessMode string) {
	installer.BlockResourceOperation(s.kh, installer.GetBlockPvcDef(claimName, storageClassName, accessMode), "delete")
	installer.BlockResourceOperation(s.kh, installer.GetBlockPoolDef(poolName, s.namespace, "1"), "delete")
	installer.BlockResourceOperation(s.kh, installer.GetBlockStorageClassDef(poolName, storageClassName, s.namespace, true), "delete")

}

func (s *BlockCreateSuite) TearDownSuite() {
	s.op.TearDown()
}

func (s *BlockCreateSuite) CheckCreatingPVC(pvcName, pvcAccessMode string) {
	logger.Infof("Test creating %s PVC(block images) when storage class is created", pvcAccessMode)
	claimName := fmt.Sprintf("test-with-storage-class-claim-%s", pvcName)
	poolName := fmt.Sprintf("test-with-storage-class-pool-%s", pvcName)
	storageClassName := "rook-ceph-block"
	defer s.tearDownTest(claimName, poolName, storageClassName, pvcAccessMode)

	//create pool and storageclass
	result0, err0 := installer.BlockResourceOperation(s.kh, installer.GetBlockPoolDef(poolName, s.namespace, "1"), "create")
	assert.Contains(s.T(), result0, fmt.Sprintf("\"%s\" created", poolName), "Make sure test pool is created")
	require.NoError(s.T(), err0)
	result1, err1 := installer.BlockResourceOperation(s.kh, installer.GetBlockStorageClassDef(poolName, storageClassName, s.namespace, true), "create")
	assert.Contains(s.T(), result1, fmt.Sprintf("\"%s\" created", storageClassName), "Make sure storageclass is created")
	require.NoError(s.T(), err1)

	//make sure storageclass is created
	err := s.kh.IsStorageClassPresent(storageClassName)
	require.Nil(s.T(), err)

	//create pvc
	result2, err2 := installer.BlockResourceOperation(s.kh, installer.GetBlockPvcDef(claimName, storageClassName, pvcAccessMode), "create")
	assert.Contains(s.T(), result2, fmt.Sprintf("\"%s\" created", claimName), "Make sure pvc is created. "+result2)
	require.NoError(s.T(), err2)

	//check status of PVC
	require.True(s.T(), s.kh.WaitUntilPVCIsBound(defaultNamespace, claimName))
	accessModes, err := s.kh.GetPVCAccessModes(defaultNamespace, claimName)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), accessModes[0], v1.PersistentVolumeAccessMode(pvcAccessMode))

	//check block image count
	b, _ := s.testClient.BlockClient.List(s.namespace)
	require.Equal(s.T(), s.initBlockCount+1, len(b), "Make sure new block image is created")

}
