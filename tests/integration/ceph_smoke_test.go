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
	"strconv"
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
	corev1 "k8s.io/api/core/v1"
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
	store := sharedstore.Create(s.T(), s.k8sh, s.installer, false, s.settings.Namespace, "lite-store", 2)
	store.Destroy()
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

// Smoke Test for the do-not-reconcile label - Fence an OSD deployment with the label and mark the
// OSD out, then trigger an orchestration and check that the operator skips the OSD and leaves both
// the label and the OSD state alone
func (s *SmokeSuite) TestDoNotReconcileLabel() {
	ctx := context.TODO()
	logger.Infof("Do Not Reconcile Label Smoke Test")

	// Taking an OSD out is only meaningful from a clean starting point, and checking here keeps a
	// cluster left unhealthy by an earlier test from being blamed on this one
	checkIfRookClusterIsHealthy(&s.Suite, s.helper, s.settings.Namespace)

	deploymentClient := s.k8sh.Clientset.AppsV1().Deployments(s.settings.Namespace)
	osdDeployments, err := deploymentClient.List(ctx, metav1.ListOptions{LabelSelector: "app=rook-ceph-osd"})
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), osdDeployments.Items)

	fenced := osdDeployments.Items[0]
	osdID := fenced.Labels["ceph-osd-id"]
	require.NotEmpty(s.T(), osdID)
	osdNum, err := strconv.ParseInt(osdID, 10, 64)
	require.NoError(s.T(), err)
	generation := fenced.Generation
	replicas := fenced.Spec.Replicas
	logger.Infof("Fencing OSD %s deployment %q at generation %d", osdID, fenced.Name, generation)

	clusterClient := s.k8sh.RookClientset.CephV1().CephClusters(s.settings.Namespace)
	cluster, err := clusterClient.Get(ctx, s.settings.ClusterName, metav1.GetOptions{})
	require.NoError(s.T(), err)

	// Any change to the CephCluster spec makes the operator run a fresh orchestration. This flag is
	// only read when the upgrade checks are enabled, so with them skipped the toggle starts an
	// orchestration without changing anything the orchestration does.
	require.True(s.T(), cluster.Spec.SkipUpgradeChecks)
	continueUpgrade := cluster.Spec.ContinueUpgradeAfterChecksEvenIfNotHealthy
	toggleSpec := []byte(fmt.Sprintf(`{"spec":{"continueUpgradeAfterChecksEvenIfNotHealthy":%t}}`, !continueUpgrade))
	revertSpec := []byte(fmt.Sprintf(`{"spec":{"continueUpgradeAfterChecksEvenIfNotHealthy":%t}}`, continueUpgrade))

	// Cleanup runs LIFO, so it is registered in reverse of the order it has to happen in: put the OSD
	// back in, unfence the deployment, revert the spec, and only then wait for the cluster to go
	// clean again so the tests that follow do not start against a recovering cluster. Each step is
	// registered before the change it undoes, since a request can fail after the apiserver or the
	// mon has already accepted it.
	defer checkIfRookClusterIsHealthy(&s.Suite, s.helper, s.settings.Namespace)
	defer func() {
		_, err := clusterClient.Patch(ctx, s.settings.ClusterName, types.MergePatchType, revertSpec, metav1.PatchOptions{})
		assert.NoError(s.T(), err)
	}()

	addLabel := []byte(fmt.Sprintf(`{"metadata":{"labels":{%q:"true"}}}`, cephv1.SkipReconcileLabelKey))
	removeLabel := []byte(fmt.Sprintf(`{"metadata":{"labels":{%q:null}}}`, cephv1.SkipReconcileLabelKey))
	defer func() {
		_, err := deploymentClient.Patch(ctx, fenced.Name, types.StrategicMergePatchType, removeLabel, metav1.PatchOptions{})
		assert.NoError(s.T(), err)
	}()
	_, err = deploymentClient.Patch(ctx, fenced.Name, types.StrategicMergePatchType, addLabel, metav1.PatchOptions{})
	require.NoError(s.T(), err)

	defer func() {
		logger.Infof("Marking OSD %s back in", osdID)
		_, err := s.k8sh.ExecToolboxWithRetry(3, s.settings.Namespace, "ceph", []string{"osd", "in", fmt.Sprintf("osd.%s", osdID)})
		assert.NoError(s.T(), err)
	}()
	logger.Infof("Marking OSD %s out", osdID)
	_, err = s.k8sh.ExecToolboxWithRetry(3, s.settings.Namespace, "ceph", []string{"osd", "out", fmt.Sprintf("osd.%s", osdID)})
	require.NoError(s.T(), err)

	osdIsOut := false
	for i := 0; i < utils.RetryLoop; i++ {
		in, err := s.osdIsIn(osdNum)
		require.NoError(s.T(), err)
		if !in {
			logger.Infof("OSD %s is out", osdID)
			osdIsOut = true
			break
		}

		logger.Infof("Waiting for OSD %s to report out", osdID)
		time.Sleep(utils.RetryInterval * time.Second)
	}
	require.True(s.T(), osdIsOut, "OSD never reported out after being marked out")

	logger.Infof("Triggering an orchestration of cluster %q", s.settings.ClusterName)
	triggered := time.Now()
	_, err = clusterClient.Patch(ctx, s.settings.ClusterName, types.MergePatchType, toggleSpec, metav1.PatchOptions{})
	require.NoError(s.T(), err)

	skipMessage := fmt.Sprintf("Skipping update for OSD %s since labeled with %s", osdID, cephv1.SkipReconcileLabelKey)
	osdSkipped := false
	for i := 0; i < utils.RetryLoop; i++ {
		current, err := deploymentClient.Get(ctx, fenced.Name, metav1.GetOptions{})
		require.NoError(s.T(), err)
		// Without the label the operator has no reason to log the skip, so report the missing fence
		// rather than letting this time out as if the orchestration never ran
		require.Contains(s.T(), current.Labels, cephv1.SkipReconcileLabelKey, "the do-not-reconcile label was removed from OSD %s", osdID)

		matches, err := s.countOperatorLogMatches(ctx, skipMessage, time.Since(triggered))
		require.NoError(s.T(), err)
		if matches > 0 {
			logger.Infof("Operator skipped the fenced OSD %s", osdID)
			osdSkipped = true
			break
		}

		logger.Infof("Waiting for the operator to skip the fenced OSD %s", osdID)
		time.Sleep(utils.RetryInterval * time.Second)
	}
	require.True(s.T(), osdSkipped, "operator never reported skipping the fenced OSD")

	// The OSD health check runs every 10s in the test cluster, so this window spans several ticks of
	// anything that might be tempted to unfence the OSD or put it back in behind the operator's back
	for i := 0; i < 6; i++ {
		time.Sleep(5 * time.Second)
		current, err := deploymentClient.Get(ctx, fenced.Name, metav1.GetOptions{})
		require.NoError(s.T(), err)
		assert.Contains(s.T(), current.Labels, cephv1.SkipReconcileLabelKey, "the do-not-reconcile label was removed from OSD %s", osdID)

		in, err := s.osdIsIn(osdNum)
		require.NoError(s.T(), err)
		assert.False(s.T(), in, "OSD %s was marked back in", osdID)
	}

	current, err := deploymentClient.Get(ctx, fenced.Name, metav1.GetOptions{})
	require.NoError(s.T(), err)
	// The generation advances whenever the deployment spec is written with different content, so an
	// unchanged generation means the fenced OSD was left as it was found
	assert.Equal(s.T(), generation, current.Generation)
	assert.Equal(s.T(), replicas, current.Spec.Replicas)
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

func (s *SmokeSuite) osdIsIn(osdID int64) (bool, error) {
	dump, err := client.GetOSDDump(s.k8sh.MakeContext(), client.AdminTestClusterInfo(s.settings.Namespace))
	if err != nil {
		return false, err
	}
	_, in, err := dump.StatusByID(osdID)
	if err != nil {
		return false, err
	}
	return in == 1, nil
}

func (s *SmokeSuite) countOperatorLogMatches(ctx context.Context, match string, window time.Duration) (int, error) {
	podClient := s.k8sh.Clientset.CoreV1().Pods(s.settings.OperatorNamespace)
	pods, err := podClient.List(ctx, metav1.ListOptions{LabelSelector: "app=rook-ceph-operator"})
	if err != nil {
		return 0, err
	}
	if len(pods.Items) == 0 {
		return 0, fmt.Errorf("no operator pod found in namespace %q", s.settings.OperatorNamespace)
	}

	// An elapsed duration keeps the window anchored where the caller wants it even if the node clock
	// disagrees with the test runner, which an absolute timestamp would not
	sinceSeconds := int64(window.Seconds()) + 1
	matches := 0
	for _, pod := range pods.Items {
		podLog, err := podClient.GetLogs(pod.Name, &corev1.PodLogOptions{SinceSeconds: &sinceSeconds}).DoRaw(ctx)
		if err != nil {
			return 0, err
		}
		matches += strings.Count(string(podLog), match)
	}
	return matches, nil
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
