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

	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ************************************************
// *** Major scenarios tested by the UpgradeSuite ***
// Setup
// - Initially create a cluster from the previous minor release
// - Upgrade to the current build of Rook to verify functionality after upgrade
// - Test basic usage of block, object, and file after upgrade
// Monitors
// - One mon in the cluster
// ************************************************
func TestUpgradeSuite(t *testing.T) {
	s := new(UpgradeSuite)
	defer func(s *UpgradeSuite) {
		HandlePanics(recover(), s.op, s.T)
	}(s)
	suite.Run(t, s)
}

type UpgradeSuite struct {
	suite.Suite
	helper    *clients.TestClient
	op        *TestCluster
	k8sh      *utils.K8sHelper
	namespace string
}

func (s *UpgradeSuite) SetupSuite() {
	s.namespace = "upgrade-ns"
	useDevices := true

	s.op, s.k8sh = StartTestCluster(s.T, s.namespace, "bluestore", false, useDevices, 3, installer.Version0_8)
	s.helper = clients.CreateTestClient(s.k8sh, s.op.installer.Manifests)
}

func (s *UpgradeSuite) TearDownSuite() {
	s.op.Teardown()
}

func (s *UpgradeSuite) TestUpgradeToMaster() {
	systemNamespace := installer.SystemNamespace(s.namespace)

	// Create block, object, and file storage on 0.8 before the upgrade
	poolName := "upgradepool"
	storageClassName := "block-upgrade"
	blockName := "block-claim-upgrade"
	podName := "test-pod-upgrade"
	logger.Infof("Initializing block before the upgrade")
	setupBlockLite(s.helper, s.k8sh, s.Suite, s.namespace, poolName, storageClassName, blockName, podName)
	createPodWithBlock(s.helper, s.k8sh, s.Suite, s.namespace, blockName, podName)
	defer blockTestDataCleanUp(s.helper, s.k8sh, s.namespace, poolName, storageClassName, blockName, podName)

	logger.Infof("Initializing file before the upgrade")
	filesystemName := "upgrade-test-fs"
	createFilesystem(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)
	createFilesystemConsumerPod(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)
	defer cleanupFilesystemConsumer(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)

	logger.Infof("Initializing object before the upgrade")
	objectStoreName := "upgraded-object"
	runObjectE2ETestLite(s.helper, s.k8sh, s.Suite, s.namespace, objectStoreName, 1)

	// verify that we're actually running 0.8 before the upgrade
	operatorContainer := "rook-ceph-operator"
	version, err := k8sutil.GetDeploymentVersion(s.k8sh.Clientset, systemNamespace, operatorContainer, operatorContainer)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), "v0.8.1", version)

	message := "my simple message"
	preFilename := "pre-upgrade-file"
	assert.Nil(s.T(), s.k8sh.WriteToPod("", podName, preFilename, message))
	assert.Nil(s.T(), s.k8sh.ReadFromPod("", podName, preFilename, message))
	assert.Nil(s.T(), s.k8sh.WriteToPod(s.namespace, filePodName, preFilename, message))
	assert.Nil(s.T(), s.k8sh.ReadFromPod(s.namespace, filePodName, preFilename, message))

	// Upgrade to master
	require.Nil(s.T(), s.k8sh.SetDeploymentVersion(systemNamespace, operatorContainer, operatorContainer, installer.VersionMaster))

	// verify that the operator spec is updated
	version, err = k8sutil.GetDeploymentVersion(s.k8sh.Clientset, systemNamespace, operatorContainer, operatorContainer)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), installer.VersionMaster, version)

	// wait for the legacy mon replicasets to be deleted
	err = s.waitForLegacyMonReplicaSetDeletion()
	require.Nil(s.T(), err)

	// wait for the mon pods to be running
	err = k8sutil.WaitForDeploymentVersion(s.k8sh.Clientset, s.namespace, "app=rook-ceph-mon", "rook-ceph-mon", installer.VersionMaster)
	require.Nil(s.T(), err)

	s.k8sh.WaitForLabeledPodsToRun("app=rook-ceph-mon", s.namespace)

	// wait for the osd pods to be updated
	err = k8sutil.WaitForDeploymentVersion(s.k8sh.Clientset, s.namespace, "app=rook-ceph-osd", "rook-ceph-osd", installer.VersionMaster)
	require.Nil(s.T(), err)

	s.k8sh.WaitForLabeledPodsToRun("app=rook-ceph-osd", s.namespace)
	logger.Infof("Done with automatic upgrade to master")

	// Give a few seconds for the daemons to settle down after the upgrade
	time.Sleep(5 * time.Second)

	// Test writing and reading from the pod with cephfs mounted that was created before the upgrade.
	postFilename := "post-upgrade-file"
	assert.Nil(s.T(), s.k8sh.ReadFromPod(s.namespace, filePodName, preFilename, message))
	assert.Nil(s.T(), s.k8sh.WriteToPod(s.namespace, filePodName, postFilename, message))
	assert.Nil(s.T(), s.k8sh.ReadFromPod(s.namespace, filePodName, postFilename, message))

	// Test writing and reading from the pod with rbd mounted that was created before the upgrade.
	// There is some unreliability right after the upgrade when there is only one osd, so we will retry if needed
	assert.Nil(s.T(), s.k8sh.ReadFromPodRetry("", podName, preFilename, message, 3))
	assert.Nil(s.T(), s.k8sh.WriteToPodRetry("", podName, postFilename, message, 3))
	assert.Nil(s.T(), s.k8sh.ReadFromPodRetry("", podName, postFilename, message, 3))
}

func (s *UpgradeSuite) waitForLegacyMonReplicaSetDeletion() error {
	// Wait for the legacy mon replicasets to be deleted during the upgrade
	sleepTime := 3
	attempts := 30
	for i := 0; i < attempts; i++ {
		rs, err := s.k8sh.Clientset.ExtensionsV1beta1().ReplicaSets(s.namespace).List(metav1.ListOptions{LabelSelector: "app=rook-ceph-mon"})
		if err != nil {
			return fmt.Errorf("failed to list mon replicasets. %v", err)
		}

		matches := 0
		for _, r := range rs.Items {
			// a legacy mon replicaset will have two dashes (rook-ceph-mon0) and a new mon replicaset will have four (rook-ceph-mon -a-66d5468994)
			if strings.Count(r.Name, "-") == 2 {
				matches++
			}
		}

		if matches == 0 {
			logger.Infof("all %d replicasets were deleted", len(rs.Items))
			break
		}

		logger.Infof("%d legacy mon replicasets still exist", matches)
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}

	return nil
}
