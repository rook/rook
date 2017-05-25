package integration

import (
	"bytes"
	"github.com/rook/rook/e2e/framework/clients"
	"github.com/rook/rook/e2e/framework/contracts"
	"github.com/rook/rook/e2e/framework/objects"
	"github.com/rook/rook/e2e/framework/utils"
	"github.com/rook/rook/e2e/tests"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"html/template"
	"os"
	"path/filepath"
	"testing"
)

// Rook Block Storage integration test
// Start MySql database that is using rook provisoned block storage.
// Make sure database is functional

func TestK8sBlockIntegration(t *testing.T) {
	suite.Run(t, new(K8sBlockEnd2EndIntegrationSuite))
}

type K8sBlockEnd2EndIntegrationSuite struct {
	suite.Suite
	testClient       *clients.TestClient
	bc               contracts.BlockOperator
	kh               *utils.K8sHelper
	init_blockCount  int
	storageclassPath string
	mysqlappPath     string
	db               *utils.MySqlHelper
}

//Test set up - does the following in order
//create pool and storage class, create a PVC, Create a MySQL app/service that uses pvc
func (s *K8sBlockEnd2EndIntegrationSuite) SetupSuite() {

	var err error

	s.testClient, err = clients.CreateTestClient(tests.Platform)
	require.Nil(s.T(), err)

	s.bc = s.testClient.GetBlockClient()
	s.kh = utils.CreatK8sHelper()
	initialBlocks, err := s.bc.BlockList()
	require.Nil(s.T(), err)
	s.init_blockCount = len(initialBlocks)
	s.storageclassPath = filepath.Join(os.Getenv("GOPATH"), "src/github.com/rook/rook/e2e/data/block/storageclass_pool.tmpl")
	s.mysqlappPath = filepath.Join(os.Getenv("GOPATH"), "src/github.com/rook/rook/e2e/data/integration/mysqlapp.yaml")

	//create storage class
	_, _, scs := s.storageClassOperation("mysql-pool", "create")
	require.Equal(s.T(), 0, scs)

	//make sure storageclass is created
	present, err := s.kh.IsStorageClassPresent("rook-block")
	require.Nil(s.T(), err)
	require.True(s.T(), present, "Make sure storageclass is present")

	//create mysql pod
	s.kh.ResourceOperation("create", s.mysqlappPath)

	//wait till mysql pod is up

	require.True(s.T(), s.kh.IsPodInExpectedState("mysqldb", "", "Running"))
	pvcStatus, err := s.kh.GetPVCStatus("mysql-pv-claim")
	require.Nil(s.T(), err)
	require.Contains(s.T(), pvcStatus, "Bound")

	dbIp, err := s.kh.GetPodHostId("mysqldb", "")
	require.Nil(s.T(), err)
	//create database connection
	s.db = utils.CreateNewMySqlHelper("mysql", "mysql", dbIp+":30003", "sample")

}

func (s *K8sBlockEnd2EndIntegrationSuite) TestBlockE2EIntegrationWithMySqlDatabase() {

	//ping database
	require.True(s.T(), s.db.PingSuccess())

	//Create  a table
	s.db.CreateTable()
	require.EqualValues(s.T(), 0, s.db.TableRowCount(), "make sure tables has no rows initially")

	//Write Data
	s.db.InsertRandomData()
	require.EqualValues(s.T(), 1, s.db.TableRowCount(), "make sure new row is created")

	//delete Data
	s.db.DeleteRandomRow()
	require.EqualValues(s.T(), 0, s.db.TableRowCount(), "make sure row is deleted")

}

func (s *K8sBlockEnd2EndIntegrationSuite) storageClassOperation(poolName string, action string) (string, string, int) {

	t, _ := template.ParseFiles(s.storageclassPath)

	var tpl bytes.Buffer
	config := map[string]string{
		"poolName": poolName,
	}

	t.Execute(&tpl, config)

	cmdStruct := objects.CommandArgs{Command: "kubectl", PipeToStdIn: tpl.String(), CmdArgs: []string{action, "-f", "-"}}

	cmdOut := utils.ExecuteCommand(cmdStruct)

	return cmdOut.StdOut, cmdOut.StdErr, cmdOut.ExitCode

}

func (s *K8sBlockEnd2EndIntegrationSuite) TearDownTest() {

	s.kh.ResourceOperation("delete", s.mysqlappPath)
	s.storageClassOperation("mysql-pool", "delete")

}
