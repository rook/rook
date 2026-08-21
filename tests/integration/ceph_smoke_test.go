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

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	store := sharedstore.Create(s.T(), s.k8sh, s.installer, sharedstore.Config{
		Namespace: s.settings.Namespace,
		StoreName: "lite-store",
		Instances: 2,
	})
	store.Destroy()
}

// Test to make sure all rook components are installed and Running
func (s *SmokeSuite) TestARookClusterInstallation_SmokeTest() {
	checkIfRookClusterIsInstalled(&s.Suite, s.k8sh, s.settings.OperatorNamespace, s.settings.Namespace, 3)
}

// Smoke Test for Mon failover - Test checks the following operations for the Mon failover in order
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

// Smoke Test for the do-not-reconcile label - Fence an OSD deployment, reconcile the cluster and
// check that the fenced deployment is left as it was found, then lift the fence and check that the
// next reconcile rewrites it
func (s *SmokeSuite) TestDoNotReconcileLabel() {
	ctx := context.TODO()
	logger.Infof("Do Not Reconcile Label Smoke Test")

	const (
		// The operator builds an OSD deployment from scratch whenever it updates one, so a label it
		// never sets survives exactly as long as the deployment is left alone
		canaryLabelKey = "smoke-test.rook.io/canary"
		// Asking for an OSD label through the CephCluster spec is what gives a reconcile real work to
		// do on the OSD deployments. Left alone they already match what the operator would generate,
		// and it updates neither the fenced nor the unfenced ones.
		requestedLabelKey = "smoke-test.rook.io/requested"
	)

	// Starting from a clean cluster keeps unhealthiness left by an earlier test from being blamed
	// on this test's deferred health check
	checkIfRookClusterIsHealthy(&s.Suite, s.helper, s.settings.Namespace)

	deploymentClient := s.k8sh.Clientset.AppsV1().Deployments(s.settings.Namespace)
	osdDeployments, err := deploymentClient.List(ctx, metav1.ListOptions{LabelSelector: "app=rook-ceph-osd"})
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), osdDeployments.Items)

	fenced := osdDeployments.Items[0]
	osdID := fenced.Labels["ceph-osd-id"]
	require.NotEmpty(s.T(), osdID)
	generation := fenced.Generation
	replicas := fenced.Spec.Replicas
	logger.Infof("Fencing OSD %s deployment %q at generation %d", osdID, fenced.Name, generation)

	clusterClient := s.k8sh.RookClientset.CephV1().CephClusters(s.settings.Namespace)

	// Cleanup runs LIFO, so it is registered in reverse of the order it has to happen in: drop the
	// labels this test put on the deployment, stop asking for the OSD label, and only then wait for
	// the cluster to settle so the tests that follow do not start against a reconciling cluster.
	// Each step is registered before the change it undoes, since a request can fail after the
	// apiserver has already accepted it.
	defer checkIfRookClusterIsHealthy(&s.Suite, s.helper, s.settings.Namespace)
	defer func() {
		dropOSDLabel := []byte(`{"spec":{"labels":{"osd":null}}}`)
		_, err := clusterClient.Patch(ctx, s.settings.ClusterName, types.MergePatchType, dropOSDLabel, metav1.PatchOptions{})
		assert.NoError(s.T(), err)
	}()
	defer func() {
		removeLabels := []byte(fmt.Sprintf(`{"metadata":{"labels":{%q:null,%q:null}}}`, cephv1.SkipReconcileLabelKey, canaryLabelKey))
		_, err := deploymentClient.Patch(ctx, fenced.Name, types.StrategicMergePatchType, removeLabels, metav1.PatchOptions{})
		assert.NoError(s.T(), err)
	}()

	addLabels := []byte(fmt.Sprintf(`{"metadata":{"labels":{%q:"true",%q:"true"}}}`, cephv1.SkipReconcileLabelKey, canaryLabelKey))
	_, err = deploymentClient.Patch(ctx, fenced.Name, types.StrategicMergePatchType, addLabels, metav1.PatchOptions{})
	require.NoError(s.T(), err)

	s.reconcileCluster(ctx, fmt.Sprintf(`{"spec":{"labels":{"osd":{%q:"fenced"}}}}`, requestedLabelKey))

	current, err := deploymentClient.Get(ctx, fenced.Name, metav1.GetOptions{})
	require.NoError(s.T(), err)
	// A canary that is already gone would make the rewrite check at the end of the test vacuous
	require.Contains(s.T(), current.Labels, canaryLabelKey, "OSD %s lost the canary label while fenced", osdID)
	assert.Contains(s.T(), current.Labels, cephv1.SkipReconcileLabelKey, "the do-not-reconcile label was removed from OSD %s", osdID)
	assert.NotContains(s.T(), current.Labels, requestedLabelKey, "the fenced OSD %s was given the label requested through the cluster spec", osdID)
	// The generation advances whenever the deployment spec is written with different content, so an
	// unchanged generation means the fenced OSD was left as it was found
	assert.Equal(s.T(), generation, current.Generation)
	assert.Equal(s.T(), replicas, current.Spec.Replicas)

	logger.Infof("Lifting the fence on OSD %s deployment %q", osdID, fenced.Name)
	unfence := []byte(fmt.Sprintf(`{"metadata":{"labels":{%q:null}}}`, cephv1.SkipReconcileLabelKey))
	_, err = deploymentClient.Patch(ctx, fenced.Name, types.StrategicMergePatchType, unfence, metav1.PatchOptions{})
	require.NoError(s.T(), err)

	s.reconcileCluster(ctx, fmt.Sprintf(`{"spec":{"labels":{"osd":{%q:"unfenced"}}}}`, requestedLabelKey))

	osdRewritten := false
	for i := 0; i < utils.RetryLoop; i++ {
		current, err = deploymentClient.Get(ctx, fenced.Name, metav1.GetOptions{})
		require.NoError(s.T(), err)
		if _, ok := current.Labels[canaryLabelKey]; !ok {
			logger.Infof("The reconcile rewrote the unfenced OSD %s", osdID)
			osdRewritten = true
			break
		}

		logger.Infof("Waiting for the reconcile to rewrite the unfenced OSD %s", osdID)
		time.Sleep(utils.RetryInterval * time.Second)
	}
	require.True(s.T(), osdRewritten, "the unfenced OSD %s kept the canary label", osdID)
	assert.Contains(s.T(), current.Labels, requestedLabelKey, "the unfenced OSD %s was not given the label requested through the cluster spec", osdID)
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

// reconcileCluster patches the CephCluster spec and waits for the operator to finish the
// orchestration that saw the patch. The patch has to change the spec, since that is what starts an
// orchestration, and the observed generation is only written once a whole orchestration, OSDs
// included, has succeeded.
func (s *SmokeSuite) reconcileCluster(ctx context.Context, specPatch string) {
	clusterClient := s.k8sh.RookClientset.CephV1().CephClusters(s.settings.Namespace)
	patched, err := clusterClient.Patch(ctx, s.settings.ClusterName, types.MergePatchType, []byte(specPatch), metav1.PatchOptions{})
	require.NoError(s.T(), err)

	logger.Infof("Waiting for cluster %q to reconcile generation %d", s.settings.ClusterName, patched.Generation)
	for i := 0; i < utils.RetryLoop; i++ {
		cluster, err := clusterClient.Get(ctx, s.settings.ClusterName, metav1.GetOptions{})
		require.NoError(s.T(), err)
		if cluster.Status.ObservedGeneration >= patched.Generation {
			logger.Infof("Cluster %q reconciled generation %d", s.settings.ClusterName, patched.Generation)
			return
		}

		logger.Infof("Cluster %q is in phase %q having observed generation %d", s.settings.ClusterName, cluster.Status.Phase, cluster.Status.ObservedGeneration)
		time.Sleep(utils.RetryInterval * time.Second)
	}

	require.Failf(s.T(), "giving up waiting for the cluster to reconcile", "generation %d was never observed", patched.Generation)
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
