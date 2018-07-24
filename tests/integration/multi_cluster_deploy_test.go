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

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
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
func TestMultiClusterDeploySuite(t *testing.T) {
	s := new(MultiClusterDeploySuite)
	defer func(s *MultiClusterDeploySuite) {
		HandlePanics(recover(), s.op, s.T)
	}(s)
	suite.Run(t, s)
}

type MultiClusterDeploySuite struct {
	suite.Suite
	helper1    *clients.TestClient
	helper2    *clients.TestClient
	k8sh       *utils.K8sHelper
	namespace1 string
	namespace2 string
	op         contracts.Setup
}

//Deploy Multiple Rook clusters
func (mrc *MultiClusterDeploySuite) SetupSuite() {

	mrc.namespace1 = "mrc-n1"
	mrc.namespace2 = "mrc-n2"

	mrc.op, mrc.k8sh = NewMCTestOperations(mrc.T, mrc.namespace1, mrc.namespace2)
	mrc.helper1 = GetTestClient(mrc.k8sh, mrc.namespace1, mrc.op, mrc.T)
	mrc.helper2 = GetTestClient(mrc.k8sh, mrc.namespace2, mrc.op, mrc.T)
	mrc.createPools()

}
func (mrc *MultiClusterDeploySuite) createPools() {
	// create a test pool in each cluster so that we get some PGs
	poolName := "multi-cluster-pool1"
	logger.Infof("Creating pool %s", poolName)
	result, err := installer.BlockResourceOperation(mrc.k8sh, installer.GetBlockPoolDef(poolName, mrc.namespace1, "1"), "create")
	assert.Contains(mrc.T(), result, fmt.Sprintf("\"%s\" created", poolName))
	require.Nil(mrc.T(), err)

	poolName = "multi-cluster-pool2"
	logger.Infof("Creating pool %s", poolName)
	result, err = installer.BlockResourceOperation(mrc.k8sh, installer.GetBlockPoolDef(poolName, mrc.namespace2, "1"), "create")
	assert.Contains(mrc.T(), result, fmt.Sprintf("\"%s\" created", poolName))
	require.Nil(mrc.T(), err)
}

func (mrc *MultiClusterDeploySuite) TearDownSuite() {
	mrc.op.TearDown()
}

//Test to make sure all rook components are installed and Running
func (mrc *MultiClusterDeploySuite) TestInstallingMultipleRookClusters() {
	//Check if Rook cluster 1 is deployed successfully
	checkIfRookClusterIsInstalled(mrc.Suite, mrc.k8sh, installer.SystemNamespace(mrc.namespace1), mrc.namespace1, 1)
	checkIfRookClusterIsHealthy(mrc.Suite, mrc.helper1, mrc.namespace1)

	//Check if Rook cluster 2 is deployed successfully
	checkIfRookClusterIsInstalled(mrc.Suite, mrc.k8sh, installer.SystemNamespace(mrc.namespace1), mrc.namespace2, 1)
	checkIfRookClusterIsHealthy(mrc.Suite, mrc.helper2, mrc.namespace2)
}

//Test Block Store Creation on multiple rook clusters
func (mrc *MultiClusterDeploySuite) TestBlockStoreOnMultipleRookCluster() {
	runBlockE2ETestLite(mrc.helper1, mrc.k8sh, mrc.Suite, mrc.namespace1)
	runBlockE2ETestLite(mrc.helper2, mrc.k8sh, mrc.Suite, mrc.namespace2)
}

//Test Filesystem Creation on multiple rook clusters
func (mrc *MultiClusterDeploySuite) TestFileStoreOnMultiRookCluster() {
	runFileE2ETestLite(mrc.helper1, mrc.k8sh, mrc.Suite, mrc.namespace1, "test-fs-1")
	runFileE2ETestLite(mrc.helper2, mrc.k8sh, mrc.Suite, mrc.namespace2, "test-fs-2")
}

//Test Object Store Creation on multiple rook clusters
func (mrc *MultiClusterDeploySuite) TestObjectStoreOnMultiRookCluster() {
	runObjectE2ETestLite(mrc.helper1, mrc.k8sh, mrc.Suite, mrc.namespace1, "default-c1", 2)
	runObjectE2ETestLite(mrc.helper2, mrc.k8sh, mrc.Suite, mrc.namespace2, "default-c2", 1)
}

//MCTestOperations struct for handling panic and test suite tear down
type MCTestOperations struct {
	installer       *installer.InstallHelper
	installData     *installer.InstallData
	kh              *utils.K8sHelper
	T               func() *testing.T
	namespace1      string
	namespace2      string
	systemNamespace string
	helper1         *clients.TestClient
	helper2         *clients.TestClient
}

//NewMCTestOperations creates new instance of BaseTestOperations struct
func NewMCTestOperations(t func() *testing.T, namespace1 string, namespace2 string) (MCTestOperations, *utils.K8sHelper) {

	kh, err := utils.CreateK8sHelper(t)
	require.NoError(t(), err)
	i := installer.NewK8sRookhelper(kh.Clientset, t)
	d := installer.NewK8sInstallData()

	op := MCTestOperations{i, d, kh, t, namespace1, namespace2, installer.SystemNamespace(namespace1), nil, nil}
	op.SetUp()
	return op, kh
}

//SetUpRook is wrapper for setting up multiple rook clusters.
func (o MCTestOperations) SetUp() {
	var err error
	err = o.installer.CreateK8sRookOperator(installer.SystemNamespace(o.namespace1))
	require.NoError(o.T(), err)

	require.True(o.T(), o.kh.IsPodInExpectedState("rook-ceph-operator", o.systemNamespace, "Running"),
		"Make sure rook-operator is in running state")

	require.True(o.T(), o.kh.IsPodInExpectedState("rook-ceph-agent", o.systemNamespace, "Running"),
		"Make sure rook-ceph-agent is in running state")

	require.True(o.T(), o.kh.IsPodInExpectedState("rook-discover", o.systemNamespace, "Running"),
		"Make sure rook-discover is in running state")

	time.Sleep(10 * time.Second)

	// start the two clusters in parallel
	logger.Infof("starting two clusters in parallel")
	errCh1 := make(chan error, 1)
	errCh2 := make(chan error, 1)
	go o.startCluster(o.namespace1, "bluestore", errCh1)
	go o.startCluster(o.namespace2, "filestore", errCh2)
	require.NoError(o.T(), <-errCh1)
	require.NoError(o.T(), <-errCh2)
	logger.Infof("finished starting clusters")

}

//TearDownRook is a wrapper for tearDown after suite
func (o MCTestOperations) TearDown() {
	defer func() {
		if r := recover(); r != nil {
			logger.Infof("Unexpected Errors while cleaning up MultiCluster test --> %v", r)
			o.T().FailNow()
		}
	}()
	if o.T().Failed() {
		o.installer.GatherAllRookLogs(o.namespace1, o.systemNamespace, o.T().Name())
		o.installer.GatherAllRookLogs(o.namespace2, o.systemNamespace, o.T().Name())
	}

	o.installer.UninstallRookFromMultipleNS(false, installer.SystemNamespace(o.namespace1), o.namespace1, o.namespace2)
}

func (o MCTestOperations) startCluster(namespace, store string, errCh chan error) {
	logger.Infof("starting cluster %s", namespace)
	if err := o.installer.CreateK8sRookCluster(namespace, o.systemNamespace, store); err != nil {
		o.installer.GatherAllRookLogs(namespace, o.systemNamespace, o.T().Name())
		errCh <- fmt.Errorf("failed to create cluster %s. %+v", namespace, err)
		return
	}

	if err := o.installer.CreateK8sRookToolbox(namespace); err != nil {
		o.installer.GatherAllRookLogs(namespace, o.systemNamespace, o.T().Name())
		errCh <- fmt.Errorf("failed to create toolbox for %s. %+v", namespace, err)
		return
	}
	logger.Infof("succeeded starting cluster %s", namespace)
	errCh <- nil
}
