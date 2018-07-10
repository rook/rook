/*
Copyright 2018 The Rook Authors. All rights reserved.

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

	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestMinioSuite(t *testing.T) {
	s := new(MinioSuite)
	defer func(s *MinioSuite) {
		HandlePanics(recover(), s, s.T)
	}(s)
	suite.Run(t, s)
}

type MinioSuite struct {
	suite.Suite
	k8sHelper       *utils.K8sHelper
	installHelper   *installer.InstallHelper
	namespace       string
	systemNamespace string
	instanceCount   int
}

func (suite *MinioSuite) SetupSuite() {
	suite.SetUp()
}

func (suite *MinioSuite) TearDownSuite() {
	suite.TearDown()
}

func (suite *MinioSuite) SetUp() {
	suite.namespace = "minio-ns"
	suite.systemNamespace = suite.namespace
	suite.instanceCount = 4

	k8sHelper, err := utils.CreateK8sHelper(suite.T)
	require.NoError(suite.T(), err)
	suite.k8sHelper = k8sHelper

	k8sversion := suite.k8sHelper.GetK8sServerVersion()
	logger.Infof("Installing minio on k8s %s", k8sversion)

	suite.installHelper = installer.NewK8sRookhelper(suite.k8sHelper.Clientset, suite.T)

	err = suite.installHelper.InstallMinio(suite.systemNamespace, suite.namespace, suite.instanceCount)
	if err != nil {
		logger.Errorf("minio was not installed successfully: %+v", err)
		suite.T().Fail()
		suite.TearDown()
		suite.T().FailNow()
	}
}

func (suite *MinioSuite) TearDown() {
	if suite.T().Failed() {
		installer.GatherMinioDebuggingInfo(suite.k8sHelper, suite.systemNamespace)
		installer.GatherMinioDebuggingInfo(suite.k8sHelper, suite.namespace)
		suite.installHelper.GatherAllMinioLogs(suite.systemNamespace, suite.namespace, suite.T().Name())
	}
	suite.installHelper.UninstallMinio(suite.systemNamespace, suite.namespace)
}

func (suite *MinioSuite) TestMinioClusterInstallation() {
	logger.Infof("Verifying that all expected pods in minio cluster %s are running", suite.namespace)

	// verify minio operator is running OK
	assert.True(suite.T(), suite.k8sHelper.CheckPodCountAndState("rook-minio-operator", suite.systemNamespace, 1, "Running"),
		"1 rook-minio-operator must be in Running state")

	// verify minio cluster instances are running OK
	assert.True(suite.T(), suite.k8sHelper.CheckPodCountAndState("rook-minio", suite.namespace, suite.instanceCount, "Running"),
		fmt.Sprintf("%d rook-minio pods must be in Running state", suite.instanceCount))

	// determine the minio operator pod name
	podNames, err := suite.k8sHelper.GetPodNamesForApp("rook-minio-operator", suite.systemNamespace)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(podNames))

	assert.NoError(suite.T(), err)
}
