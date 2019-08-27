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

type YugabyteDBSuite struct {
	k8sHelper     *utils.K8sHelper
	ybdbInstaller *installer.YugabyteDBInstaller
	namespace     string
	systemNS      string
	replicaCount  int
	suite.Suite
}

func TestYugabyteDBSuite(t *testing.T) {
	if installer.SkipTestSuite(installer.YugabyteDBTestSuite) {
		t.Skip()
	}

	y := new(YugabyteDBSuite)
	defer func(y *YugabyteDBSuite) {
		HandlePanics(recover(), y, y.T)
	}(y)
	suite.Run(t, y)
}

func (y *YugabyteDBSuite) SetupSuite() {
	y.Setup()
}

func (y *YugabyteDBSuite) TearDownSuite() {
	y.Teardown()
}

func (y *YugabyteDBSuite) Setup() {
	k8sHelperObj, err := utils.CreateK8sHelper(y.T)
	require.NoError(y.T(), err)
	y.k8sHelper = k8sHelperObj

	logger.Info("YugabyteDB integration test setup started.")
	y.namespace = "rook-yugabytedb"
	y.systemNS = installer.SystemNamespace(y.namespace)
	y.replicaCount = 1

	k8sversion := y.k8sHelper.GetK8sServerVersion()
	logger.Infof("Installing yugabytedb on kubernetes %s", k8sversion)

	y.ybdbInstaller = installer.NewYugabyteDBInstaller(y.T, k8sHelperObj)

	err = y.ybdbInstaller.InstallYugabyteDB(y.systemNS, y.namespace, y.replicaCount)
	if err != nil {
		logger.Errorf("yugabytedb installation failed with error: %+v", err)
		y.T().Fail()
		y.Teardown()
		y.T().FailNow()
	}
}

func (y *YugabyteDBSuite) Teardown() {
	y.ybdbInstaller.GatherAllLogs(y.systemNS, y.namespace, y.T().Name())
	y.ybdbInstaller.RemoveAllYugabyteDBResources(y.systemNS, y.namespace)
}

func (y *YugabyteDBSuite) TestYBClusterComponents() {
	logger.Info("Verifying yugabytedb cluster is created & has all required components")

	// verify operator pod is running
	assert.True(y.T(), y.k8sHelper.CheckPodCountAndState("rook-yugabytedb-operator", y.systemNS, 1, "Running"),
		"1 rook-yugabytedb-operator must be in Running state")

	// verify master pod is running
	assert.True(y.T(), y.k8sHelper.CheckPodCountAndState("yb-master-rook-yugabytedb", y.namespace, y.replicaCount, "Running"),
		fmt.Sprintf("%d yb-master must be in Running state", y.replicaCount))

	// verify master pod is running
	assert.True(y.T(), y.k8sHelper.CheckPodCountAndState("yb-tserver-rook-yugabytedb", y.namespace, y.replicaCount, "Running"),
		fmt.Sprintf("%d yb-tserver must be in Running state", y.replicaCount))
}
