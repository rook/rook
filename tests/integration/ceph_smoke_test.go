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

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	mons := 3
	rbdMirrorWorkers := 1
	suite.op, suite.k8sh = StartTestCluster(suite.T, smokeSuiteMinimalTestVersion, suite.namespace, "bluestore", false, false, "", mons, rbdMirrorWorkers, installer.VersionMaster, installer.NautilusVersion)
	suite.helper = clients.CreateTestClient(suite.k8sh, suite.op.installer.Manifests)
}

func (suite *SmokeSuite) AfterTest(suiteName, testName string) {
	suite.op.installer.CollectOperatorLog(suiteName, testName, installer.SystemNamespace(suite.namespace))
}

func (suite *SmokeSuite) TearDownSuite() {
	suite.op.Teardown()
}

func (suite *SmokeSuite) TestBlockStorage_SmokeTest() {
	runBlockCSITest(suite.helper, suite.k8sh, suite.Suite, suite.namespace)
}

func (suite *SmokeSuite) TestFileStorage_SmokeTest() {
	useCSI := true
	runFileE2ETest(suite.helper, suite.k8sh, suite.Suite, suite.namespace, "smoke-test-fs", useCSI)
}

func (suite *SmokeSuite) TestObjectStorage_SmokeTest() {
	runObjectE2ETest(suite.helper, suite.k8sh, suite.Suite, suite.namespace)
}

// Test to make sure all rook components are installed and Running
func (suite *SmokeSuite) TestARookClusterInstallation_SmokeTest() {
	checkIfRookClusterIsInstalled(suite.Suite, suite.k8sh, installer.SystemNamespace(suite.namespace), suite.namespace, 3)
}

// Smoke Test for Mon failover - Test check the following operations for the Mon failover in order
// Delete mon pod, Wait for new mon pod
func (suite *SmokeSuite) TestMonFailover() {
	logger.Infof("Mon Failover Smoke Test")

	deployments, err := suite.getNonCanaryMonDeployments()
	require.Nil(suite.T(), err)
	require.Equal(suite.T(), 3, len(deployments))

	monToKill := deployments[0].Name
	logger.Infof("Killing mon %s", monToKill)
	propagation := metav1.DeletePropagationForeground
	delOptions := &metav1.DeleteOptions{PropagationPolicy: &propagation}
	err = suite.k8sh.Clientset.AppsV1().Deployments(suite.namespace).Delete(monToKill, delOptions)
	require.Nil(suite.T(), err)

	// Wait for the health check to start a new monitor
	originalMonDeleted := false
	for i := 0; i < 30; i++ {
		deployments, err := suite.getNonCanaryMonDeployments()
		require.Nil(suite.T(), err)

		// Make sure the old mon is not still alive
		foundOldMon := false
		for _, mon := range deployments {
			if mon.Name == monToKill {
				foundOldMon = true
			}
		}

		// Check if we have three monitors
		if foundOldMon {
			if originalMonDeleted {
				// Depending on the state of the orchestration, the operator might trigger
				// re-creation of the deleted mon. In this case, consider the test successful
				// rather than wait for the failover which will never occur.
				logger.Infof("Original mon created again, no need to wait for mon failover")
				return
			}
			logger.Infof("Waiting for old monitor to stop")
		} else {
			logger.Infof("Waiting for a new monitor to start")
			originalMonDeleted = true
			if len(deployments) == 3 {
				var newMons []string
				for _, mon := range deployments {
					newMons = append(newMons, mon.Name)
				}
				logger.Infof("Found a new monitor! monitors=%v", newMons)
				return
			}

			assert.Equal(suite.T(), 2, len(deployments))
		}

		time.Sleep(5 * time.Second)
	}

	require.Fail(suite.T(), "giving up waiting for a new monitor")
}

// Smoke Test for pool Resizing
func (suite *SmokeSuite) TestPoolResize() {
	logger.Infof("Pool Resize Smoke Test")

	poolName := "testpool"
	err := suite.helper.PoolClient.Create(poolName, suite.namespace, 1)
	require.Nil(suite.T(), err)

	poolFound := false

	// Wait for pool to appear
	for i := 0; i < 10; i++ {
		pools, err := suite.helper.PoolClient.ListCephPools(suite.namespace)
		require.Nil(suite.T(), err)
		for _, p := range pools {
			if p.Name != poolName {
				continue
			}
			poolFound = true
		}
		if poolFound {
			break
		}

		logger.Infof("Waiting for pool to appear")
		time.Sleep(2 * time.Second)
	}

	require.Equal(suite.T(), true, poolFound, "pool not found")

	err = suite.helper.PoolClient.Update(poolName, suite.namespace, 3)
	require.Nil(suite.T(), err)

	poolFound = false
	// Wait for pool resize to happen
	for i := 0; i < 10; i++ {

		details, err := suite.helper.PoolClient.GetCephPoolDetails(suite.namespace, poolName)
		require.Nil(suite.T(), err)
		if details.Size > 1 {
			logger.Infof("pool %s size got updated", poolName)
			require.Equal(suite.T(), 3, int(details.Size))
			poolFound = true
			break
		}
		logger.Infof("pool %s size not updated yet. details: %+v", poolName, details)

		logger.Infof("Waiting for pool %s resize to happen", poolName)
		time.Sleep(2 * time.Second)
	}

	require.Equal(suite.T(), true, poolFound, fmt.Sprintf("pool %s not found", poolName))
}

// Smoke Test for Client CRD
func (suite *SmokeSuite) TestCreateClient() {
	logger.Infof("Create Client Smoke Test")

	clientName := "client1"
	caps := map[string]string{
		"mon": "allow rwx",
		"mgr": "allow rwx",
		"osd": "allow rwx",
	}
	err := suite.helper.UserClient.Create(clientName, suite.namespace, caps)
	require.Nil(suite.T(), err)

	clientFound := false

	for i := 0; i < 30; i++ {
		clients, _ := suite.helper.UserClient.Get(suite.namespace, "client."+clientName)
		if clients != "" {
			clientFound = true
		}

		if clientFound {
			break
		}

		logger.Infof("Waiting for client to appear")
		time.Sleep(2 * time.Second)
	}

	require.Equal(suite.T(), true, clientFound, "client not found")

	logger.Infof("Update Client Smoke Test")
	newcaps := map[string]string{
		"mon": "allow r",
		"mgr": "allow rw",
		"osd": "allow *",
	}
	caps, _ = suite.helper.UserClient.Update(suite.namespace, clientName, newcaps)

	require.Equal(suite.T(), "allow r", caps["mon"], "wrong caps")
	require.Equal(suite.T(), "allow rw", caps["mgr"], "wrong caps")
	require.Equal(suite.T(), "allow *", caps["osd"], "wrong caps")
}

func (suite *SmokeSuite) getNonCanaryMonDeployments() ([]appsv1.Deployment, error) {
	opts := metav1.ListOptions{LabelSelector: "app=rook-ceph-mon"}
	deployments, err := suite.k8sh.Clientset.AppsV1().Deployments(suite.namespace).List(opts)
	if err != nil {
		return nil, err
	}
	nonCanaryMonDeployments := []appsv1.Deployment{}
	for _, deployment := range deployments.Items {
		if !strings.HasSuffix(deployment.GetName(), "-canary") {
			nonCanaryMonDeployments = append(nonCanaryMonDeployments, deployment)
		}
	}
	return nonCanaryMonDeployments, nil
}
