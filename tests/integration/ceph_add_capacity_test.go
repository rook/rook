/*
Copyright 2026 The Rook Authors. All rights reserved.

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
	"testing"
	"time"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	initialOSDCount = 3
	targetOSDCount  = 6
)

// ************************************************
// *** Major scenarios tested by the AddCapacitySuite ***
// Setup
// - Deploy a CephCluster with PVC-based storage (storageClassDeviceSets) and 3 OSDs
// - Verify the cluster is healthy with 3 OSDs
// Add Capacity
// - Increase storageClassDeviceSets count from 3 to 6
// - Verify 6 OSD pods reach Running state
// - Verify CephCluster CR reaches Ready phase
// - Verify ceph health is HEALTH_OK
// ************************************************
func TestCephAddCapacitySuite(t *testing.T) {
	s := new(AddCapacitySuite)
	defer func(s *AddCapacitySuite) {
		HandlePanics(recover(), s.TearDownSuite, s.T)
	}(s)
	suite.Run(t, s)
}

type AddCapacitySuite struct {
	suite.Suite
	helper    *clients.TestClient
	k8sh      *utils.K8sHelper
	settings  *installer.TestCephSettings
	installer *installer.CephInstaller
}

func (s *AddCapacitySuite) SetupSuite() {
	namespace := "rook-ceph"
	s.settings = &installer.TestCephSettings{
		ClusterName:       "rook-ceph",
		Namespace:         namespace,
		OperatorNamespace: installer.SystemNamespace(namespace),
		StorageClassName:  installer.StorageClassName(),
		UseHelm:           false,
		UsePVC:            true,
		Mons:              1,
		SkipOSDCreation:   false,
		RookVersion:       installer.LocalBuildTag,
		CephVersion:       installer.ReturnCephVersion(),
		OsdCount:          initialOSDCount,
		OsdPVCSize:        "6Gi",
		AllowLoopDevices:  true,
	}
	s.settings.ApplyEnvVars()
	s.installer, s.k8sh = StartTestCluster(s.T, s.settings)
	s.helper = clients.CreateTestClient(s.k8sh, s.installer.Manifests)
}

func (s *AddCapacitySuite) AfterTest(suiteName, testName string) {
	s.installer.CollectOperatorLog(suiteName, testName)
}

func (s *AddCapacitySuite) TearDownSuite() {
	s.installer.UninstallRook()
}

func (s *AddCapacitySuite) TestAddOSDCapacity() {
	logger.Infof("================================================================")
	logger.Infof("  ADD OSD CAPACITY TEST: scaling from %d to %d OSDs", initialOSDCount, targetOSDCount)
	logger.Infof("  Namespace: %s, Cluster: %s", s.settings.Namespace, s.settings.ClusterName)
	logger.Infof("================================================================")

	// Step 1: Verify the cluster has 3 OSDs running and ceph health is HEALTH_OK
	logger.Infof("*** STEP 1: Verifying initial cluster state (%d OSDs expected) ***", initialOSDCount)
	s.logOSDPods()

	require.True(s.T(), s.k8sh.CheckPodCountAndState("rook-ceph-osd", s.settings.Namespace, initialOSDCount, "Running"),
		fmt.Sprintf("expected %d OSD pods in Running state before expansion", initialOSDCount))
	logger.Infof("STEP 1: confirmed %d OSD pods are Running", initialOSDCount)

	s.waitForHealthOK(300 * time.Second)
	checkIfRookClusterIsHealthy(&s.Suite, s.helper, s.settings.Namespace)
	logger.Infof("STEP 1 PASSED: cluster is healthy with %d OSDs", initialOSDCount)

	// Step 2: Increase the storageClassDeviceSets count from 3 to 6
	logger.Infof("*** STEP 2: Patching CephCluster CR to increase OSD count from %d to %d ***", initialOSDCount, targetOSDCount)
	patchJSON := fmt.Sprintf(`[{"op":"replace","path":"/spec/storage/storageClassDeviceSets/0/count","value":%d}]`, targetOSDCount)
	logger.Infof("STEP 2: applying JSON patch: %s", patchJSON)

	_, err := s.k8sh.Kubectl("-n", s.settings.Namespace, "patch", "CephCluster", s.settings.ClusterName,
		"--type=json", "-p", patchJSON)
	require.NoError(s.T(), err)
	logger.Infof("STEP 2 PASSED: CephCluster CR patched, storageClassDeviceSets[0].count is now %d", targetOSDCount)

	// Step 3: Verify 6 OSD pods reach Running state
	logger.Infof("*** STEP 3: Waiting for %d OSD pods to reach Running state ***", targetOSDCount)
	require.True(s.T(), s.k8sh.CheckPodCountAndState("rook-ceph-osd", s.settings.Namespace, targetOSDCount, "Running"),
		fmt.Sprintf("expected %d OSD pods in Running state after capacity expansion", targetOSDCount))

	s.logOSDPods()
	logger.Infof("STEP 3 PASSED: %d OSD pods are Running", targetOSDCount)

	// Step 4: Verify CephCluster CR reaches Ready state
	logger.Infof("*** STEP 4: Waiting for CephCluster CR to reach Ready phase ***")
	err = s.k8sh.WaitForStatusPhase(s.settings.Namespace, "CephCluster", s.settings.ClusterName, "Ready", 300*time.Second)
	require.NoError(s.T(), err)
	logger.Infof("STEP 4 PASSED: CephCluster CR is in Ready phase")

	// Step 5: Verify ceph health is HEALTH_OK
	logger.Infof("*** STEP 5: Waiting for ceph health to be HEALTH_OK ***")
	s.waitForHealthOK(600 * time.Second)
	checkIfRookClusterIsHealthy(&s.Suite, s.helper, s.settings.Namespace)
	logger.Infof("STEP 5 PASSED: ceph health is HEALTH_OK")

	logger.Infof("================================================================")
	logger.Infof("  ADD OSD CAPACITY TEST PASSED: %d -> %d OSDs", initialOSDCount, targetOSDCount)
	logger.Infof("================================================================")
}

func (s *AddCapacitySuite) logOSDPods() {
	pods, err := s.k8sh.Clientset.CoreV1().Pods(s.settings.Namespace).List(
		context.TODO(), metav1.ListOptions{LabelSelector: "app=rook-ceph-osd"})
	if err != nil {
		logger.Warningf("failed to list OSD pods: %v", err)
		return
	}
	logger.Infof("--- OSD pods in namespace %s (%d total) ---", s.settings.Namespace, len(pods.Items))
	for _, pod := range pods.Items {
		logger.Infof("  pod=%s  phase=%s  node=%s", pod.Name, pod.Status.Phase, pod.Spec.NodeName)
	}
	logger.Infof("---")
}

// waitForHealthOK polls the CephCluster CR status until ceph health reports HEALTH_OK.
func (s *AddCapacitySuite) waitForHealthOK(timeout time.Duration) {
	ctx := context.TODO()
	var lastHealth string
	err := wait.PollUntilContextTimeout(ctx, 10*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			cluster, err := s.k8sh.RookClientset.CephV1().CephClusters(s.settings.Namespace).Get(ctx, s.settings.ClusterName, metav1.GetOptions{})
			if err != nil {
				logger.Warningf("failed to get CephCluster CR: %v", err)
				return false, nil
			}
			lastHealth = cluster.Status.CephStatus.Health
			if lastHealth == "HEALTH_OK" {
				logger.Infof("ceph health is HEALTH_OK")
				return true, nil
			}
			logger.Infof("waiting for HEALTH_OK, current health: %s", lastHealth)
			return false, nil
		})
	require.NoError(s.T(), err, "timed out waiting for HEALTH_OK, last health: %s", lastHealth)
}
