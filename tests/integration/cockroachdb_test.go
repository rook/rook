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
	"strings"
	"testing"
	"time"

	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ************************************************
// *** Major scenarios tested by the CockroachDBSuite ***
// Setup
// - via the cluster CRD with very simple properties
//   - 3 replicas
//   - default service ports
//   - insecure
//   - 1Gi volume from default provider
//   - 25% cache, 25% maxSQLMemory
// ************************************************
func TestCockroachDBSuite(t *testing.T) {
	s := new(CockroachDBSuite)
	defer func(s *CockroachDBSuite) {
		HandlePanics(recover(), s, s.T)
	}(s)
	suite.Run(t, s)
}

type CockroachDBSuite struct {
	suite.Suite
	k8sHelper       *utils.K8sHelper
	installHelper   *installer.InstallHelper
	namespace       string
	systemNamespace string
	instanceCount   int
}

func (suite *CockroachDBSuite) SetupSuite() {
	suite.SetUp()
}

func (suite *CockroachDBSuite) TearDownSuite() {
	suite.TearDown()
}

func (suite *CockroachDBSuite) SetUp() {
	suite.namespace = "cockroachdb-ns"
	suite.systemNamespace = installer.SystemNamespace(suite.namespace)
	suite.instanceCount = 1

	k8sHelper, err := utils.CreateK8sHelper(suite.T)
	require.NoError(suite.T(), err)
	suite.k8sHelper = k8sHelper

	k8sversion := suite.k8sHelper.GetK8sServerVersion()
	logger.Infof("Installing cockroachdb on k8s %s", k8sversion)

	suite.installHelper = installer.NewK8sRookhelper(suite.k8sHelper.Clientset, suite.T)

	err = suite.installHelper.InstallCockroachDB(suite.systemNamespace, suite.namespace, suite.instanceCount)
	if err != nil {
		logger.Errorf("cockroachdb was not installed successfully: %+v", err)
		suite.T().Fail()
		suite.TearDown()
		suite.T().FailNow()
	}
}

func (suite *CockroachDBSuite) TearDown() {
	if suite.T().Failed() {
		installer.GatherCockroachDBDebuggingInfo(suite.k8sHelper, suite.systemNamespace)
		installer.GatherCockroachDBDebuggingInfo(suite.k8sHelper, suite.namespace)
		suite.installHelper.GatherAllCockroachDBLogs(suite.systemNamespace, suite.namespace, suite.T().Name())
	}
	suite.installHelper.UninstallCockroachDB(suite.systemNamespace, suite.namespace)
}

func (suite *CockroachDBSuite) TestCockroachDBClusterInstallation() {
	logger.Infof("Verifying that all expected pods in cockroachdb cluster %s are running", suite.namespace)

	// verify cockroachdb operator is running OK
	assert.True(suite.T(), suite.k8sHelper.CheckPodCountAndState("rook-cockroachdb-operator", suite.systemNamespace, 1, "Running"),
		"1 rook-cockroachdb-operator must be in Running state")

	// verify cockroachdb cluster instances are running OK
	assert.True(suite.T(), suite.k8sHelper.CheckPodCountAndState("rook-cockroachdb", suite.namespace, suite.instanceCount, "Running"),
		fmt.Sprintf("%d rook-cockroachdb pods must be in Running state", suite.instanceCount))

	// determine the cockroachdb operator pod name
	podNames, err := suite.k8sHelper.GetPodNamesForApp("rook-cockroachdb-operator", suite.systemNamespace)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(podNames))
	operatorPodName := podNames[0]

	// execute a sql command via exec in the cockroachdb operator pod to verify the database is functional
	command := "/cockroach/cockroach"
	commandArgs := []string{
		"sql", "--insecure", fmt.Sprintf("--host=cockroachdb-public.%s", suite.namespace), "-e",
		`create database rookcockroachdb; use rookcockroachdb; create table testtable ( testID int ); insert into testtable values (123456789); select * from testtable;`,
	}

	inc := 0
	var result string
	for inc < utils.RetryLoop {
		result, err = suite.k8sHelper.Exec(suite.systemNamespace, operatorPodName, command, commandArgs)
		logger.Infof("cockroachdb sql command exited, err: %+v. result: %s", err, result)
		if err == nil {
			break
		}
		logger.Warning("cockroachdb sql command failed, will try again")
		inc++
		time.Sleep(utils.RetryInterval * time.Second)
	}

	assert.NoError(suite.T(), err)
	assert.True(suite.T(), strings.Contains(result, "testid"))
	assert.True(suite.T(), strings.Contains(result, "123456789"))
}
