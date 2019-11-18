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
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/suite"
)

// ************************************************
// *** Major scenarios tested by the SmokeSuite ***
// Setup
// - via the cluster CRD
// Monitors
// - Three mons in the cluster
// - Failover of an unhealthy monitor
// OSDs
// - Bluestore running on devices
// Block
// - Mount/unmount a block device through the dynamic provisioner
// - Fencing of the block device
// - Read/write to the device
// File system
// - Create the file system via the CRD
// - Mount/unmount a file system in pod
// - Read/write to the file system
// - Delete the file system
// Object
// - Create the object store via the CRD
// - Create/delete buckets
// - Create/delete users
// - PUT/GET objects
// ************************************************
func TestCephSmokeSuite(t *testing.T) {
	if installer.SkipTestSuite(installer.CephTestSuite) {
		t.Skip()
	}

	s := new(SmokeSuite)
	defer func(s *SmokeSuite) {
		HandlePanics(recover(), s.op, s.T)
	}(s)
	suite.Run(t, s)
}

type SmokeSuite struct {
	suite.Suite
	helper    *clients.TestClient
	op        *TestCluster
	k8sh      *utils.K8sHelper
	namespace string
}

func (suite *SmokeSuite) SetupSuite() {
	suite.namespace = "smoke-ns"
	useDevices := true
	mons := 3
	rbdMirrorWorkers := 1
	suite.op, suite.k8sh = StartTestCluster(suite.T, smokeSuiteMinimalTestVersion, suite.namespace, "bluestore", false, useDevices, mons, rbdMirrorWorkers, installer.VersionMaster, installer.NautilusVersion)
	suite.helper = clients.CreateTestClient(suite.k8sh, suite.op.installer.Manifests)
}

func (suite *SmokeSuite) AfterTest(suiteName, testName string) {
	suite.op.installer.CollectOperatorLog(suiteName, testName, installer.SystemNamespace(suite.namespace))
}

func (suite *SmokeSuite) TearDownSuite() {
	suite.op.Teardown()
}

func (suite *SmokeSuite) TestBlockCSI_SmokeTest() {
	runCephCSIE2ETest(suite.helper, suite.k8sh, suite.Suite, suite.op.T(), suite.namespace)
}

func (suite *SmokeSuite) TestBlockStorage_SmokeTest() {
	runBlockE2ETest(suite.helper, suite.k8sh, suite.Suite, suite.namespace)
}

func (suite *SmokeSuite) TestFileStorage_SmokeTest() {
	runFileE2ETest(suite.helper, suite.k8sh, suite.Suite, suite.namespace, "smoke-test-fs")
}

func (suite *SmokeSuite) TestObjectStorage_SmokeTest() {
	runObjectE2ETest(suite.helper, suite.k8sh, suite.Suite, suite.namespace)
}

// Test to make sure all rook components are installed and Running
func (suite *SmokeSuite) TestRookClusterInstallation_SmokeTest() {
	checkIfRookClusterIsInstalled(suite.Suite, suite.k8sh, installer.SystemNamespace(suite.namespace), suite.namespace, 3)
}
