package longhaul

import (
	"strconv"
	"sync"
	"testing"

	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/suite"
	"time"
)

// Rook Block Storage longhaul test
// Start MySql database that is using rook provisioned block storage.
// Make sure database is functional over multiple runs.
//NOTE: This tests doesn't clean up the cluster or volume after the run, the tests is designed
//to reuse the same cluster and volume for multiple runs or over a period of time.
// It is recommended to run this test with -count test param (to repeat th test n number of times)
// along with --load_parallel_runs params(number of concurrent operations per test)
func TestBlockLongHaul(t *testing.T) {
	suite.Run(t, new(BlockLongHaulSuite))
}

type BlockLongHaulSuite struct {
	suite.Suite
	kh        *utils.K8sHelper
	installer *installer.InstallHelper
}

//Test set up - does the following in order
//create pool and storage class, create a PVC, Create a MySQL app/service that uses pvc

func (s *BlockLongHaulSuite) SetupSuite() {

	s.kh, s.installer = setUpRookAndPoolInNamespace(s.T, defaultLongHaulNamespace, "rook-block", "rook-pool")
}

func (s *BlockLongHaulSuite) TestBlockLonghaulRun() {
	var wg sync.WaitGroup
	wg.Add(s.installer.Env.LoadVolumeNumber)
	for i := 1; i <= s.installer.Env.LoadVolumeNumber; i++ {
		if i == 1 {
			go BlockVolumeOperations(s, &wg, "rook-block", "mysqlapp-persist", "mysqldb", "mysql-persist-claim", false)
		} else {
			go BlockVolumeOperations(s, &wg, "rook-block", "mysqlapp-ephemeral-"+strconv.Itoa(i), "mysqldbeph"+strconv.Itoa(i), "mysql-ephemeral-claim"+strconv.Itoa(i), true)

		}

	}
	wg.Wait()
}

func BlockVolumeOperations(s *BlockLongHaulSuite, wg *sync.WaitGroup, storageClassName string, appName string, appLabel string, pvcName string, cleanup bool) {
	defer wg.Done()
	db := createPVCAndMountMysqlPod(s.T, s.kh, storageClassName, appName, appLabel, pvcName)
	performBlockOperations(s.installer, db)
	if cleanup {
		mySqlPodOperation(s.kh, storageClassName, appName, appLabel, pvcName, "delete")
		s.kh.IsPodWithLabelDeleted(appLabel, "default")
	}
	db.CloseConnection()
	db = nil
	time.Sleep(5 * time.Second)
}

func (s *BlockLongHaulSuite) TearDownSuite() {
	s.kh = nil
	s = nil
}
