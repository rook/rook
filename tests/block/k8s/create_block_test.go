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

package k8s

import (
	"bytes"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rook/rook/tests"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/objects"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// Test K8s Block Image Creation Scenarios. These tests work when platform is set to Kubernetes
var (
	claimName = "test-claim1"
)

func TestK8sBlockCreate(t *testing.T) {
	suite.Run(t, new(K8sBlockImageCreateSuite))
}

type K8sBlockImageCreateSuite struct {
	suite.Suite
	testClient       *clients.TestClient
	bc               contracts.BlockOperator
	kh               *utils.K8sHelper
	init_blockCount  int
	pvPath           string
	storageclassPath string
}

func (s *K8sBlockImageCreateSuite) SetupSuite() {

	var err error

	s.testClient, err = clients.CreateTestClient(tests.Platform)
	require.Nil(s.T(), err)

	s.bc = s.testClient.GetBlockClient()
	s.kh = utils.CreatK8sHelper()
	initialBlocks, err := s.bc.BlockList()
	require.Nil(s.T(), err)
	s.init_blockCount = len(initialBlocks)
	s.pvPath = filepath.Join(os.Getenv("GOPATH"), "src/github.com/rook/rook/tests/data/block/pvc.tmpl")
	s.storageclassPath = filepath.Join(os.Getenv("GOPATH"), "src/github.com/rook/rook/tests/data/block/storageclass_pool.tmpl")
}

// Test case when persistentvolumeclaim is created for a storage class that doesn't exist
func (s *K8sBlockImageCreateSuite) TestCreatePVCWhenNoStorageClassExists() {

	s.T().Log("Test creating PVC(block images) when storage class is not created")
	//Create PVC
	sout1, serr1, status1 := s.pvcOperation(claimName, "create")
	require.Contains(s.T(), sout1, "persistentvolumeclaim \"test-claim1\" created", "Make sure pvc is created")
	require.Empty(s.T(), serr1, "Make sure there are no errors")
	require.Equal(s.T(), 0, status1)

	//check status of PVC
	pvcStatus, err := s.kh.GetPVCStatus(claimName)
	require.Nil(s.T(), err)
	require.Contains(s.T(), pvcStatus, "Pending", "Makes sure PVC is in Pending state")

	//check block image count
	b, _ := s.bc.BlockList()
	require.Equal(s.T(), s.init_blockCount, len(b), "Make sure new block image is not created")

}

// Test case when persistentvolumeclaim is created for a valid storage class
func (s *K8sBlockImageCreateSuite) TestCreatePVCWhenStorageClassExists() {

	s.T().Log("Test creating PVC(block images) when storage class is created")
	//create pool and storageclass
	sout, serr, status := s.storageClassOperation("test-pool-1", "create")
	require.Contains(s.T(), sout, "pool \"test-pool-1\" created", "Make sure test pool is created")
	require.Contains(s.T(), sout, "storageclass \"rook-block\" created", "Make sure storageclass is created")
	require.Empty(s.T(), serr, "Make sure there are no errors")
	require.Equal(s.T(), 0, status)

	//make sure storageclass is created
	present, err := s.kh.IsStorageClassPresent("rook-block")
	require.Nil(s.T(), err)
	require.True(s.T(), present, "Make sure storageclass is present")

	//create pvc
	sout1, serr1, status1 := s.pvcOperation(claimName, "create")
	require.Contains(s.T(), sout1, "persistentvolumeclaim \"test-claim1\" created", "Make sure pvc is created")
	require.Empty(s.T(), serr1, "Make sure there are no errors")
	require.Equal(s.T(), 0, status1)

	//check status of PVC
	require.True(s.T(), s.isPVCBound(claimName))

	//check block image count
	b, _ := s.bc.BlockList()
	require.Equal(s.T(), s.init_blockCount+1, len(b), "Make sure new block image is created")

}

// Test case when persistentvolumeclaim is created for a valid storage class twice
func (s *K8sBlockImageCreateSuite) TestCreateSamePVCTwice() {

	s.T().Log("Test PVC(create block images) when storage class is not created")
	s.T().Log("Test creating PVC(block images) when storage class is created")
	//create pool and storageclass
	sout, serr, status := s.storageClassOperation("test-pool-1", "create")
	require.Contains(s.T(), sout, "pool \"test-pool-1\" created", "Make sure test pool is created")
	require.Contains(s.T(), sout, "storageclass \"rook-block\" created", "Make sure storageclass is created")
	require.Empty(s.T(), serr, "Make sure there are no errors")
	require.Equal(s.T(), 0, status)

	//make sure storageclass is created
	present, err := s.kh.IsStorageClassPresent("rook-block")
	require.Nil(s.T(), err)
	require.True(s.T(), present, "Make sure storageclass is present")

	//create pvc
	sout1, serr1, status1 := s.pvcOperation(claimName, "create")
	require.Contains(s.T(), sout1, "persistentvolumeclaim \"test-claim1\" created", "Make sure pvc is created")
	require.Empty(s.T(), serr1, "Make sure there are no errors")
	require.Equal(s.T(), 0, status1)

	//check status of PVC
	require.True(s.T(), s.isPVCBound(claimName))

	b1, _ := s.bc.BlockList()
	require.Equal(s.T(), s.init_blockCount+1, len(b1), "Make sure new block image is created")

	//Create same pvc again
	sout2, serr2, status2 := s.pvcOperation(claimName, "create")
	require.Empty(s.T(), sout2, "Make sure there are no output")
	require.Contains(s.T(), serr2, "persistentvolumeclaims \"test-claim1\" already exists", "Check Error")
	require.Equal(s.T(), 0, status2)

	//check status of PVC
	require.True(s.T(), s.isPVCBound(claimName))

	//check bock image count
	b2, _ := s.bc.BlockList()
	require.Equal(s.T(), len(b1), len(b2), "Make sure new block image is created")

}

func (s *K8sBlockImageCreateSuite) TearDownTest() {

	s.pvcOperation(claimName, "delete")
	s.storageClassOperation("test-pool-1", "delete")

}

func (s *K8sBlockImageCreateSuite) storageClassOperation(poolName string, action string) (string, string, int) {

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

func (s *K8sBlockImageCreateSuite) pvcOperation(claimName string, action string) (string, string, int) {
	t, _ := template.ParseFiles(s.pvPath)

	var tpl bytes.Buffer
	config := map[string]string{
		"claimName": claimName,
	}

	t.Execute(&tpl, config)

	cmdStruct := objects.CommandArgs{Command: "kubectl", PipeToStdIn: tpl.String(), CmdArgs: []string{action, "-f", "-"}}

	cmdOut := utils.ExecuteCommand(cmdStruct)

	return cmdOut.StdOut, cmdOut.StdErr, cmdOut.ExitCode

}

func (s *K8sBlockImageCreateSuite) isPVCBound(name string) bool {
	inc := 0
	for inc < 10 {
		status, _ := s.kh.GetPVCStatus(claimName)
		if strings.TrimRight(status, "\n") == "'Bound'" {
			return true
		}
		time.Sleep(time.Second * 1)
		inc++

	}
	return false
}
