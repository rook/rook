package smokeTest

import (
	"errors"
	"github.com/dangula/rook/e2e/rook-test-framework/enums"
	"github.com/dangula/rook/e2e/rook-test-framework/rook-infra-manager"
	"github.com/stretchr/testify/assert"
	"testing"
	"github.com/dangula/rook/e2e/rook-test-framework/objects"
)

var env objects.EnvironmentManifest

func init() {
	env = objects.NewManifest()
}

func TestFileStorage_SmokeTest(t *testing.T) {
	rookPlatform, errPlatform := enums.GetRookPlatFormTypeFromString(env.Platform)

	if errPlatform != nil {
		assert.Nil(t, errPlatform)
		panic(errPlatform)
	}

	k8sVersion, errVersion := enums.GetK8sVersionFromString(env.K8sVersion)

	if errVersion != nil {
		assert.Nil(t, errVersion)
		panic(errVersion)
	}

	if env.RookTag == "" {
		err := errors.New("RookTag parameter is required")
		assert.Nil(t, err)
		panic(err)
	}

	errInfra, rookInfra := rook_infra_manager.GetRookTestInfraManager(rookPlatform, true, k8sVersion)

	if errInfra != nil {
		assert.Nil(t, errInfra)
	}

	//defer rookInfra.TearDownInfrastructureCreatedEnvironment()

	rookInfra.ValidateAndSetupTestPlatform()

	errInstall, _ := rookInfra.InstallRook(env.RookTag)

	if errInstall != nil {
		assert.Nil(t, errInstall)
	}

	t.Log("File Storage Smoke Test - Create,Mount,write to, read from  and Unmount Filesystem")
	sc, _ := CreateSmokeTestClient(rookInfra.GetRookPlatform())
	defer fileSmokecleanUp()
	rh := sc.rookHelp
	rfc := sc.GetFileSystemClient()

	t.Log("Step 1: Create file System")
	_, fsc_err := sc.CreateFileStorage()
	assert.Nil(t, fsc_err)
	rawlist, _ := rfc.FS_List()
	filesystemData := rh.ParseFileSystemData(rawlist)
	assert.Equal(t, "testfs", filesystemData.Name, "make sure filesystem name matches")
	t.Log("File system created")

	t.Log("Step 2: Mount file System")
	_, mtfs_err := sc.MountFileStorage()
	assert.Nil(t, mtfs_err)
	t.Log("File system mounted successfully")

	t.Log("Step 3: Write to file system")
	_, wfs_err := sc.WriteToFileStorage("Test data for file", "fsFile1")
	assert.Nil(t, wfs_err)
	t.Log("Write to file system successful")

	t.Log("Step 4: Read from file system")
	read, rd_err := sc.ReadFromFileStorage("fsFile1")
	assert.Nil(t, rd_err)
	assert.Contains(t, read, "Test data for file", "make sure content of the files is unchanged")
	t.Log("Read from file system successful")

	t.Log("Step 5: Mount file System")
	_, umtfs_err := sc.UnmountFileStorage()
	assert.Nil(t, umtfs_err)
	t.Log("File system mounted successfully")

	t.Log("Step 6: Delete file System")
	_, fsd_err := sc.DeleteFileStorage()
	assert.Nil(t, fsd_err)
	//Delete is not actually deleting filesystem
	t.Log("File system deleted")
}

func fileSmokecleanUp() {
	sc, _ := CreateSmokeTestClient(enums.Kubernetes)
	sc.UnmountFileStorage()
	sc.DeleteFileStorage()
}
