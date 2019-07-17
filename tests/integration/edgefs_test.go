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

// ************************************************
// *** Major scenarios tested by the Edgefs ***
// Setup via the cluster CRD with very simple properties
//   - all nodes deployment
//   - rtlfs (3 folders on each node)
//   - default cluster settings
//   - no services provided yet
//   - insecure
// ************************************************
type EdgefsSuite struct {
	suite.Suite
	k8shelper       *utils.K8sHelper
	installer       *installer.EdgefsInstaller
	namespace       string
	systemNamespace string
}

func TestEdgefsSuite(t *testing.T) {
	if installer.SkipTestSuite(installer.EdgeFSTestSuite) {
		t.Skip()
	}

	s := new(EdgefsSuite)
	defer func(s *EdgefsSuite) {
		HandlePanics(recover(), s, s.T)
	}(s)
	suite.Run(t, s)
}

func (suite *EdgefsSuite) SetupSuite() {
	suite.Setup()
}

func (suite *EdgefsSuite) TearDownSuite() {
	suite.Teardown()
}

func (suite *EdgefsSuite) Setup() {
	suite.namespace = "rook-edgefs"
	suite.systemNamespace = installer.SystemNamespace(suite.namespace)

	k8shelper, err := utils.CreateK8sHelper(suite.T)
	require.NoError(suite.T(), err)
	suite.k8shelper = k8shelper

	k8sversion := suite.k8shelper.GetK8sServerVersion()
	logger.Infof("Installing Edgefs on k8s %s", k8sversion)

	suite.installer = installer.NewEdgefsInstaller(suite.k8shelper, suite.T)

	err = suite.installer.InstallEdgefs(suite.systemNamespace, suite.namespace)
	if err != nil {
		logger.Errorf("Edgefs was not installed successfully: %+v", err)
		suite.T().Fail()
		suite.Teardown()
		suite.T().FailNow()
	}
}

func (suite *EdgefsSuite) Teardown() {
	suite.installer.GatherAllEdgefsLogs(suite.systemNamespace, suite.namespace, suite.T().Name())
	suite.installer.UninstallEdgefs(suite.systemNamespace, suite.namespace)
}

func (suite *EdgefsSuite) TestEdgefsClusterInstallation() {
	logger.Infof("Verifying that all expected pods in edgefs cluster %s are running", suite.namespace)
	// Verify edgefs operator is running OK
	assert.True(suite.T(), suite.k8shelper.CheckPodCountAndState("rook-edgefs-operator", suite.systemNamespace, 1, "Running"),
		"1 rook-edgefs-operator must be in Running state")

	// Verify edgefs cluster instances are running OK
	targetPods, err := suite.k8shelper.CountPodsWithLabel("app=rook-edgefs-target", suite.namespace)
	require.NoError(suite.T(), err)

	assert.True(suite.T(), suite.k8shelper.CheckPodCountAndState("rook-edgefs-target", suite.namespace, targetPods, "Running"),
		fmt.Sprintf("%d rook-edgefs pods must be in Running state", targetPods))

	podNames, err := suite.k8shelper.GetPodNamesForApp("rook-edgefs-operator", suite.systemNamespace)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(podNames))

	assert.NoError(suite.T(), err)
}
