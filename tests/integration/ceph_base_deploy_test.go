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
	"os"
	"strings"
	"time"

	"testing"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	defaultNamespace = "default"
	// UPDATE these versions when the integration test matrix changes
	// These versions are for running a minimal test suite for more efficient tests across different versions of K8s
	// instead of running all suites on all versions
	// To run on multiple versions, add a comma separate list such as 1.16.0,1.17.0
	blockMinimalTestVersion        = "1.13.0"
	multiClusterMinimalTestVersion = "1.14.0"
	helmMinimalTestVersion         = "1.15.0"
	upgradeMinimalTestVersion      = "1.16.0"
	smokeSuiteMinimalTestVersion   = "1.17.0"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "integrationTest")
)

// Test to make sure all rook components are installed and Running
func checkIfRookClusterIsInstalled(s suite.Suite, k8sh *utils.K8sHelper, opNamespace, clusterNamespace string, mons int) {
	logger.Infof("Make sure all Pods in Rook Cluster %s are running", clusterNamespace)
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-operator", opNamespace, 1, "Running"),
		"Make sure there is 1 rook-operator present in Running state")
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-mgr", clusterNamespace, 1, "Running"),
		"Make sure there is 1 rook-ceph-mgr present in Running state")
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-osd", clusterNamespace, 1, "Running"),
		"Make sure there is at lest 1 rook-ceph-osd present in Running state")
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-mon", clusterNamespace, mons, "Running"),
		fmt.Sprintf("Make sure there are %d rook-ceph-mon present in Running state", mons))
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-crashcollector", clusterNamespace, 1, "Running"),
		"Make sure there is at lest 1 rook-ceph-crash present in Running state")
}

func checkIfRookClusterIsHealthy(s suite.Suite, testClient *clients.TestClient, clusterNamespace string) {
	logger.Infof("Testing cluster %s health", clusterNamespace)
	var err error

	retryCount := 0
	for retryCount < utils.RetryLoop {
		err = clients.IsClusterHealthy(testClient, clusterNamespace)
		if err == nil {
			logger.Infof("cluster %s is healthy", clusterNamespace)
			return
		}

		retryCount++
		logger.Infof("waiting for cluster %s to become healthy. err: %+v", clusterNamespace, err)
		<-time.After(time.Duration(utils.RetryInterval) * time.Second)
	}

	require.Nil(s.T(), err)
}

func HandlePanics(r interface{}, op installer.TestSuite, t func() *testing.T) {
	if r != nil {
		logger.Infof("unexpected panic occurred during test %s, --> %v", t().Name(), r)
		t().Fail()
		op.Teardown()
		t().FailNow()
	}
}

// TestCluster struct for handling panic and test suite tear down
type TestCluster struct {
	installer        *installer.CephInstaller
	kh               *utils.K8sHelper
	helper           *clients.TestClient
	T                func() *testing.T
	namespace        string
	storeType        string
	useDevices       bool
	mons             int
	rbdMirrorWorkers int
}

func checkIfShouldRunForMinimalTestMatrix(t func() *testing.T, k8sh *utils.K8sHelper, version string) {
	testArgs := os.Getenv("TEST_ARGUMENTS")
	if !strings.Contains(testArgs, "min-test-matrix") {
		logger.Infof("running all tests")
		return
	}
	versions := strings.Split(version, ",")
	logger.Infof("checking if tests are running on k8s %q", version)
	matchedVersion := false
	kubeVersion := ""
	for _, v := range versions {
		kubeVersion, matchedVersion = k8sh.VersionMinorMatches(v)
		if matchedVersion {
			break
		}
	}
	if !matchedVersion {
		logger.Infof("Skipping test suite since kube version %q does not match", kubeVersion)
		t().Skip()
	}
	logger.Infof("Running test suite since kube version is %q", kubeVersion)
}

// StartTestCluster creates new instance of TestCluster struct
func StartTestCluster(t func() *testing.T, minimalMatrixK8sVersion, namespace, storeType string, useHelm, useDevices bool, mons,
	rbdMirrorWorkers int, rookVersion string, cephVersion cephv1.CephVersionSpec) (*TestCluster, *utils.K8sHelper) {

	kh, err := utils.CreateK8sHelper(t)
	require.NoError(t(), err)
	checkIfShouldRunForMinimalTestMatrix(t, kh, minimalMatrixK8sVersion)

	i := installer.NewCephInstaller(t, kh.Clientset, useHelm, rookVersion, cephVersion)

	op := &TestCluster{i, kh, nil, t, namespace, storeType, useDevices, mons, rbdMirrorWorkers}

	if rookVersion != installer.VersionMaster {
		// make sure we have the images from a previous release locally so the test doesn't hit a timeout
		assert.NoError(t(), kh.GetDockerImage("rook/ceph:"+rookVersion))
	}

	assert.NoError(t(), kh.GetDockerImage(cephVersion.Image))

	op.Setup()
	return op, kh
}

// SetUpRook is a wrapper for setting up rook
func (op *TestCluster) Setup() {
	isRookInstalled, err := op.installer.InstallRookOnK8sWithHostPathAndDevices(op.namespace, op.storeType,
		op.useDevices, cephv1.MonSpec{Count: op.mons, AllowMultiplePerNode: true}, false /* startWithAllNodes */, op.rbdMirrorWorkers)

	if !isRookInstalled || err != nil {
		logger.Errorf("Rook was not installed successfully: %v", err)
		if !op.installer.T().Failed() {
			op.installer.GatherAllRookLogs(op.installer.T().Name(), op.namespace, installer.SystemNamespace(op.namespace))
		}
		op.T().Fail()
		op.Teardown()
		op.T().FailNow()
	}
}

// SetInstallData updates the installer helper based on the version of Rook desired
func (op *TestCluster) SetInstallData(version string) {}

// TearDownRook is a wrapper for tearDown after Suite
func (op *TestCluster) Teardown() {
	op.installer.UninstallRook(op.namespace, true)
}
