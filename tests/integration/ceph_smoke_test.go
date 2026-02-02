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
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
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
	s := new(SmokeSuite)
	defer func(s *SmokeSuite) {
		HandlePanics(recover(), s.TearDownSuite, s.T)
	}(s)
	suite.Run(t, s)
}

type SmokeSuite struct {
	suite.Suite
	helper    *clients.TestClient
	settings  *installer.TestCephSettings
	installer *installer.CephInstaller
	k8sh      *utils.K8sHelper
}

func (s *SmokeSuite) SetupSuite() {
	namespace := "smoke-ns"
	s.settings = &installer.TestCephSettings{
		ClusterName:             "smoke-cluster",
		Namespace:               namespace,
		OperatorNamespace:       installer.SystemNamespace(namespace),
		StorageClassName:        installer.StorageClassName(),
		UseHelm:                 false,
		UsePVC:                  installer.UsePVC(),
		Mons:                    3,
		SkipOSDCreation:         false,
		ConnectionsEncrypted:    true,
		ConnectionsCompressed:   true,
		UseCrashPruner:          true,
		EnableVolumeReplication: true,
		TestNFSCSI:              true,
		ChangeHostName:          true,
		RookVersion:             installer.LocalBuildTag,
		CephVersion:             installer.ReturnCephVersion(),
	}
	s.settings.ApplyEnvVars()
	s.installer, s.k8sh = StartTestCluster(s.T, s.settings)
	s.helper = clients.CreateTestClient(s.k8sh, s.installer.Manifests)
}

func (s *SmokeSuite) AfterTest(suiteName, testName string) {
	s.installer.CollectOperatorLog(suiteName, testName)
}

func (s *SmokeSuite) TearDownSuite() {
	s.installer.UninstallRook()
}

func (s *SmokeSuite) TestCephNFS_SmokeTest() {
	runNFSFileE2ETest(s.helper, s.k8sh, &s.Suite, s.settings, "smoke-test-nfs")
}

func (s *SmokeSuite) TestBlockStorage_SmokeTest() {
	runBlockCSITest(s.helper, s.k8sh, &s.Suite, s.settings.Namespace)
}

func (s *SmokeSuite) TestFileStorage_SmokeTest() {
	preserveFilesystemOnDelete := true
	runFileE2ETest(s.helper, s.k8sh, &s.Suite, s.settings, "smoke-test-fs", preserveFilesystemOnDelete)
}

func (s *SmokeSuite) TestObjectStorage_SmokeTest() {
	if utils.IsPlatformOpenShift() {
		s.T().Skip("object store tests skipped on openshift")
	}
	storeName := "lite-store"
	deleteStore := true
	tls := false
	runObjectE2ETestLite(s.T(), s.helper, s.k8sh, s.installer, s.settings.Namespace, storeName, 2, deleteStore, tls, false)
}

// Test to make sure all rook components are installed and Running
func (s *SmokeSuite) TestARookClusterInstallation_SmokeTest() {
	checkIfRookClusterIsInstalled(&s.Suite, s.k8sh, s.settings.OperatorNamespace, s.settings.Namespace, 3)
}

// Smoke Test for Mon failover - Test check the following operations for the Mon failover in order
// Delete mon pod, Wait for new mon pod
func (s *SmokeSuite) TestMonFailover() {
	ctx := context.TODO()
	logger.Infof("Mon Failover Smoke Test")

	deployments, err := s.getNonCanaryMonDeployments()
	require.NoError(s.T(), err)
	require.Equal(s.T(), 3, len(deployments))

	// Scale down a mon so the operator won't trigger a reconcile
	monToKill := deployments[0].Name
	logger.Infof("Scaling down mon %s", monToKill)
	scale, err := s.k8sh.Clientset.AppsV1().Deployments(s.settings.Namespace).GetScale(ctx, monToKill, metav1.GetOptions{})
	assert.NoError(s.T(), err)
	scale.Spec.Replicas = 0
	_, err = s.k8sh.Clientset.AppsV1().Deployments(s.settings.Namespace).UpdateScale(ctx, monToKill, scale, metav1.UpdateOptions{})
	assert.NoError(s.T(), err)

	// Wait for the health check to start a new monitor
	for i := 0; i < 30; i++ {
		deployments, err := s.getNonCanaryMonDeployments()
		require.NoError(s.T(), err)

		var currentMons []string
		var originalMonDeployment *appsv1.Deployment
		for i, mon := range deployments {
			currentMons = append(currentMons, mon.Name)
			if mon.Name == monToKill {
				originalMonDeployment = &deployments[i]
			}
		}
		logger.Infof("mon deployments: %v", currentMons)

		// Check if the original mon was scaled up again
		// Depending on the state of the orchestration, the operator might trigger
		// re-creation of the deleted mon. In this case, consider the test successful
		// rather than wait for the failover which will never occur.
		if originalMonDeployment != nil && *originalMonDeployment.Spec.Replicas > 0 {
			logger.Infof("Original mon created again, no need to wait for mon failover")
			return
		}

		if len(deployments) == 3 && originalMonDeployment == nil {
			logger.Infof("Found a new monitor!")
			return
		}

		logger.Infof("Waiting for a new monitor to start and previous one to be deleted")
		time.Sleep(5 * time.Second)
	}

	require.Fail(s.T(), "giving up waiting for a new monitor")
}

// Smoke Test for pool Resizing
func (s *SmokeSuite) TestPoolResize() {
	ctx := context.TODO()
	logger.Infof("Pool Resize Smoke Test")

	poolName := "testpool"
	err := s.helper.PoolClient.Create(poolName, s.settings.Namespace, 1)
	require.NoError(s.T(), err)

	poolFound := false
	clusterInfo := client.AdminTestClusterInfo(s.settings.Namespace)

	// Wait for pool to appear
	for i := 0; i < 10; i++ {
		pools, err := s.helper.PoolClient.ListCephPools(clusterInfo)
		require.NoError(s.T(), err)
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

	require.Equal(s.T(), true, poolFound, "pool not found")

	err = s.helper.PoolClient.Update(poolName, s.settings.Namespace, 2)
	require.NoError(s.T(), err)

	poolResized := false
	// Wait for pool resize to happen
	for i := 0; i < 10; i++ {
		details, err := s.helper.PoolClient.GetCephPoolDetails(clusterInfo, poolName)
		require.NoError(s.T(), err)
		if details.Size > 1 {
			logger.Infof("pool %s size was updated", poolName)
			// nolint:gosec // G115 no overflow expected in the test
			require.Equal(s.T(), 2, int(details.Size))
			poolResized = true

			// resize the pool back to 1 to avoid hangs around not having enough OSDs to satisfy rbd
			err = s.helper.PoolClient.Update(poolName, s.settings.Namespace, 1)
			require.NoError(s.T(), err)
		} else if poolResized && details.Size == 1 {
			logger.Infof("pool resized back to 1")
			break
		}

		logger.Debugf("pool %s size not updated yet. details: %+v", poolName, details)
		logger.Infof("Waiting for pool %s resize to happen", poolName)
		time.Sleep(2 * time.Second)
	}

	require.Equal(s.T(), true, poolResized, fmt.Sprintf("pool %s not found", poolName))

	// Verify the Kubernetes Secret has been created (bootstrap peer token)
	pool, err := s.k8sh.RookClientset.CephV1().CephBlockPools(s.settings.Namespace).Get(ctx, poolName, metav1.GetOptions{})
	assert.NoError(s.T(), err)
	if pool.Spec.Mirroring.Enabled {
		secretName := pool.Status.Info[opcontroller.RBDMirrorBootstrapPeerSecretName]
		assert.NotEmpty(s.T(), secretName)
		// now fetch the secret which contains the bootstrap peer token
		secret, err := s.k8sh.Clientset.CoreV1().Secrets(s.settings.Namespace).Get(ctx, secretName, metav1.GetOptions{})
		require.NoError(s.T(), err)
		assert.NotEmpty(s.T(), secret.Data["token"])
	}

	// clean up the pool
	err = s.helper.PoolClient.DeletePool(s.helper.BlockClient, clusterInfo, poolName)
	assert.NoError(s.T(), err)
}

// Smoke Test for Client CRD
func (s *SmokeSuite) TestCreateClient() {
	logger.Infof("Create Client Smoke Test")

	clientName := "client1"
	caps := map[string]string{
		"mon": "allow rwx",
		"mgr": "allow rwx",
		"osd": "allow rwx",
	}
	clusterInfo := client.AdminTestClusterInfo(s.settings.Namespace)
	err := s.helper.UserClient.Create(clientName, s.settings.Namespace, caps)
	require.NoError(s.T(), err)

	clientFound := false

	for i := 0; i < 30; i++ {
		clients, _ := s.helper.UserClient.Get(clusterInfo, "client."+clientName)
		if clients != "" {
			clientFound = true
		}

		if clientFound {
			break
		}

		logger.Infof("Waiting for client to appear")
		time.Sleep(2 * time.Second)
	}

	assert.Equal(s.T(), true, clientFound, "client not found")

	logger.Infof("Update Client Smoke Test")
	newcaps := map[string]string{
		"mon": "allow r",
		"mgr": "allow rw",
		"osd": "allow *",
	}
	caps, _ = s.helper.UserClient.Update(clusterInfo, clientName, newcaps)

	assert.Equal(s.T(), "allow r", caps["mon"], "wrong caps")
	assert.Equal(s.T(), "allow rw", caps["mgr"], "wrong caps")
	assert.Equal(s.T(), "allow *", caps["osd"], "wrong caps")

	err = s.helper.UserClient.Delete(clientName, s.settings.Namespace)
	require.NoError(s.T(), err)
}

// Smoke Test for RBD Mirror CRD
func (s *SmokeSuite) TestCreateRBDMirrorClient() {
	logger.Infof("Create rbd-mirror Smoke Test")

	rbdMirrorName := "my-rbd-mirror"

	err := s.helper.RBDMirrorClient.Create(s.settings.Namespace, rbdMirrorName, 1)
	require.NoError(s.T(), err)

	err = s.helper.RBDMirrorClient.Delete(s.settings.Namespace, rbdMirrorName)
	require.NoError(s.T(), err)
}

func (s *SmokeSuite) getNonCanaryMonDeployments() ([]appsv1.Deployment, error) {
	ctx := context.TODO()
	opts := metav1.ListOptions{LabelSelector: "app=rook-ceph-mon"}
	deployments, err := s.k8sh.Clientset.AppsV1().Deployments(s.settings.Namespace).List(ctx, opts)
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
