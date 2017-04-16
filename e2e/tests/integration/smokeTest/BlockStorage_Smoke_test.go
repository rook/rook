package smokeTest

import (
	"errors"
	"github.com/dangula/rook/e2e/objects"
	"github.com/dangula/rook/e2e/rook-test-framework/enums"
	"github.com/dangula/rook/e2e/rook-test-framework/managers"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestBlockStorage_SmokeTest(t *testing.T) {
	env := objects.NewManifest()

	rookPlatform, errPlatform := enums.GetRookPlatFormTypeFromString(env.Platform)

	if errPlatform != nil {
		assert.Nil(t, errPlatform)
	}

	k8sVersion, errVersion := enums.GetK8sVersionFromString(env.K8sVersion)

	if errPlatform != nil {
		assert.Nil(t, errVersion)
	}

	if env.RookTag == "" {
		assert.Nil(t, errors.New("RookTag parameter is required"))
	}

	errInfra, rookInfra := managers.GetRookTestInfraManager(rookPlatform, true, k8sVersion)

	if errInfra != nil {
		assert.Nil(t, errInfra)
	}

	rookInfra.ValidateAndSetupTestPlatform()

	errInstall, _ := rookInfra.InstallRook(env.RookTag)

	if errInstall != nil {
		assert.Nil(t, errInstall)
	}

	t.Log("Block Storage Smoke Test - Create,Mount,write to, read from  and Unmount Block")
	sc, _ := CreateSmokeTestClient(enums.Kubernetes)
	//defer blockTestcleanup()
	rh := sc.rookHelp
	rbc := sc.GetBlockClient()
	t.Log("Step 0 : Get Initial List Block")
	rawlistInit, _ := rbc.Block_List()
	initblocklistMap := rh.ParseBlockListData(rawlistInit)

	t.Log("step 1: Create block storage")
	_, cb_err := sc.CreateBlockStorage()
	assert.Nil(t, cb_err)
	rawlistAfterCreate, _ := rbc.Block_List()
	blocklistMapAfterBlockCreate := rh.ParseBlockListData(rawlistAfterCreate)
	assert.Empty(t, len(initblocklistMap), len(blocklistMapAfterBlockCreate)+1, "Make sure a new block is created")
	t.Log("Block Storage created successfully")

	t.Log("step 2: Mount block storage")
	_, mt_err := sc.MountBlockStorage()
	assert.Nil(t, mt_err)
	t.Log("Block Storage Mounted successfully")

	t.Log("step 3: Write to block storage")
	_, wt_err := sc.WriteToBlockStorage("Test Data", "testFile1")
	assert.Nil(t, wt_err)
	t.Log("Write to Block storage successfully")

	t.Log("step 4: Read from  block storage")
	read, r_err := sc.ReadFromBlockStorage("testFile1")
	assert.Nil(t, r_err)
	assert.Contains(t, read, "Test Data", "make sure content of the files is unchanged")
	t.Log("Read from  Block storage successfully")

	t.Log("step 5: Unmount block storage")
	_, unmt_err := sc.UnMountBlockStorage()
	assert.Nil(t, unmt_err)
	t.Log("Block Storage unmounted successfully")

	t.Log("step 6: Create block storage")
	_, db_err := sc.DeleteBlockStorage()
	assert.Nil(t, db_err)
	rawlistAfterDelete, _ := rbc.Block_List()
	blocklistMapAfterBlockDelete := rh.ParseBlockListData(rawlistAfterDelete)
	//This is a stop gap, block storage is not deleted when pods are deleted
	sc.CleanUpDymanicBlockStorge()
	assert.Empty(t, len(initblocklistMap), len(blocklistMapAfterBlockDelete), "Make sure a new block is created")
	t.Log("Block Storage deleted successfully")

}

func blockTestcleanup() {
	sc, _ := CreateSmokeTestClient(enums.Kubernetes)
	sc.UnMountBlockStorage()
	sc.DeleteBlockStorage()
	sc.CleanUpDymanicBlockStorge()
}
