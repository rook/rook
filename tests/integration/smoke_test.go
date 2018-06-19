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
	"regexp"
	"strings"
	"testing"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/util/version"
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
func TestSmokeSuite(t *testing.T) {
	s := new(SmokeSuite)
	defer func(s *SmokeSuite) {
		HandlePanics(recover(), s.op, s.T)
	}(s)
	suite.Run(t, s)
}

type SmokeSuite struct {
	suite.Suite
	helper    *clients.TestClient
	op        contracts.Setup
	k8sh      *utils.K8sHelper
	namespace string
}

func (suite *SmokeSuite) SetupSuite() {
	suite.namespace = "smoke-ns"
	useDevices := true
	suite.op, suite.k8sh = StartBaseTestOperations(suite.T, suite.namespace, "bluestore", false, useDevices, 3)
	suite.helper = GetTestClient(suite.k8sh, suite.namespace, suite.op, suite.T)
}

func (suite *SmokeSuite) TearDownSuite() {
	suite.op.TearDown()
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

//Test to make sure all rook components are installed and Running
func (suite *SmokeSuite) TestRookClusterInstallation_smokeTest() {
	checkIfRookClusterIsInstalled(suite.Suite, suite.k8sh, installer.SystemNamespace(suite.namespace), suite.namespace, 3)
}

func (suite *SmokeSuite) TestOperatorGetFlexvolumePath() {
	v := version.MustParseSemantic(suite.k8sh.GetK8sServerVersion())
	if !v.LessThan(version.MustParseSemantic("1.9.0")) {
		suite.T().Skip("Skipping test - known issues with k8s 1.9 (https://github.com/rook/rook/issues/1330)")
	}
	// get the operator pod
	sysNamespace := installer.SystemNamespace(suite.namespace)
	listOpts := metav1.ListOptions{LabelSelector: "app=rook-ceph-operator"}
	podList, err := suite.k8sh.Clientset.CoreV1().Pods(sysNamespace).List(listOpts)
	require.Nil(suite.T(), err)
	require.Equal(suite.T(), 1, len(podList.Items))

	// get the raw log for the operator pod
	opPodName := podList.Items[0].Name
	rawLog, err := suite.k8sh.Clientset.CoreV1().Pods(sysNamespace).GetLogs(opPodName, &v1.PodLogOptions{}).Do().Raw()
	require.Nil(suite.T(), err)

	r := regexp.MustCompile(`discovered flexvolume dir path from source.*\n`)
	logStmt := string(r.Find(rawLog))
	logger.Infof("flexvolume discovery log statement: %s", logStmt)

	// verify that the volume plugin dir was discovered by the operator pod and that it did not come from
	// an env var or the default
	require.NotEmpty(suite.T(), logStmt)
	assert.True(suite.T(), strings.Contains(logStmt, "discovered flexvolume dir path from source"))
	assert.False(suite.T(), strings.Contains(logStmt, "discovered flexvolume dir path from source env var"))
	assert.False(suite.T(), strings.Contains(logStmt, "discovered flexvolume dir path from source default"))
}
