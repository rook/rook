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
	flexDriverMinimalTestVersion      = "1.11.0,1.13.0"
	cephMasterSuiteMinimalTestVersion = "1.12.0"
	multiClusterMinimalTestVersion    = "1.15.0"
	helmMinimalTestVersion            = "1.17.0"
	upgradeMinimalTestVersion         = "1.18.0"
	smokeSuiteMinimalTestVersion      = "1.19.0"
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
		healthy, err := clients.IsClusterHealthy(testClient, clusterNamespace)
		if healthy {
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
	installer               *installer.CephInstaller
	kh                      *utils.K8sHelper
	helper                  *clients.TestClient
	T                       func() *testing.T
	clusterName             string
	namespace               string
	storeType               string
	storageClassName        string
	useHelm                 bool
	usePVC                  bool
	mons                    int
	rbdMirrorWorkers        int
	rookCephCleanup         bool
	skipOSDCreation         bool
	minimalMatrixK8sVersion string
	rookVersion             string
	cephVersion             cephv1.CephVersionSpec
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
func StartTestCluster(t func() *testing.T, cluster *TestCluster) (*TestCluster, *utils.K8sHelper) {
	kh, err := utils.CreateK8sHelper(t)
	require.NoError(t(), err)
	checkIfShouldRunForMinimalTestMatrix(t, kh, cluster.minimalMatrixK8sVersion)

	cluster.installer = installer.NewCephInstaller(t, kh.Clientset, cluster.useHelm, cluster.clusterName, cluster.rookVersion, cluster.cephVersion, cluster.rookCephCleanup)
	cluster.kh = kh
	cluster.helper = nil
	cluster.T = t

	if cluster.rookVersion != installer.VersionMaster {
		// make sure we have the images from a previous release locally so the test doesn't hit a timeout
		assert.NoError(t(), kh.GetDockerImage("rook/ceph:"+cluster.rookVersion))
	}

	assert.NoError(t(), kh.GetDockerImage(cluster.cephVersion.Image))

	cluster.Setup()
	return cluster, kh
}

// Setup is a wrapper for setting up rook
func (op *TestCluster) Setup() {
	// Turn on DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	isRookInstalled, err := op.installer.InstallRook(op.namespace, op.storeType, op.usePVC, op.storageClassName,
		cephv1.MonSpec{Count: op.mons, AllowMultiplePerNode: true}, false /* startWithAllNodes */, op.rbdMirrorWorkers, op.skipOSDCreation, op.rookVersion)

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

// Teardown is a wrapper for tearDown after Suite
func (op *TestCluster) Teardown() {
	op.installer.UninstallRook(op.namespace)
}
