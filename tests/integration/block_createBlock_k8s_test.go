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
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// Test K8s Block Image Creation Scenarios. These tests work when platform is set to Kubernetes

func TestK8sBlockCreate(t *testing.T) {
	suite.Run(t, new(K8sBlockImageCreateSuite))
}

type K8sBlockImageCreateSuite struct {
	suite.Suite
	testClient     *clients.TestClient
	bc             contracts.BlockOperator
	kh             *utils.K8sHelper
	initBlockCount int
	namespace      string
	installer      *installer.InstallHelper
}

func (s *K8sBlockImageCreateSuite) SetupSuite() {

	var err error
	s.namespace = "block-k8s-ns"
	s.kh, err = utils.CreateK8sHelper(s.T)
	assert.NoError(s.T(), err)

	s.installer = installer.NewK8sRookhelper(s.kh.Clientset, s.T)

	isRookInstalled, err := s.installer.InstallRookOnK8s(s.namespace, "bluestore")
	assert.NoError(s.T(), err)
	if !isRookInstalled {
		logger.Errorf("Rook Was not installed successfully")
		s.TearDownSuite()
		s.T().FailNow()
	}

	s.testClient, err = clients.CreateTestClient(s.kh, s.namespace)
	if err != nil {
		logger.Errorf("Cannot create rook test client, er -> %v", err)
		s.TearDownSuite()
		s.T().FailNow()
	}

	s.bc = s.testClient.GetBlockClient()
	initialBlocks, err := s.bc.BlockList()
	assert.Nil(s.T(), err)
	s.initBlockCount = len(initialBlocks)
}

// Test case when persistentvolumeclaim is created for a storage class that doesn't exist
func (s *K8sBlockImageCreateSuite) TestCreatePVCWhenNoStorageClassExists() {
	logger.Infof("Test creating PVC(block images) when storage class is not created")

	//Create PVC
	claimName := "test-no-storage-class-claim"
	poolName := "test-no-storage-class-pool"
	storageClassName := "rook-block"
	defer s.tearDownTest(claimName, poolName, storageClassName)

	result, err := installer.BlockResourceOperation(s.kh, installer.GetBlockPvcDef(claimName, storageClassName), "create")
	require.Contains(s.T(), result, fmt.Sprintf("persistentvolumeclaim \"%s\" created", claimName), "Make sure pvc is created. "+result)
	require.NoError(s.T(), err)

	//check status of PVC
	pvcStatus, err := s.kh.GetPVCStatus(defaultNamespace, claimName)
	require.Nil(s.T(), err)
	require.Contains(s.T(), pvcStatus, "Pending", "Makes sure PVC is in Pending state")

	//check block image count
	b, _ := s.bc.BlockList()
	require.Equal(s.T(), s.initBlockCount, len(b), "Make sure new block image is not created")

}

// Test case when persistentvolumeclaim is created for a valid storage class
func (s *K8sBlockImageCreateSuite) TestCreatePVCWhenStorageClassExists() {
	logger.Infof("Test creating PVC(block images) when storage class is created")
	claimName := "test-with-storage-class-claim"
	poolName := "test-with-storage-class-pool"
	storageClassName := "rook-block"
	defer s.tearDownTest(claimName, poolName, storageClassName)

	//create pool and storageclass
	result0, err0 := installer.BlockResourceOperation(s.kh, installer.GetBlockPoolDef(poolName, s.namespace, "1"), "create")
	require.Contains(s.T(), result0, fmt.Sprintf("pool \"%s\" created", poolName), "Make sure test pool is created")
	require.NoError(s.T(), err0)
	result1, err1 := installer.BlockResourceOperation(s.kh, installer.GetBlockStorageClassDef(poolName, storageClassName, s.namespace), "create")
	require.Contains(s.T(), result1, fmt.Sprintf("storageclass \"%s\" created", storageClassName), "Make sure storageclass is created")
	require.NoError(s.T(), err1)

	//make sure storageclass is created
	present, err := s.kh.IsStorageClassPresent(storageClassName)
	require.Nil(s.T(), err)
	require.True(s.T(), present, "Make sure storageclass is present")

	//create pvc
	result2, err2 := installer.BlockResourceOperation(s.kh, installer.GetBlockPvcDef(claimName, storageClassName), "create")
	require.Contains(s.T(), result2, fmt.Sprintf("persistentvolumeclaim \"%s\" created", claimName), "Make sure pvc is created. "+result2)
	require.NoError(s.T(), err2)

	//check status of PVC
	require.True(s.T(), s.kh.WaitUntilPVCIsBound(defaultNamespace, claimName))

	//check block image count
	b, _ := s.bc.BlockList()
	require.Equal(s.T(), s.initBlockCount+1, len(b), "Make sure new block image is created")

}

// Test case when persistentvolumeclaim is created for a valid storage class twice
func (s *K8sBlockImageCreateSuite) TestCreateSamePVCTwice() {
	logger.Infof("Test creating PVC(create block images) twice")
	claimName := "test-twice-claim"
	poolName := "test-twice-pool"
	storageClassName := "rook-block"
	defer s.tearDownTest(claimName, poolName, storageClassName)
	status, _ := s.kh.GetPVCStatus(defaultNamespace, claimName)
	logger.Infof("PVC %s status: %s", claimName, status)
	s.bc.BlockList()

	logger.Infof("create pool and storageclass")
	result0, err0 := installer.BlockResourceOperation(s.kh, installer.GetBlockPoolDef(poolName, s.namespace, "1"), "create")
	require.Contains(s.T(), result0, fmt.Sprintf("pool \"%s\" created", poolName), "Make sure test pool is created")
	require.NoError(s.T(), err0)
	result1, err1 := installer.BlockResourceOperation(s.kh, installer.GetBlockStorageClassDef(poolName, storageClassName, s.namespace), "create")
	require.Contains(s.T(), result1, fmt.Sprintf("storageclass \"%s\" created", storageClassName), "Make sure storageclass is created")
	require.NoError(s.T(), err1)

	logger.Infof("make sure storageclass is created")
	present, err := s.kh.IsStorageClassPresent("rook-block")
	require.Nil(s.T(), err)
	require.True(s.T(), present, "Make sure storageclass is present")

	logger.Infof("create pvc")
	result2, err2 := installer.BlockResourceOperation(s.kh, installer.GetBlockPvcDef(claimName, storageClassName), "create")
	require.Contains(s.T(), result2, fmt.Sprintf("persistentvolumeclaim \"%s\" created", claimName), "Make sure pvc is created. "+result2)
	require.NoError(s.T(), err2)

	logger.Infof("check status of PVC")
	require.True(s.T(), s.kh.WaitUntilPVCIsBound(defaultNamespace, claimName))

	b1, err := s.bc.BlockList()
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), s.initBlockCount+1, len(b1), "Make sure new block image is created")

	logger.Infof("Create same pvc again")
	result3, err3 := installer.BlockResourceOperation(s.kh, installer.GetBlockPvcDef(claimName, storageClassName), "create")
	require.Contains(s.T(), result3, fmt.Sprintf("persistentvolumeclaims \"%s\" already exists", claimName), "make sure PVC is not created again. "+result3)
	require.NoError(s.T(), err3)

	logger.Infof("check status of PVC")
	require.True(s.T(), s.kh.WaitUntilPVCIsBound(defaultNamespace, claimName))

	logger.Infof("check block image count")
	b2, _ := s.bc.BlockList()
	assert.Equal(s.T(), len(b1), len(b2), "Make sure new block image is created")

}

func (s *K8sBlockImageCreateSuite) tearDownTest(claimName string, poolName string, storageClassName string) {
	installer.BlockResourceOperation(s.kh, installer.GetBlockPvcDef(claimName, storageClassName), "delete")
	installer.BlockResourceOperation(s.kh, installer.GetBlockPoolDef(poolName, s.namespace, "1"), "delete")
	installer.BlockResourceOperation(s.kh, installer.GetBlockStorageClassDef(poolName, storageClassName, s.namespace), "delete")

}

func (s *K8sBlockImageCreateSuite) TearDownSuite() {
	if s.T().Failed() {
		gatherAllRookLogs(s.kh, s.Suite, s.installer.Env.HostType, s.namespace, s.namespace)
	}
	s.installer.UninstallRookFromK8s(s.namespace, false)
}
