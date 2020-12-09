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
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rook/rook/pkg/daemon/ceph/client"
	oppool "github.com/rook/rook/pkg/operator/ceph/pool"
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
// - Quota limit wrt no of objects
// ************************************************
func TestCephSmokeSuite(t *testing.T) {
	if installer.SkipTestSuite(installer.CephTestSuite) {
		t.Skip()
	}

	// Skip the suite if CSI is not supported
	kh, err := utils.CreateK8sHelper(func() *testing.T { return t })
	require.NoError(t, err)
	checkSkipCSITest(t, kh)

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
	smokeTestCluster := TestCluster{
		namespace:               suite.namespace,
		storeType:               "bluestore",
		storageClassName:        installer.StorageClassName(),
		useHelm:                 false,
		usePVC:                  installer.UsePVC(),
		mons:                    3,
		rbdMirrorWorkers:        1,
		rookCephCleanup:         true,
		skipOSDCreation:         false,
		minimalMatrixK8sVersion: smokeSuiteMinimalTestVersion,
		rookVersion:             installer.VersionMaster,
		cephVersion:             installer.OctopusVersion,
	}

	suite.op, suite.k8sh = StartTestCluster(suite.T, &smokeTestCluster)
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
	preserveFilesystemOnDelete := true
	runFileE2ETest(suite.helper, suite.k8sh, suite.Suite, suite.namespace, "smoke-test-fs", useCSI, preserveFilesystemOnDelete)
}

func (suite *SmokeSuite) TestObjectStorage_SmokeTest() {
	if !utils.IsPlatformOpenShift() {
		runObjectE2ETest(suite.helper, suite.k8sh, suite.Suite, suite.namespace)
	}
}

// Test to make sure all rook components are installed and Running
func (suite *SmokeSuite) TestARookClusterInstallation_SmokeTest() {
	checkIfRookClusterIsInstalled(suite.Suite, suite.k8sh, installer.SystemNamespace(suite.namespace), suite.namespace, 3)
}

// Smoke Test for Mon failover - Test check the following operations for the Mon failover in order
// Delete mon pod, Wait for new mon pod
func (suite *SmokeSuite) TestMonFailover() {
	ctx := context.TODO()
	logger.Infof("Mon Failover Smoke Test")

	deployments, err := suite.getNonCanaryMonDeployments()
	require.Nil(suite.T(), err)
	require.Equal(suite.T(), 3, len(deployments))

	monToKill := deployments[0].Name
	logger.Infof("Killing mon %s", monToKill)
	propagation := metav1.DeletePropagationForeground
	delOptions := &metav1.DeleteOptions{PropagationPolicy: &propagation}
	err = suite.k8sh.Clientset.AppsV1().Deployments(suite.namespace).Delete(ctx, monToKill, *delOptions)
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
	ctx := context.TODO()
	logger.Infof("Pool Resize Smoke Test")

	poolName := "testpool"
	err := suite.helper.PoolClient.Create(poolName, suite.namespace, 1)
	require.Nil(suite.T(), err)

	poolFound := false
	clusterInfo := client.AdminClusterInfo(suite.namespace)

	// Wait for pool to appear
	for i := 0; i < 10; i++ {
		pools, err := suite.helper.PoolClient.ListCephPools(clusterInfo)
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

	err = suite.helper.PoolClient.Update(poolName, suite.namespace, 2)
	require.Nil(suite.T(), err)

	poolResized := false
	// Wait for pool resize to happen
	for i := 0; i < 10; i++ {
		details, err := suite.helper.PoolClient.GetCephPoolDetails(clusterInfo, poolName)
		require.Nil(suite.T(), err)
		if details.Size > 1 {
			logger.Infof("pool %s size was updated", poolName)
			require.Equal(suite.T(), 2, int(details.Size))
			poolResized = true

			// resize the pool back to 1 to avoid hangs around not having enough OSDs to satisfy rbd
			err = suite.helper.PoolClient.Update(poolName, suite.namespace, 1)
			require.Nil(suite.T(), err)
		} else if poolResized && details.Size == 1 {
			logger.Infof("pool resized back to 1")
			break
		}

		logger.Debugf("pool %s size not updated yet. details: %+v", poolName, details)
		logger.Infof("Waiting for pool %s resize to happen", poolName)
		time.Sleep(2 * time.Second)
	}

	require.Equal(suite.T(), true, poolResized, fmt.Sprintf("pool %s not found", poolName))

	// Verify the Kubernetes Secret has been created (bootstrap peer token)
	pool, err := suite.k8sh.RookClientset.CephV1().CephBlockPools(suite.namespace).Get(ctx, poolName, metav1.GetOptions{})
	assert.NoError(suite.T(), err)
	if pool.Spec.Mirroring.Enabled {
		secretName := pool.Status.Info[oppool.RBDMirrorBootstrapPeerSecretName]
		assert.NotEmpty(suite.T(), secretName)
		// now fetch the secret which contains the bootstrap peer token
		s, err := suite.k8sh.Clientset.CoreV1().Secrets(suite.namespace).Get(ctx, secretName, metav1.GetOptions{})
		require.Nil(suite.T(), err)
		assert.NotEmpty(suite.T(), s.Data["token"])

		// Once we have a scenario with another Ceph cluster - needs to be added in the MultiCluster suite
		// We would need to add a bootstrap peer token following the below procedure
		// bootstrapSecretName := "bootstrap-peer-token"
		// token := "eyJmc2lkIjoiYzZiMDg3ZjItNzgyOS00ZGJiLWJjZmMtNTNkYzM0ZTBiMzVkIiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFBV1lsWmZVQ1Q2RGhBQVBtVnAwbGtubDA5YVZWS3lyRVV1NEE9PSIsIm1vbl9ob3N0IjoiW3YyOjE5Mi4xNjguMTExLjEwOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTA6Njc4OV0sW3YyOjE5Mi4xNjguMTExLjEyOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTI6Njc4OV0sW3YyOjE5Mi4xNjguMTExLjExOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTE6Njc4OV0ifQ=="
		// s = oppool.GenerateBootstrapPeerSecret(bootstrapSecretName, suite.namespace, string(pool.GetUID()), []byte(token))
		// s, err = suite.k8sh.Clientset.CoreV1().Secrets(suite.namespace).Create(s)
		// require.Nil(suite.T(), err, err.Error())

		// // update the ceph block pool cr
		// pool.Spec.Mirrored.PeersSecretNames = append(pool.Spec.Mirrored.PeersSecretNames, bootstrapSecretName)
		// _, err = suite.k8sh.RookClientset.CephV1().CephBlockPools(suite.namespace).Update(pool)
		// require.Nil(suite.T(), err, err.Error())

		// mirrorInfo, err := client.PrintPoolMirroringInfo(suite.k8sh.MakeContext(), clusterInfo, poolName)
		// require.Nil(suite.T(), err, err.Error())
		// assert.Equal(suite.T(), "image", mirrorInfo.Mode)
		// assert.Equal(suite.T(), 1, len(mirrorInfo.Peers))
	}

	// clean up the pool
	err = suite.helper.PoolClient.DeletePool(suite.helper.BlockClient, clusterInfo, poolName)
	assert.NoError(suite.T(), err)
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
	clusterInfo := client.AdminClusterInfo(suite.namespace)
	err := suite.helper.UserClient.Create(clientName, suite.namespace, caps)
	require.Nil(suite.T(), err)

	clientFound := false

	for i := 0; i < 30; i++ {
		clients, _ := suite.helper.UserClient.Get(clusterInfo, "client."+clientName)
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
	caps, _ = suite.helper.UserClient.Update(clusterInfo, clientName, newcaps)

	require.Equal(suite.T(), "allow r", caps["mon"], "wrong caps")
	require.Equal(suite.T(), "allow rw", caps["mgr"], "wrong caps")
	require.Equal(suite.T(), "allow *", caps["osd"], "wrong caps")
}

// Smoke Test for RBD Mirror CRD
func (suite *SmokeSuite) TestCreateRBDMirrorClient() {
	logger.Infof("Create rbd-mirror Smoke Test")

	rbdMirrorName := "my-rbd-mirror"

	err := suite.helper.RBDMirrorClient.Create(suite.namespace, rbdMirrorName, 1)
	require.Nil(suite.T(), err)

	err = suite.helper.RBDMirrorClient.Delete(suite.namespace, rbdMirrorName)
	require.Nil(suite.T(), err)
}

func (suite *SmokeSuite) getNonCanaryMonDeployments() ([]appsv1.Deployment, error) {
	ctx := context.TODO()
	opts := metav1.ListOptions{LabelSelector: "app=rook-ceph-mon"}
	deployments, err := suite.k8sh.Clientset.AppsV1().Deployments(suite.namespace).List(ctx, opts)
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
