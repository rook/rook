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
	if installer.SkipTestSuite(installer.NFSTestSuite) {
		t.Skip()
	}

	s := new(NfsSuite)
	defer func(s *NfsSuite) {
		HandlePanics(recover(), s, s.T)
	}(s)
	suite.Run(t, s)
}

type NfsSuite struct {
	suite.Suite
	k8shelper       *utils.K8sHelper
	installer       *installer.NFSInstaller
	rwClient        *clients.ReadWriteOperation
	namespace       string
	systemNamespace string
	instanceCount   int
}

func (suite *NfsSuite) SetupSuite() {
	suite.Setup()
}

func (suite *NfsSuite) TearDownSuite() {
	suite.Teardown()
}

func (suite *NfsSuite) Setup() {
	suite.namespace = "nfs-ns"
	suite.systemNamespace = installer.SystemNamespace(suite.namespace)
	suite.instanceCount = 1

	k8shelper, err := utils.CreateK8sHelper(suite.T)
	require.NoError(suite.T(), err)
	suite.k8shelper = k8shelper

	k8sversion := suite.k8shelper.GetK8sServerVersion()
	logger.Infof("Installing nfs server on k8s %s", k8sversion)

	suite.installer = installer.NewNFSInstaller(suite.k8shelper, suite.T)

	suite.rwClient = clients.CreateReadWriteOperation(suite.k8shelper)

	err = suite.installer.InstallNFSServer(suite.systemNamespace, suite.namespace, suite.instanceCount)
	if err != nil {
		logger.Errorf("nfs server installation failed: %+v", err)
		suite.T().Fail()
		suite.Teardown()
		suite.T().FailNow()
	}
}

func (suite *NfsSuite) Teardown() {
	suite.installer.GatherAllNFSServerLogs(suite.systemNamespace, suite.namespace, suite.T().Name())
	suite.installer.UninstallNFSServer(suite.systemNamespace, suite.namespace)
}

func (suite *NfsSuite) TestNfsServerInstallation() {
	logger.Infof("Verifying that nfs server pod %s is running", suite.namespace)

	// verify nfs server operator is running OK
	assert.True(suite.T(), suite.k8shelper.CheckPodCountAndState("rook-nfs-operator", suite.systemNamespace, 1, "Running"),
		"1 rook-nfs-operator must be in Running state")

	// verify nfs server instances are running OK
	assert.True(suite.T(), suite.k8shelper.CheckPodCountAndState(suite.namespace, suite.namespace, suite.instanceCount, "Running"),
		fmt.Sprintf("%d rook-nfs pods must be in Running state", suite.instanceCount))

	// verify bigger export is running OK
	assert.True(suite.T(), true, suite.k8shelper.WaitUntilPVCIsBound("default", "nfs-pv-claim-bigger"))

	podList, err := suite.rwClient.CreateWriteClient("nfs-pv-claim-bigger")
	require.NoError(suite.T(), err)
	assert.True(suite.T(), true, suite.checkReadData(podList))
	suite.rwClient.Delete()

	// verify another smaller export is running OK
	assert.True(suite.T(), true, suite.k8shelper.WaitUntilPVCIsBound("default", "nfs-pv-claim"))

	defer suite.rwClient.Delete()
	podList, err = suite.rwClient.CreateWriteClient("nfs-pv-claim")
	require.NoError(suite.T(), err)
	assert.True(suite.T(), true, suite.checkReadData(podList))
}

func (suite *NfsSuite) checkReadData(podList []string) bool {
	inc := 0
	var result string
	var err error
	// the following for loop retries to read data from the first pod in the pod list
	for inc < utils.RetryLoop {
		// the nfs volume is mounted on "/mnt" and the data(hostname of the pod) is written in "/mnt/data" of the pod
		// results stores the hostname of either one of the pod which is same as the pod name, which is read from "/mnt/data"
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
	// the value of result must be same as the name of pod.
	if result == podList[0] || result == podList[1] {
		return true
	}

	return false
}
