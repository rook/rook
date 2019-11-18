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
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// *************************************************************
// *** Major scenarios tested by the MultiClusterDeploySuite ***
// Setup
// - Two clusters started in different namespaces via the CRD
// Monitors
// - One mon in each cluster
// OSDs
// - Bluestore running on a directory
// Block
// - Create a pool in each cluster
// - Mount/unmount a block device through the dynamic provisioner
// File system
// - Create a file system via the CRD
// Object
// - Create the object store via the CRD
// *************************************************************
func TestCephMultiClusterDeploySuite(t *testing.T) {
	if installer.SkipTestSuite(installer.CephTestSuite) {
		t.Skip()
	}

	s := new(MultiClusterDeploySuite)
	defer func(s *MultiClusterDeploySuite) {
		HandlePanics(recover(), s.op, s.T)
	}(s)
	suite.Run(t, s)
}

type MultiClusterDeploySuite struct {
	suite.Suite
	testClient *clients.TestClient
	k8sh       *utils.K8sHelper
	namespace1 string
	namespace2 string
	op         *MCTestOperations
}

// Deploy Multiple Rook clusters
func (mrc *MultiClusterDeploySuite) SetupSuite() {

	mrc.namespace1 = "mrc-n1"
	mrc.namespace2 = "mrc-n2"

	mrc.op, mrc.k8sh = NewMCTestOperations(mrc.T, mrc.namespace1, mrc.namespace2)
	mrc.testClient = clients.CreateTestClient(mrc.k8sh, mrc.op.installer.Manifests)
	mrc.createPools()
}

func (mrc *MultiClusterDeploySuite) AfterTest(suiteName, testName string) {
	mrc.op.installer.CollectOperatorLog(suiteName, testName, mrc.op.systemNamespace)
}

func (mrc *MultiClusterDeploySuite) createPools() {
	// create a test pool in each cluster so that we get some PGs
	poolName := "multi-cluster-pool1"
	logger.Infof("Creating pool %s", poolName)
	err := mrc.testClient.PoolClient.Create(poolName, mrc.namespace1, 1)
	require.Nil(mrc.T(), err)

	poolName = "multi-cluster-pool2"
	logger.Infof("Creating pool %s", poolName)
	err = mrc.testClient.PoolClient.Create(poolName, mrc.namespace2, 1)
	require.Nil(mrc.T(), err)
}

func (mrc *MultiClusterDeploySuite) TearDownSuite() {
	mrc.op.Teardown()
}

// Test to make sure all rook components are installed and Running
func (mrc *MultiClusterDeploySuite) TestInstallingMultipleRookClusters() {
	// Check if Rook cluster 1 is deployed successfully
	checkIfRookClusterIsInstalled(mrc.Suite, mrc.k8sh, installer.SystemNamespace(mrc.namespace1), mrc.namespace1, 1)
	checkIfRookClusterIsHealthy(mrc.Suite, mrc.testClient, mrc.namespace1)

	// Check if Rook cluster 2 is deployed successfully
	checkIfRookClusterIsInstalled(mrc.Suite, mrc.k8sh, installer.SystemNamespace(mrc.namespace1), mrc.namespace2, 1)
	checkIfRookClusterIsHealthy(mrc.Suite, mrc.testClient, mrc.namespace2)
}

// Test Block Store Creation on multiple rook clusters
func (mrc *MultiClusterDeploySuite) TestBlockStoreOnMultipleRookCluster() {
	runBlockE2ETestLite(mrc.testClient, mrc.k8sh, mrc.Suite, mrc.namespace1, mrc.op.installer.CephVersion)
	runBlockE2ETestLite(mrc.testClient, mrc.k8sh, mrc.Suite, mrc.namespace2, mrc.op.installer.CephVersion)
}

// Test Filesystem Creation on multiple rook clusters
func (mrc *MultiClusterDeploySuite) TestFileStoreOnMultiRookCluster() {
	runFileE2ETestLite(mrc.testClient, mrc.k8sh, mrc.Suite, mrc.namespace1, "test-fs-1")
	runFileE2ETestLite(mrc.testClient, mrc.k8sh, mrc.Suite, mrc.namespace2, "test-fs-2")
}

// Test Object Store Creation on multiple rook clusters
func (mrc *MultiClusterDeploySuite) TestObjectStoreOnMultiRookCluster() {
	runObjectE2ETestLite(mrc.testClient, mrc.k8sh, mrc.Suite, mrc.namespace1, "default-c1", 2)
	runObjectE2ETestLite(mrc.testClient, mrc.k8sh, mrc.Suite, mrc.namespace2, "default-c2", 1)
}

// MCTestOperations struct for handling panic and test suite tear down
type MCTestOperations struct {
	installer       *installer.CephInstaller
	kh              *utils.K8sHelper
	T               func() *testing.T
	namespace1      string
	namespace2      string
	systemNamespace string
}

// NewMCTestOperations creates new instance of TestCluster struct
func NewMCTestOperations(t func() *testing.T, namespace1 string, namespace2 string) (*MCTestOperations, *utils.K8sHelper) {

	kh, err := utils.CreateK8sHelper(t)
	require.NoError(t(), err)
	checkIfShouldRunForMinimalTestMatrix(t, kh, multiClusterMinimalTestVersion)

	i := installer.NewCephInstaller(t, kh.Clientset, false, installer.VersionMaster, installer.NautilusVersion)

	op := &MCTestOperations{i, kh, t, namespace1, namespace2, installer.SystemNamespace(namespace1)}
	op.Setup()
	return op, kh
}

// SetUpRook is wrapper for setting up multiple rook clusters.
func (o MCTestOperations) Setup() {
	var err error
	err = o.installer.CreateCephOperator(installer.SystemNamespace(o.namespace1))
	require.NoError(o.T(), err)

	require.True(o.T(), o.kh.IsPodInExpectedState("rook-ceph-operator", o.systemNamespace, "Running"),
		"Make sure rook-operator is in running state")

	require.True(o.T(), o.kh.IsPodInExpectedState("rook-discover", o.systemNamespace, "Running"),
		"Make sure rook-discover is in running state")

	time.Sleep(10 * time.Second)

	// start the two clusters in parallel
	logger.Infof("starting two clusters in parallel")
	err = o.startCluster(o.namespace1, "bluestore")
	require.NoError(o.T(), err)
	err = o.startCluster(o.namespace2, "filestore")
	require.NoError(o.T(), err)

	require.True(o.T(), o.kh.IsPodInExpectedState("rook-ceph-agent", o.systemNamespace, "Running"),
		"Make sure rook-ceph-agent is in running state")

	logger.Infof("finished starting clusters")
}

// TearDownRook is a wrapper for tearDown after suite
func (o MCTestOperations) Teardown() {
	defer func() {
		if r := recover(); r != nil {
			logger.Infof("Unexpected Errors while cleaning up MultiCluster test --> %v", r)
			o.T().FailNow()
		}
	}()

	o.installer.UninstallRookFromMultipleNS(true, installer.SystemNamespace(o.namespace1), o.namespace1, o.namespace2)
}

func (o MCTestOperations) startCluster(namespace, store string) error {
	logger.Infof("starting cluster %s", namespace)
	useDevices := false
	// do not use disks for this cluster, otherwise the 2 test clusters will each try to use the
	// same disks.
	err := o.installer.CreateK8sRookClusterWithHostPathAndDevices(namespace, o.systemNamespace, store,
		useDevices, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, true, 1, installer.NautilusVersion)
	if err != nil {
		o.T().Fail()
		o.installer.GatherAllRookLogs(o.T().Name(), namespace, o.systemNamespace)
		return fmt.Errorf("failed to create cluster %s. %+v", namespace, err)
	}

	if err := o.installer.CreateK8sRookToolbox(namespace); err != nil {
		o.T().Fail()
		o.installer.GatherAllRookLogs(o.T().Name(), namespace, o.systemNamespace)
		return fmt.Errorf("failed to create toolbox for %s. %+v", namespace, err)
	}
	logger.Infof("succeeded starting cluster %s", namespace)
	return nil
}
