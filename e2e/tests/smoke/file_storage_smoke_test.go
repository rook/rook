package smoke

import (
	"github.com/rook/rook/e2e/framework/enums"
	"github.com/rook/rook/e2e/framework/manager"
	"github.com/rook/rook/e2e/framework/objects"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"testing"
)

var env objects.EnvironmentManifest

func init() {
	env = objects.NewManifest()
}

type FileSystemTestSuite struct {
	suite.Suite
	rookPlatform enums.RookPlatformType
	k8sVersion   enums.K8sVersion
	rookTag      string
}

func TestFileSystemSmokeSuite(t *testing.T) {

	suite.Run(t, new(FileSystemTestSuite))
}

func (suite *FileSystemTestSuite) SetupTest() {
	var err error

	suite.rookPlatform, err = enums.GetRookPlatFormTypeFromString(env.Platform)

	require.Nil(suite.T(), err)

	suite.k8sVersion, err = enums.GetK8sVersionFromString(env.K8sVersion)

	require.Nil(suite.T(), err)

	suite.rookTag = env.RookTag

	require.NotEmpty(suite.T(), suite.rookTag, "RookTag parameter is required")

}

func (suite *FileSystemTestSuite) TestFileStorage_SmokeTest() {
	suite.T().Skip("Skipping test : existing issue https://github.com/rook/rook/issues/612")
	var err error

	err, rookInfra := rook_test_infra.GetRookTestInfraManager(suite.rookPlatform, true, suite.k8sVersion)

	require.Nil(suite.T(), err)

	rookInfra.ValidateAndSetupTestPlatform()

	err, _ = rookInfra.InstallRook(suite.rookTag)

	require.Nil(suite.T(), err)

	suite.T().Log("File Storage Smoke Test - Create,Mount,write to, read from  and Unmount Filesystem")
	sc, _ := CreateSmokeTestClient(rookInfra.GetRookPlatform())
	defer fileSmokecleanUp()
	rh := sc.rookHelp
	rfc := sc.GetFileSystemClient()

	suite.T().Log("Step 1: Create file System")
	_, fsc_err := sc.CreateFileStorage()
	require.Nil(suite.T(), fsc_err)
	rawlist, _ := rfc.FSList()
	filesystemData := rh.ParseFileSystemData(rawlist)
	require.Equal(suite.T(), "testfs", filesystemData.Name, "make sure filesystem name matches")
	suite.T().Log("File system created")

	suite.T().Log("Step 2: Mount file System")
	_, mtfs_err := sc.MountFileStorage()
	require.Nil(suite.T(), mtfs_err)
	suite.T().Log("File system mounted successfully")

	suite.T().Log("Step 3: Write to file system")
	_, wfs_err := sc.WriteToFileStorage("Test data for file", "fsFile1")
	require.Nil(suite.T(), wfs_err)
	suite.T().Log("Write to file system successful")

	suite.T().Log("Step 4: Read from file system")
	read, rd_err := sc.ReadFromFileStorage("fsFile1")
	require.Nil(suite.T(), rd_err)
	require.Contains(suite.T(), read, "Test data for file", "make sure content of the files is unchanged")
	suite.T().Log("Read from file system successful")

	suite.T().Log("Step 5: Mount file System")
	_, umtfs_err := sc.UnmountFileStorage()
	require.Nil(suite.T(), umtfs_err)
	suite.T().Log("File system mounted successfully")

	suite.T().Log("Step 6: Deleting file storage")
	_, fsd_err := sc.DeleteFileStorage()
	require.Nil(suite.T(), fsd_err)
	//Delete is not actually deleting filesystem
	suite.T().Log("File system deleted")
}

func fileSmokecleanUp() {
	sc, _ := CreateSmokeTestClient(enums.Kubernetes)
	sc.UnmountFileStorage()
	sc.DeleteFileStorage()
}
