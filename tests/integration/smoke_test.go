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
	"testing"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/enums"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestSmokeSuiteK8s(t *testing.T) {
	suite.Run(t, new(SmokeSuite))
}

type SmokeSuite struct {
	suite.Suite
	helper    *clients.TestClient
	k8sh      *utils.K8sHelper
	installer *installer.InstallHelper
}

func (suite *SmokeSuite) SetupSuite() {
	kh, err := utils.CreateK8sHelper(suite.T)
	require.NoError(suite.T(), err)

	suite.k8sh = kh

	suite.installer = installer.NewK8sRookhelper(kh.Clientset, suite.T)

	err = suite.installer.InstallRookOnK8s(defaultRookNamespace)
	require.NoError(suite.T(), err)

	suite.helper, err = clients.CreateTestClient(enums.Kubernetes, kh, defaultRookNamespace)
	require.Nil(suite.T(), err)
}

func (suite *SmokeSuite) TearDownSuite() {
	if suite.T().Failed() {
		gatherAllRookLogs(suite.k8sh, suite.Suite, defaultRookSystemNamespace, defaultRookNamespace)

	}
	suite.installer.UninstallRookFromK8s(defaultRookNamespace, false)
}

func (suite *SmokeSuite) TestBlockStorage_SmokeTest() {
	runBlockE2ETest(suite.helper, suite.k8sh, suite.Suite, defaultRookNamespace)
}
func (suite *SmokeSuite) TestFileStorage_SmokeTest() {
	runFileE2ETest(suite.helper, suite.k8sh, suite.Suite, defaultRookNamespace, "smoke-test-fs")
}
func (suite *SmokeSuite) TestObjectStorage_SmokeTest() {
	runObjectE2ETest(suite.helper, suite.k8sh, suite.Suite, defaultRookNamespace)
}

//Test to make sure all rook components are installed and Running
func (suite *SmokeSuite) TestRookClusterInstallation_smokeTest() {
	checkIfRookClusterIsInstalled(suite.k8sh, suite.Suite, defaultRookSystemNamespace, defaultRookNamespace)
}
