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

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ***************************************************
// *** Major scenarios tested by the TestHelmSuite ***
// Setup
// - A cluster created via the Helm chart
// Monitors
// - One mon
// OSDs
// - Bluestore running on a raw block device
// Block
// - Create a pool in the cluster
// - Mount/unmount a block device through the dynamic provisioner
// File system
// - Create a file system via the CRD
// Object
// - Create the object store via the CRD
// ***************************************************
func TestCephHelmSuite(t *testing.T) {
	if installer.SkipTestSuite(installer.CephTestSuite) {
		t.Skip()
	}

	s := new(HelmSuite)
	defer func(s *HelmSuite) {
		HandlePanics(recover(), s.op, s.T)
	}(s)
	suite.Run(t, s)
}

type HelmSuite struct {
	suite.Suite
	helper            *clients.TestClient
	kh                *utils.K8sHelper
	op                *TestCluster
	operatorNamespace string
	clusterNamespaces []string
	poolName          string
	rookCephCleanup   bool
}

func (hs *HelmSuite) SetupSuite() {
	hs.operatorNamespace = "helm-ns"
	hs.poolName = "multi-helm-cluster-pool1"
	hs.namespace1 = "cluster-ns1"
	hs.namespace2 = "cluster-ns2"

	hs.op, hs.kh = NewMCTestOperations(hs.T, installer.SystemNamespace(hs.operatorNamespace), hs.namespace1, hs.namespace2, true, false)
	hs.testClient = clients.CreateTestClient(hs.kh, hs.op.installer.Manifests)
	hs.createPools()
}

func (hs *HelmSuite) createPools() {
	// create a test pool in each cluster so that we get some PGs
	logger.Infof("Creating pool %s", hs.poolName)
	err := hs.testClient.PoolClient.Create(hs.poolName, hs.namespace1, 1)
	require.Nil(hs.T(), err)
}

func (hs *HelmSuite) TearDownSuite() {
	hs.deletePools()
	hs.op.Teardown()
}

func (hs *HelmSuite) deletePools() {
	// create a test pool in each cluster so that we get some PGs
	logger.Infof("Deleting pool %s", hs.poolName)
	clusterInfo := client.AdminClusterInfo(hs.namespace1)
	if err := hs.testClient.PoolClient.DeletePool(hs.testClient.BlockClient, clusterInfo, hs.poolName); err != nil {
		logger.Errorf("failed to delete pool %q. %v", hs.poolName, err)
	} else {
		logger.Infof("deleted pool %q", hs.poolName)
	}
}

func (hs *HelmSuite) AfterTest(suiteName, testName string) {
	hs.op.installer.CollectOperatorLog(suiteName, testName, installer.SystemNamespace(hs.operatorNamespace))
}

// Test to make sure all rook components are installed and Running
func (hs *HelmSuite) TestARookInstallViaHelm() {
	checkIfRookClusterIsInstalled(hs.Suite, hs.kh, hs.operatorNamespace, hs.clusterNamespaces, 1)
}

// Test BlockCreation on Rook that was installed via Helm
func (hs *HelmSuite) TestBlockStoreOnRookInstalledViaHelm() {
	runBlockCSITestLite(hs.helper, hs.kh, hs.Suite, hs.clusterNamespaces, hs.operatorNamespace, hs.op.installer.CephVersion)
}

// Test File System Creation on Rook that was installed via helm
func (hs *HelmSuite) TestFileStoreOnRookInstalledViaHelm() {
	runFileE2ETestLite(hs.helper, hs.kh, hs.Suite, hs.clusterNamespaces, "testfs")
}

// Test Object StoreCreation on Rook that was installed via helm
func (hs *HelmSuite) TestObjectStoreOnRookInstalledViaHelm() {
	runObjectE2ETestLite(hs.helper, hs.kh, hs.Suite, hs.clusterNamespaces, "default", 3, true)
}
