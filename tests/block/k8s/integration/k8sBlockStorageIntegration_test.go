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
	"bytes"
	"html/template"
	"os"
	"path/filepath"
	"testing"

	"github.com/rook/rook/tests"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/objects"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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
	initBlockCount   int
	storageclassPath string
	mysqlappPath     string
	db               *utils.MySQLHelper
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
	s.initBlockCount = len(initialBlocks)
	s.storageclassPath = filepath.Join(os.Getenv("GOPATH"), "src/github.com/rook/rook/tests/data/block/storageclass_pool.tmpl")
	s.mysqlappPath = filepath.Join(os.Getenv("GOPATH"), "src/github.com/rook/rook/tests/data/integration/mysqlapp.yaml")

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

	dbIP, err := s.kh.GetPodHostID("mysqldb", "")
	require.Nil(s.T(), err)
	//create database connection
	s.db = utils.CreateNewMySQLHelper("mysql", "mysql", dbIP+":30003", "sample")

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
	tests.CleanUp()

}

func (s *K8sBlockEnd2EndIntegrationSuite) TearDownSuite() {

	tests.CleanUp()

}
