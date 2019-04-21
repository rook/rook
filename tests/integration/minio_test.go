/*
Copyright 2019 The Rook Authors. All rights reserved.

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
	k8shelper       *utils.K8sHelper
	installer       *installer.MinioInstaller
	namespace       string
	systemNamespace string
	instanceCount   int
}

func (suite *MinioSuite) SetupSuite() {
	suite.Setup()
}

func (suite *MinioSuite) TearDownSuite() {
	suite.Teardown()
}

func (suite *MinioSuite) Setup() {
	suite.namespace = "minio-ns"
	suite.systemNamespace = installer.SystemNamespace(suite.namespace)
	suite.instanceCount = 1

	k8shelper, err := utils.CreateK8sHelper(suite.T)
	require.NoError(suite.T(), err)
	suite.k8shelper = k8shelper

	k8sversion := suite.k8shelper.GetK8sServerVersion()
	logger.Infof("Installing Minio on k8s %s", k8sversion)

	suite.installer = installer.NewMinioInstaller(suite.k8shelper, suite.T)

	err = suite.installer.InstallMinio(suite.systemNamespace, suite.namespace, suite.instanceCount)
	if err != nil {
		logger.Errorf("minio was not installed successfully: %+v", err)
		suite.T().Fail()
		suite.Teardown()
		suite.T().FailNow()
	}
}

func (suite *MinioSuite) Teardown() {
	if suite.T().Failed() {
		installer.GatherCRDObjectDebuggingInfo(suite.k8shelper, suite.systemNamespace)
		installer.GatherCRDObjectDebuggingInfo(suite.k8shelper, suite.namespace)
	}
	suite.installer.GatherAllMinioLogs(suite.systemNamespace, suite.namespace, suite.T().Name())
	suite.installer.UninstallMinio(suite.systemNamespace, suite.namespace)
}

func (suite *MinioSuite) TestMinioClusterInstallation() {
	logger.Infof("Verifying that all expected pods in minio cluster %s are running", suite.namespace)

	// verify minio operator is running OK
	assert.True(suite.T(), suite.k8shelper.CheckPodCountAndState("rook-minio-operator", suite.systemNamespace, 1, "Running"),
		"1 rook-minio-operator must be in Running state")

	// verify minio cluster instances are running OK
	assert.True(suite.T(), suite.k8shelper.CheckPodCountAndState("rook-minio", suite.namespace, suite.instanceCount, "Running"),
		fmt.Sprintf("%d rook-minio pods must be in Running state", suite.instanceCount))

	// determine the minio operator pod name
	podNames, err := suite.k8shelper.GetPodNamesForApp("rook-minio-operator", suite.systemNamespace)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(podNames))
	/*operatorPodName := podNames[0]

	command := "mc"
	commandArgs := []string{"mb", "play/mybucket"}

	inc := 0
	var result string
	for inc < utils.RetryLoop {
		result, err = suite.k8shelper.Exec(suite.systemNamespace, operatorPodName, command, commandArgs)
		logger.Infof("minio create bucket command exited, err: %+v. result: %s", err, result)
		if err == nil {
			break
		}
		logger.Warning("minio create bucket command failed, will try again")
		inc++
		time.Sleep(utils.RetryInterval * time.Second)
	}

	assert.NoError(suite.T(), err)
	// According to Minio docs, the expected output is "Bucket created successfully ‘play/mybucket’."
	// so we perform a sanity check
	assert.True(suite.T(), strings.Contains(result, "created successfully"))*/
}
