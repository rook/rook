package smoke

import (
	"github.com/rook/rook/e2e/framework/enums"
	"github.com/rook/rook/e2e/framework/manager"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"testing"
)

func TestBlockStorageSmokeSuite(t *testing.T) {
	suite.Run(t, new(BlockStorageTestSuite))
}

type BlockStorageTestSuite struct {
	suite.Suite
	rookPlatform enums.RookPlatformType
	k8sVersion   enums.K8sVersion
	rookTag      string
}

func (suite *BlockStorageTestSuite) TearDownSuite() {
	err, rookInfra := rook_test_infra.GetRookTestInfraManager(suite.rookPlatform, true, suite.k8sVersion)

	require.Nil(suite.T(), err)

	rookInfra.TearDownInfrastructureCreatedEnvironment()
}

func (suite *BlockStorageTestSuite) SetupTest() {
	var err error

	suite.rookPlatform, err = enums.GetRookPlatFormTypeFromString(env.Platform)

	require.Nil(suite.T(), err)

	suite.k8sVersion, err = enums.GetK8sVersionFromString(env.K8sVersion)

	require.Nil(suite.T(), err)

	suite.rookTag = env.RookTag

	require.NotEmpty(suite.T(), suite.rookTag, "RookTag parameter is required")
}

func (suite *BlockStorageTestSuite) TestBlockStorage_SmokeTest() {
	var err error

	err, rookInfra := rook_test_infra.GetRookTestInfraManager(suite.rookPlatform, true, suite.k8sVersion)

	require.Nil(suite.T(), err)

	rookInfra.ValidateAndSetupTestPlatform()

	err, _ = rookInfra.InstallRook(suite.rookTag)

	require.Nil(suite.T(), err)

	suite.T().Log("Block Storage Smoke Test - Create,Mount,write to, read from  and Unmount Block")
	sc, _ := CreateSmokeTestClient(rookInfra.GetRookPlatform())
	defer blockTestcleanup()
	rh := sc.rookHelp
	rbc := sc.GetBlockClient()
	suite.T().Log("Step 0 : Get Initial List Block")
	rawlistInit, _ := rbc.BlockList()
	initblocklistMap := rh.ParseBlockListData(rawlistInit)

	suite.T().Log("step 1: Create block storage")
	_, cb_err := sc.CreateBlockStorage()
	require.Nil(suite.T(), cb_err)
	rawlistAfterCreate, _ := rbc.BlockList()
	blocklistMapAfterBlockCreate := rh.ParseBlockListData(rawlistAfterCreate)
	require.Empty(suite.T(), len(initblocklistMap), len(blocklistMapAfterBlockCreate)+1, "Make sure a new block is created")
	suite.T().Log("Block Storage created successfully")

	suite.T().Log("step 2: Mount block storage")
	_, mt_err := sc.MountBlockStorage()
	require.Nil(suite.T(), mt_err)
	suite.T().Log("Block Storage Mounted successfully")

	suite.T().Log("step 3: Write to block storage")
	_, wt_err := sc.WriteToBlockStorage("Test Data", "testFile1")
	require.Nil(suite.T(), wt_err)
	suite.T().Log("Write to Block storage successfully")

	suite.T().Log("step 4: Read from  block storage")
	read, r_err := sc.ReadFromBlockStorage("testFile1")
	require.Nil(suite.T(), r_err)
	require.Contains(suite.T(), read, "Test Data", "make sure content of the files is unchanged")
	suite.T().Log("Read from  Block storage successfully")

	suite.T().Log("step 5: Unmount block storage")
	_, unmt_err := sc.UnMountBlockStorage()
	require.Nil(suite.T(), unmt_err)
	suite.T().Log("Block Storage unmounted successfully")

	suite.T().Log("step 6: Deleting block storage")
	_, db_err := sc.DeleteBlockStorage()
	require.Nil(suite.T(), db_err)
	rawlistAfterDelete, _ := rbc.BlockList()
	blocklistMapAfterBlockDelete := rh.ParseBlockListData(rawlistAfterDelete)
	//This is a stop gap, block storage is not deleted when pods are deleted
	sc.CleanUpDymanicBlockStorge()
	require.Empty(suite.T(), len(initblocklistMap), len(blocklistMapAfterBlockDelete), "Make sure a new block is created")
	suite.T().Log("Block Storage deleted successfully")

}

func blockTestcleanup() {
	sc, _ := CreateSmokeTestClient(enums.Kubernetes)
	sc.UnMountBlockStorage()
	sc.DeleteBlockStorage()
	sc.CleanUpDymanicBlockStorge()
}
