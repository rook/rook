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
	"path/filepath"
	"testing"

	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	localPathPVCmd = "tests/scripts/localPathPV.sh"
)

// *************************************************************
// *** Major scenarios tested by the MultiClusterDeploySuite ***
// Setup
// - Two clusters started in different namespaces via the CRD
// Monitors
// - One mon in each cluster
// OSDs
// - Bluestore running on a raw block device
// Block
// - Create a pool in each cluster
// - Mount/unmount a block device through the dynamic provisioner
// File system
// - Create a file system via the CRD
// Object
// - Create the object store via the CRD
// *************************************************************
func TestCephMultiClusterDeploySuite(t *testing.T) {
	s := new(MultiClusterDeploySuite)
	defer func(s *MultiClusterDeploySuite) {
		HandlePanics(recover(), s.TearDownSuite, s.T)
	}(s)
	suite.Run(t, s)
}

type MultiClusterDeploySuite struct {
	suite.Suite
	testClient        *clients.TestClient
	k8sh              *utils.K8sHelper
	settings          *installer.TestCephSettings
	externalManifests installer.CephManifests
	installer         *installer.CephInstaller
	coreToolbox       string
	externalToolbox   string
	poolName          string
}

// Deploy Multiple Rook clusters
func (s *MultiClusterDeploySuite) SetupSuite() {
	s.poolName = "multi-cluster-pool1"
	coreNamespace := "multi-core"
	s.settings = &installer.TestCephSettings{
		ClusterName:        "multi-cluster",
		Namespace:          coreNamespace,
		OperatorNamespace:  installer.SystemNamespace(coreNamespace),
		StorageClassName:   "manual",
		UsePVC:             installer.UsePVC(),
		Mons:               1,
		MultipleMgrs:       true,
		RookVersion:        installer.LocalBuildTag,
		CephVersion:        installer.SquidVersion,
		RequireMsgr2:       true,
		ClusterConcurrency: 2,
	}
	s.settings.ApplyEnvVars()
	externalSettings := &installer.TestCephSettings{
		IsExternal:        true,
		ClusterName:       "test-external",
		Namespace:         "multi-external",
		OperatorNamespace: s.settings.OperatorNamespace,
		RookVersion:       s.settings.RookVersion,
		CephVersion:       installer.SquidVersion,
	}
	externalSettings.ApplyEnvVars()
	s.externalManifests = installer.NewCephManifests(externalSettings)

	// Start the core storage cluster
	s.setupMultiClusterCore()
	s.createPools()

	// Start the external cluster that will connect to the core cluster
	// create an external cluster
	s.startExternalCluster()

	logger.Infof("finished starting clusters")
}

func (s *MultiClusterDeploySuite) AfterTest(suiteName, testName string) {
	s.installer.CollectOperatorLog(suiteName, testName)
}

func (s *MultiClusterDeploySuite) createPools() {
	// create a test pool in each cluster so that we get some PGs
	logger.Infof("Creating pool %s", s.poolName)
	err := s.testClient.PoolClient.Create(s.poolName, s.settings.Namespace, 1)
	require.Nil(s.T(), err)
}

func (s *MultiClusterDeploySuite) deletePools() {
	// create a test pool in each cluster so that we get some PGs
	clusterInfo := client.AdminTestClusterInfo(s.settings.Namespace)
	if err := s.testClient.PoolClient.DeletePool(s.testClient.BlockClient, clusterInfo, s.poolName); err != nil {
		logger.Errorf("failed to delete pool %q. %v", s.poolName, err)
	} else {
		logger.Infof("deleted pool %q", s.poolName)
	}
}

func (s *MultiClusterDeploySuite) TearDownSuite() {
	s.deletePools()
	s.installer.UninstallRookFromMultipleNS(s.externalManifests, s.installer.Manifests)
}

// Test to make sure all rook components are installed and Running
func (s *MultiClusterDeploySuite) TestInstallingMultipleRookClusters() {
	// Check if Rook cluster 1 is deployed successfully
	client.RunAllCephCommandsInToolboxPod = s.coreToolbox
	checkIfRookClusterIsInstalled(&s.Suite, s.k8sh, s.settings.OperatorNamespace, s.settings.Namespace, 1)
	checkIfRookClusterIsHealthy(&s.Suite, s.testClient, s.settings.Namespace)

	// Check if Rook external cluster is deployed successfully
	// Checking health status is enough to validate the connection
	client.RunAllCephCommandsInToolboxPod = s.externalToolbox
	checkIfRookClusterIsHealthy(&s.Suite, s.testClient, s.externalManifests.Settings().Namespace)
}

// Setup is wrapper for setting up multiple rook clusters.
func (s *MultiClusterDeploySuite) setupMultiClusterCore() {
	root, err := utils.FindRookRoot()
	require.NoError(s.T(), err, "failed to get rook root")
	cmdArgs := utils.CommandArgs{
		Command: filepath.Join(root, localPathPVCmd),
		CmdArgs: []string{installer.TestScratchDevice()},
	}
	cmdOut := utils.ExecuteCommand(cmdArgs)
	require.NoError(s.T(), cmdOut.Err)

	s.installer, s.k8sh = StartTestCluster(s.T, s.settings)
	s.testClient = clients.CreateTestClient(s.k8sh, s.installer.Manifests)
	s.coreToolbox = client.RunAllCephCommandsInToolboxPod
}

func (s *MultiClusterDeploySuite) startExternalCluster() {
	err := s.installer.CreateRookExternalCluster(s.externalManifests)
	if err != nil {
		s.T().Fail()
		s.installer.GatherAllRookLogs(s.T().Name(), s.externalManifests.Settings().Namespace)
		require.NoError(s.T(), err)
	}

	s.externalToolbox = client.RunAllCephCommandsInToolboxPod
	logger.Infof("succeeded starting external cluster %s", s.externalManifests.Settings().Namespace)
}
