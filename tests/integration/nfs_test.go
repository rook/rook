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
	"fmt"
	"testing"
	"time"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// *******************************************************
// *** Major scenarios tested by the NfsSuite ***
// Setup
// - via the server CRD with very simple properties
//   - 1 replica
//   - Default server permissions
//   - Mount a NFS export and write data to it and verify
// *******************************************************
func TestNfsSuite(t *testing.T) {
	s := new(NfsSuite)
	defer func(s *NfsSuite) {
		HandlePanics(recover(), s, s.T)
	}(s)
	suite.Run(t, s)
}

type NfsSuite struct {
	suite.Suite
	k8sHelper       *utils.K8sHelper
	installHelper   *installer.InstallHelper
	rwClient        *clients.ReadWriteOperation
	namespace       string
	systemNamespace string
	instanceCount   int
}

func (suite *NfsSuite) SetupSuite() {
	suite.SetUp()
}

func (suite *NfsSuite) TearDownSuite() {
	suite.TearDown()
}

func (suite *NfsSuite) SetUp() {
	suite.namespace = "nfs-ns"
	suite.systemNamespace = installer.SystemNamespace(suite.namespace)
	suite.instanceCount = 1

	k8sHelper, err := utils.CreateK8sHelper(suite.T)
	require.NoError(suite.T(), err)
	suite.k8sHelper = k8sHelper

	k8sversion := suite.k8sHelper.GetK8sServerVersion()
	logger.Infof("Installing nfs server on k8s %s", k8sversion)

	suite.installHelper = installer.NewK8sRookhelper(suite.k8sHelper.Clientset, suite.T)

	suite.rwClient = clients.CreateReadWriteOperation(suite.k8sHelper)

	err = suite.installHelper.InstallNFSServer(suite.systemNamespace, suite.namespace, suite.instanceCount)
	if err != nil {
		logger.Errorf("nfs server installation failed: %+v", err)
		suite.T().Fail()
		suite.TearDown()
		suite.T().FailNow()
	}
}

func (suite *NfsSuite) TearDown() {
	if suite.T().Failed() {
		installer.GatherNFSServerDebuggingInfo(suite.k8sHelper, suite.systemNamespace)
		installer.GatherNFSServerDebuggingInfo(suite.k8sHelper, suite.namespace)
		suite.installHelper.GatherAllNFSServerLogs(suite.systemNamespace, suite.namespace, suite.T().Name())
	}
	suite.installHelper.UninstallNFSServer(suite.systemNamespace, suite.namespace)
}

func (suite *NfsSuite) TestNfsServerInstallation() {
	logger.Infof("Verifying that nfs server pod %s is running", suite.namespace)

	// verify nfs server operator is running OK
	assert.True(suite.T(), suite.k8sHelper.CheckPodCountAndState("rook-nfs-operator", suite.systemNamespace, 1, "Running"),
		"1 rook-nfs-operator must be in Running state")

	// verify nfs server instances are running OK
	assert.True(suite.T(), suite.k8sHelper.CheckPodCountAndState("rook-nfs", suite.namespace, suite.instanceCount, "Running"),
		fmt.Sprintf("%d rook-nfs pods must be in Running state", suite.instanceCount))

	// determine the nfs operator pod name
	podNames, err := suite.k8sHelper.GetPodNamesForApp("rook-nfs-operator", suite.systemNamespace)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(podNames))

	// verify nfs server storage
	nfsPVC, err := suite.k8sHelper.GetPVCStatus("default", "nfs-pv-claim")
	require.NoError(suite.T(), err)
	assert.Contains(suite.T(), nfsPVC, "Bound")

	defer suite.rwClient.Delete()
	podList, err := suite.rwClient.CreateWriteClient("nfs-pv-claim")
	require.NoError(suite.T(), err)
	assert.True(suite.T(), true, suite.checkReadData(podList))
}

func (suite *NfsSuite) checkReadData(podList []string) bool {
	inc := 0
	var result string
	var err error
	for inc < utils.RetryLoop {
		result, err = suite.rwClient.Read(podList[0])
		logger.Infof("nfs volume read exited, err: %+v. result: %s", err, result)
		if err == nil {
			break
		}
		logger.Warning("nfs volume read failed, will try again")
		inc++
		time.Sleep(utils.RetryInterval * time.Second)
	}
	require.NoError(suite.T(), err)
	if result == podList[0] || result == podList[1] {
		return true
	}

	return false
}
