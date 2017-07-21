package longhaul

import (
	"bytes"
	"html/template"
	"sync"
	"testing"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/enums"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/objects"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// Rook Block Storage integration test
// Start MySql database that is using rook provisoned block storage.
// Make sure database is functional

func TestK8sBlockLongHaul(t *testing.T) {
	suite.Run(t, new(K8sBlockLongHaulSuite))
}

type K8sBlockLongHaulSuite struct {
	suite.Suite
	testClient       *clients.TestClient
	bc               contracts.BlockOperator
	kh               *utils.K8sHelper
	initBlockCount   int
	storageClassPath string
	mysqlAppPath     string
	db               *utils.MySQLHelper
	wg               sync.WaitGroup
	installer        *installer.InstallHelper
}

//Test set up - does the following in order
//create pool and storage class, create a PVC, Create a MySQL app/service that uses pvc
func (s *K8sBlockLongHaulSuite) SetupSuite() {

	var err error
	s.kh, err = utils.CreatK8sHelper()
	assert.Nil(s.T(), err)

	s.installer = installer.NewK8sRookhelper(s.kh.Clientset)

	err = s.installer.InstallRookOnK8s()
	require.NoError(s.T(), err)

	s.testClient, err = clients.CreateTestClient(enums.Kubernetes, s.kh)
	require.Nil(s.T(), err)

	s.bc = s.testClient.GetBlockClient()
	initialBlocks, err := s.bc.BlockList()
	require.Nil(s.T(), err)
	s.initBlockCount = len(initialBlocks)
	s.storageClassPath = "../../../../data/block/storageclass_pool.tmpl"
	s.mysqlAppPath = "../../../../data/integration/mysqlapp.yaml"

	//create storage class
	if scp, _ := s.kh.IsStorageClassPresent("rook-block"); !scp {
		_, _, scs := s.storageClassOperation("mysql-pool", "create")
		require.Equal(s.T(), 0, scs)

		//make sure storageclass is created
		present, err := s.kh.IsStorageClassPresent("rook-block")
		require.Nil(s.T(), err)
		require.True(s.T(), present, "Make sure storageclass is present")
	}
	//create mysql pod
	if _, err := s.kh.GetPVCStatus("mysql-pv-claim"); err != nil {

		s.kh.ResourceOperation("create", s.mysqlAppPath)

		//wait till mysql pod is up
		require.True(s.T(), s.kh.IsPodInExpectedState("mysqldb", "", "Running"))
		pvcStatus, err := s.kh.GetPVCStatus("mysql-pv-claim")
		require.Nil(s.T(), err)
		require.Contains(s.T(), pvcStatus, "Bound")
	}
	dbIP, err := s.kh.GetPodHostID("mysqldb", "")
	require.Nil(s.T(), err)
	//create database connection
	s.db = utils.CreateNewMySQLHelper("mysql", "mysql", dbIP+":30003", "sample")

	require.True(s.T(), s.db.PingSuccess())

	if exist := s.db.TableExists(); !exist {
		s.db.CreateTable()
	}

}

func (s *K8sBlockLongHaulSuite) TestBlockLonghaulRun() {

	s.wg.Add(s.installer.Env.LoadConcurrentRuns)
	for i := 1; i <= s.installer.Env.LoadConcurrentRuns; i++ {
		go s.dbOperation(i)
	}
	s.wg.Wait()
}

func (s *K8sBlockLongHaulSuite) dbOperation(i int) {
	defer s.wg.Done()
	//InsertRandomData
	s.db.InsertRandomData()
	s.db.InsertRandomData()
	s.db.InsertRandomData()
	s.db.InsertRandomData()
	s.db.InsertRandomData()
	s.db.InsertRandomData()

	//delete Data
	s.db.DeleteRandomRow()

}
func (s *K8sBlockLongHaulSuite) TearDownSuite() {
	s.db.CloseConnection()
	s.testClient = nil
	s.bc = nil
	s.kh = nil
	s.db = nil
	s = nil

	s.installer.UninstallRookFromK8s()

}
func (s *K8sBlockLongHaulSuite) storageClassOperation(poolName string, action string) (string, string, int) {

	t, _ := template.ParseFiles(s.storageClassPath)

	var tpl bytes.Buffer
	config := map[string]string{
		"poolName": poolName,
	}

	t.Execute(&tpl, config)

	cmdStruct := objects.CommandArgs{Command: "kubectl", PipeToStdIn: tpl.String(), CmdArgs: []string{action, "-f", "-"}}

	cmdOut := utils.ExecuteCommand(cmdStruct)

	return cmdOut.StdOut, cmdOut.StdErr, cmdOut.ExitCode

}
