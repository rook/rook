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

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/tests/framework/enums"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	kubeContext *installer.InstallHelper
)

func TestRookSmokeSuiteK8s(t *testing.T) {
	suite.Run(t, new(SmokeSuite))
}

type SmokeSuite struct {
	suite.Suite
	helper *TestHelper
}

func (suite *SmokeSuite) SetupSuite() {
	var err error

	kubeContext, err = installer.NewK8sRookhelper()
	require.NoError(suite.T(), err)

	err = kubeContext.InstallRookOnK8s()
	require.NoError(suite.T(), err)

	suite.helper, err = CreateSmokeTestClient(enums.Kubernetes, kubeContext.Env.K8sVersion)
	require.Nil(suite.T(), err)
}

// Smoke Test for Block Storage - Test check the following operations on Block Storage in order
//Create,Mount,Write,Read,Unmount and Delete.
var logger = capnslog.NewPackageLogger("github.com/rook/rook", "smoketest")

func (suite *SmokeSuite) TestBlockStorage_SmokeTest() {
	defer suite.blockTestDataCleanUp()
	logger.Infof("Block Storage Smoke Test - Create,Mount,write to, read from  and Unmount Block")
	rbc := suite.helper.GetBlockClient()

	logger.Infof("Step 0 : Get Initial List Block")
	initBlockImages, _ := rbc.BlockList()

	logger.Infof("step 1: Create block storage")
	_, cbErr := suite.helper.CreateBlockStorage()
	require.Nil(suite.T(), cbErr)
	require.True(suite.T(), retryBlockImageCountCheck(suite.helper, len(initBlockImages), 1), "Make sure a new block is created")
	logger.Infof("Block Storage created successfully")
	require.True(suite.T(), suite.helper.WaitUntilPVCIsBound(), "Make sure PVC is Bound")

	logger.Infof("step 2: Mount block storage")
	_, mtErr := suite.helper.MountBlockStorage()
	require.Nil(suite.T(), mtErr)
	logger.Infof("Block Storage Mounted successfully")

	logger.Infof("step 3: Write to block storage")
	_, wtErr := suite.helper.WriteToBlockStorage("Test Data", "testFile1")
	require.Nil(suite.T(), wtErr)
	logger.Infof("Write to Block storage successfully")

	logger.Infof("step 4: Read from  block storage")
	read, rErr := suite.helper.ReadFromBlockStorage("testFile1")
	require.Nil(suite.T(), rErr)
	require.Contains(suite.T(), read, "Test Data", "make sure content of the files is unchanged")
	logger.Infof("Read from  Block storage successfully")

	logger.Infof("step 5: Unmount block storage")
	_, unmtErr := suite.helper.UnMountBlockStorage()
	require.Nil(suite.T(), unmtErr)
	logger.Infof("Block Storage unmounted successfully")

	logger.Infof("step 6: Deleting block storage")
	_, dbErr := suite.helper.DeleteBlockStorage()
	require.Nil(suite.T(), dbErr)
	require.True(suite.T(), retryBlockImageCountCheck(suite.helper, len(initBlockImages), 0), "Make sure a new block is created")
	logger.Infof("Block Storage deleted successfully")

}

func (suite *SmokeSuite) blockTestDataCleanUp() {
	suite.helper.UnMountBlockStorage()
	suite.helper.DeleteBlockStorage()
	suite.helper.CleanUpDymanicBlockStorage()
}

func (suite *SmokeSuite) TearDownSuite() {

	kubeContext.UninstallRookFromK8s()

}

// periodically checking if block image count has changed to expected value
// When creating pvc in k8s platform, it may take some time for the block Image to be bounded
func retryBlockImageCountCheck(h *TestHelper, imageCount int, expectedChange int) bool {
	inc := 0
	for inc < 60 {
		blockImages, _ := h.GetBlockClient().BlockList()
		if imageCount+expectedChange == len(blockImages) {
			return true
		}
		time.Sleep(time.Second * 3)
		inc++
	}
	return false
}
