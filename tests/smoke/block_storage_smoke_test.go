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

package smoke

import (
	"testing"
	"time"

	"fmt"

	"github.com/rook/rook/tests/framework/enums"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestBlockStorageSmokeSuite(t *testing.T) {
	suite.Run(t, new(BlockStorageTestSuite))
}

type BlockStorageTestSuite struct {
	suite.Suite
	helper *SmokeTestHelper
	rook   *RookHelper
}

func (suite *BlockStorageTestSuite) SetupTest() {
	var err error

	suite.rook, err = NewRookHelper()
	err = suite.rook.InstallRook()
	if err != nil {
		panic(fmt.Errorf("failed to install rook: %v", err))
	}

	suite.helper, err = CreateSmokeTestClient(enums.Kubernetes)
	require.Nil(suite.T(), err)
}

// Smoke Test for Block Storage - Test check the following operations on Block Storage in order
//Create,Mount,Write,Read,Unmount and Delete.
func (suite *BlockStorageTestSuite) TestBlockStorage_SmokeTest() {

	suite.T().Log("Block Storage Smoke Test - Create,Mount,write to, read from  and Unmount Block")
	rbc := suite.helper.GetBlockClient()

	suite.T().Log("Step 0 : Get Initial List Block")
	initBlockImages, _ := rbc.BlockList()

	suite.T().Log("step 1: Create block storage")
	_, cb_err := suite.helper.CreateBlockStorage()
	require.Nil(suite.T(), cb_err)
	require.True(suite.T(), retryBlockImageCountCheck(suite.helper, len(initBlockImages), 1), "Make sure a new block is created")
	suite.T().Log("Block Storage created successfully")

	suite.T().Log("step 2: Mount block storage")
	_, mt_err := suite.helper.MountBlockStorage()
	require.Nil(suite.T(), mt_err)
	suite.T().Log("Block Storage Mounted successfully")

	suite.T().Log("step 3: Write to block storage")
	_, wt_err := suite.helper.WriteToBlockStorage("Test Data", "testFile1")
	require.Nil(suite.T(), wt_err)
	suite.T().Log("Write to Block storage successfully")

	suite.T().Log("step 4: Read from  block storage")
	read, r_err := suite.helper.ReadFromBlockStorage("testFile1")
	require.Nil(suite.T(), r_err)
	require.Contains(suite.T(), read, "Test Data", "make sure content of the files is unchanged")
	suite.T().Log("Read from  Block storage successfully")

	suite.T().Log("step 5: Unmount block storage")
	_, unmt_err := suite.helper.UnMountBlockStorage()
	require.Nil(suite.T(), unmt_err)
	suite.T().Log("Block Storage unmounted successfully")

	suite.T().Log("step 6: Deleting block storage")
	_, db_err := suite.helper.DeleteBlockStorage()
	require.Nil(suite.T(), db_err)
	require.True(suite.T(), retryBlockImageCountCheck(suite.helper, len(initBlockImages), 0), "Make sure a new block is created")
	suite.T().Log("Block Storage deleted successfully")

}

func (s *BlockStorageTestSuite) TearDownTest() {
	s.helper.UnMountBlockStorage()
	s.helper.DeleteBlockStorage()
	s.helper.CleanUpDymanicBlockStorage()
}

// periodically checking if block image count has changed to expected value
// When creating pvc in k8s platform, it may take some time for the block Image to be bounded
func retryBlockImageCountCheck(h *SmokeTestHelper, imageCount int, expectedChange int) bool {
	inc := 0
	for inc < 30 {
		blockImages, _ := h.GetBlockClient().BlockList()
		if imageCount+expectedChange == len(blockImages) {
			return true
		} else {
			time.Sleep(time.Second * 2)
			inc++
		}
	}
	return false
}
