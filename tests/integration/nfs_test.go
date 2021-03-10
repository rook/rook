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
	"k8s.io/apimachinery/pkg/util/version"
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
		HandlePanics(recover(), s.Teardown, s.T)
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

func (s *NfsSuite) SetupSuite() {
	s.Setup()
}

func (s *NfsSuite) TearDownSuite() {
	s.Teardown()
}

func (s *NfsSuite) Setup() {
	s.namespace = "rook-nfs"
	s.systemNamespace = installer.SystemNamespace(s.namespace)
	s.instanceCount = 1

	k8shelper, err := utils.CreateK8sHelper(s.T)
	v := version.MustParseSemantic(k8shelper.GetK8sServerVersion())
	if !v.AtLeast(version.MustParseSemantic("1.14.0")) {
		logger.Info("Skipping NFS tests when not at least K8s v1.14")
		s.T().Skip()
	}

	require.NoError(s.T(), err)
	s.k8shelper = k8shelper

	k8sversion := s.k8shelper.GetK8sServerVersion()
	logger.Infof("Installing nfs server on k8s %s", k8sversion)

	s.installer = installer.NewNFSInstaller(s.k8shelper, s.T)

	s.rwClient = clients.CreateReadWriteOperation(s.k8shelper)

	err = s.installer.InstallNFSServer(s.systemNamespace, s.namespace, s.instanceCount)
	if err != nil {
		logger.Errorf("nfs server installation failed: %+v", err)
		s.T().Fail()
		s.Teardown()
		s.T().FailNow()
	}
}

func (s *NfsSuite) Teardown() {
	s.installer.GatherAllNFSServerLogs(s.systemNamespace, s.namespace, s.T().Name())
	s.installer.UninstallNFSServer(s.systemNamespace, s.namespace)
}

func (s *NfsSuite) TestNfsServerInstallation() {
	logger.Infof("Verifying that nfs server pod %s is running", s.namespace)

	// verify nfs server operator is running OK
	assert.True(s.T(), s.k8shelper.CheckPodCountAndState("rook-nfs-operator", s.systemNamespace, 1, "Running"),
		"1 rook-nfs-operator must be in Running state")

	// verify nfs server instances are running OK
	assert.True(s.T(), s.k8shelper.CheckPodCountAndState(s.namespace, s.namespace, s.instanceCount, "Running"),
		fmt.Sprintf("%d rook-nfs pods must be in Running state", s.instanceCount))

	// verify bigger export is running OK
	assert.True(s.T(), true, s.k8shelper.WaitUntilPVCIsBound("default", "nfs-pv-claim-bigger"))

	podList, err := s.rwClient.CreateWriteClient("nfs-pv-claim-bigger")
	require.NoError(s.T(), err)
	assert.True(s.T(), true, s.checkReadData(podList))
	err = s.rwClient.Delete()
	assert.NoError(s.T(), err)

	// verify another smaller export is running OK
	assert.True(s.T(), true, s.k8shelper.WaitUntilPVCIsBound("default", "nfs-pv-claim"))

	defer s.rwClient.Delete() //nolint, delete a nfs consuming pod in rook
	podList, err = s.rwClient.CreateWriteClient("nfs-pv-claim")
	require.NoError(s.T(), err)
	assert.True(s.T(), true, s.checkReadData(podList))
}

func (s *NfsSuite) checkReadData(podList []string) bool {
	var result string
	var err error
	// the following for loop retries to read data from the first pod in the pod list
	for i := 0; i < utils.RetryLoop; i++ {
		// the nfs volume is mounted on "/mnt" and the data(hostname of the pod) is written in "/mnt/data" of the pod
		// results stores the hostname of either one of the pod which is same as the pod name, which is read from "/mnt/data"
		result, err = s.rwClient.Read(podList[0])
		logger.Infof("nfs volume read exited, err: %+v. result: %s", err, result)
		if err == nil {
			break
		}
		logger.Warning("nfs volume read failed, will try again")
		time.Sleep(utils.RetryInterval * time.Second)
	}
	require.NoError(s.T(), err)
	// the value of result must be same as the name of pod.
	if result == podList[0] || result == podList[1] {
		return true
	}

	return false
}
