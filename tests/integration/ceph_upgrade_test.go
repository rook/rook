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

	rookcephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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
func TestCephUpgradeSuite(t *testing.T) {
	if installer.SkipTestSuite(installer.CephTestSuite) {
		t.Skip()
	}

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
	mons := 3
	rbdMirrorWorkers := 0
	cephVersion := rookcephv1.CephVersionSpec{
		// Ceph v13.2.3 got ceph-volume features needed for provisioning
		// test that when Rook upgrades, it can still run non-ceph-volume osds
		Image: "ceph/ceph:v13.2.0-20190410",
	}
	s.op, s.k8sh = StartTestCluster(s.T,
		upgradeMinimalTestVersion,
		s.namespace,
		"",
		false,
		useDevices,
		mons,
		rbdMirrorWorkers,
		installer.Version1_0,
		cephVersion,
	)
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
	setupBlockLite(s.helper, s.k8sh, s.Suite, s.namespace, poolName, storageClassName, blockName, podName, s.op.installer.CephVersion)
	createPodWithBlock(s.helper, s.k8sh, s.Suite, s.namespace, blockName, podName)
	defer blockTestDataCleanUp(s.helper, s.k8sh, s.Suite, s.namespace, poolName, storageClassName, blockName, podName)

	logger.Infof("Initializing file before the upgrade")
	filesystemName := "upgrade-test-fs"
	createFilesystem(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)
	createFilesystemConsumerPod(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)
	defer func() {
		cleanupFilesystemConsumer(s.k8sh, s.Suite, s.namespace, filePodName)
		cleanupFilesystem(s.helper, s.k8sh, s.Suite, s.namespace, filesystemName)
	}()

	logger.Infof("Initializing object before the upgrade")
	objectStoreName := "upgraded-object"
	runObjectE2ETestLite(s.helper, s.k8sh, s.Suite, s.namespace, objectStoreName, 1)

	// verify that we're actually running the right pre-upgrade image
	operatorContainer := "rook-ceph-operator"
	version, err := k8sutil.GetDeploymentImage(s.k8sh.Clientset, systemNamespace, operatorContainer, operatorContainer)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "rook/ceph:"+installer.Version1_0, version)

	message := "my simple message"
	preFilename := "pre-upgrade-file"
	assert.NoError(s.T(), s.k8sh.WriteToPod("", podName, preFilename, message))
	assert.NoError(s.T(), s.k8sh.ReadFromPod("", podName, preFilename, message))
	assert.NoError(s.T(), s.k8sh.WriteToPod(s.namespace, filePodName, preFilename, message))
	assert.NoError(s.T(), s.k8sh.ReadFromPod(s.namespace, filePodName, preFilename, message))

	// Gather logs before Ceph upgrade to help with debugging
	if installer.Env.Logs == "all" {
		s.k8sh.PrintPodDescribe(s.namespace)
	}
	n := strings.Replace(s.T().Name(), "/", "_", -1) + "_before_ceph_upgrade"
	s.op.installer.GatherAllRookLogs(n, systemNamespace, s.namespace)

	// Get some info about the currently deployed mons to determine later if they are all updated
	monDepList, err := k8sutil.GetDeployments(s.k8sh.Clientset, s.namespace, "app=rook-ceph-mon")
	require.NoError(s.T(), err)
	numMons := len(monDepList.Items)

	// Get some info about the currently deployed OSDs to determine later if they are all updated
	osdDepList, err := k8sutil.GetDeployments(s.k8sh.Clientset, s.namespace, "app=rook-ceph-osd")
	require.NoError(s.T(), err)
	osdDeps := osdDepList.Items
	numOSDs := len(osdDeps) // there should be this many upgraded OSDs
	require.True(s.T(), numOSDs > 0)
	d := osdDeps[0]
	oldRookVersion := d.Labels["rook-version"] // upgraded OSDs should not have this version label
	oldCephVersion := d.Labels["ceph-version"] // upgraded OSDs should not have this version label

	// Update to the next version of cluster roles before the operator is restarted
	err = s.updateClusterRoles()
	require.NoError(s.T(), err)

	// Upgrade Ceph version
	s.k8sh.Kubectl("-n", s.namespace, "patch", "CephCluster", s.namespace, "--type=merge",
		"-p", fmt.Sprintf(`{"spec": {"cephVersion": {"image": "%s"}}}`, installer.NautilusVersion.Image))

	// we need to make sure Ceph is fully updated (including RGWs and MDSes) before proceeding to
	// upgrade rook; we do not support upgrading Ceph simultaneously with Rook upgrade
	monsNotOldVersion := fmt.Sprintf("app=rook-ceph-mon,ceph-version!=%s", oldCephVersion)
	err = s.k8sh.WaitForDeploymentCount(monsNotOldVersion, s.namespace, numMons)
	require.NoError(s.T(), err)
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(monsNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	osdsNotOldVersion := fmt.Sprintf("app=rook-ceph-osd,ceph-version!=%s", oldCephVersion)
	err = s.k8sh.WaitForDeploymentCount(osdsNotOldVersion, s.namespace, numOSDs)
	require.NoError(s.T(), err)
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(osdsNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	mdsesNotOldVersion := fmt.Sprintf("app=rook-ceph-mds,ceph-version!=%s", oldCephVersion)
	err = s.k8sh.WaitForDeploymentCount(mdsesNotOldVersion, s.namespace, 4 /* always expect 4 mdses */)
	require.NoError(s.T(), err)
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(mdsesNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	rgwsNotOldVersion := fmt.Sprintf("app=rook-ceph-rgw,ceph-version!=%s", oldCephVersion)
	err = s.k8sh.WaitForDeploymentCount(rgwsNotOldVersion, s.namespace, 1 /* always expect 1 rgw */)
	require.NoError(s.T(), err)
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(rgwsNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	// Gather logs after Ceph upgrade to help with debugging
	if installer.Env.Logs == "all" {
		s.k8sh.PrintPodDescribe(s.namespace)
	}
	n = strings.Replace(s.T().Name(), "/", "_", -1) + "_after_ceph_upgrade"
	s.op.installer.GatherAllRookLogs(n, systemNamespace, s.namespace)

	// Upgrade Rook to master
	require.NoError(s.T(), s.k8sh.SetDeploymentVersion(systemNamespace, operatorContainer, operatorContainer, installer.VersionMaster))

	// verify that the operator spec is updated
	version, err = k8sutil.GetDeploymentImage(s.k8sh.Clientset, systemNamespace, operatorContainer, operatorContainer)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "rook/ceph:"+installer.VersionMaster, version)

	// wait for the mon pods to be running. we can no longer check a version in the pod spec, so we just wait a bit.
	time.Sleep(15 * time.Second)

	monsNotOldVersion = fmt.Sprintf("app=rook-ceph-mon,rook-version!=%s", oldRookVersion)
	err = s.k8sh.WaitForDeploymentCount(monsNotOldVersion, s.namespace, numMons)
	require.NoError(s.T(), err)
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(monsNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	// wait for the osd pods to be updated
	osdsNotOldVersion = fmt.Sprintf("app=rook-ceph-osd,rook-version!=%s", oldRookVersion)
	err = s.k8sh.WaitForDeploymentCount(osdsNotOldVersion, s.namespace, numOSDs)
	require.NoError(s.T(), err)
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(osdsNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	mdsesNotOldVersion = fmt.Sprintf("app=rook-ceph-mds,rook-version!=%s", oldRookVersion)
	err = s.k8sh.WaitForDeploymentCount(mdsesNotOldVersion, s.namespace, 4 /* always expect 4 mdses */)
	require.NoError(s.T(), err)
	err = s.k8sh.WaitForLabeledDeploymentsToBeReady(mdsesNotOldVersion, s.namespace)
	require.NoError(s.T(), err)

	logger.Infof("Done with automatic upgrade to master")

	// Give a few seconds for the daemons to settle down after the upgrade
	time.Sleep(5 * time.Second)

	// wait for filesystem to be active
	err = waitForFilesystemActive(s.k8sh, s.namespace, filesystemName)
	require.NoError(s.T(), err)

	// Test writing and reading from the pod with cephfs mounted that was created before the upgrade.
	postFilename := "post-upgrade-file"
	assert.NoError(s.T(), s.k8sh.ReadFromPodRetry(s.namespace, filePodName, preFilename, message, 5))
	assert.NoError(s.T(), s.k8sh.WriteToPodRetry(s.namespace, filePodName, postFilename, message, 5))
	assert.NoError(s.T(), s.k8sh.ReadFromPodRetry(s.namespace, filePodName, postFilename, message, 5))

	// Test writing and reading from the pod with rbd mounted that was created before the upgrade.
	// There is some unreliability right after the upgrade when there is only one osd, so we will retry if needed
	assert.NoError(s.T(), s.k8sh.ReadFromPodRetry("", podName, preFilename, message, 5))
	assert.NoError(s.T(), s.k8sh.WriteToPodRetry("", podName, postFilename, message, 5))
	assert.NoError(s.T(), s.k8sh.ReadFromPodRetry("", podName, postFilename, message, 5))
}

// Update the clusterroles that have been modified in master from the previous release
func (s *UpgradeSuite) updateClusterRoles() error {
	logger.Infof("Placeholder: create the new resources that have been added since 1.0")
	namespace := s.namespace
	newResources := `
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
rules:
- apiGroups:
  - ""
  resources:
  - pods
  - configmaps
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - delete
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-cmd-reporter
subjects:
- kind: ServiceAccount
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rook-ceph-cmd-reporter-psp
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
- kind: ServiceAccount
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-osd
rules:
- apiGroups:
  - ""
  resources:
  - nodes
  verbs:
  - get
  - list
---
# Allow the ceph osd to access cluster-wide resources necessary for determining their topology location
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-osd
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-osd
subjects:
- kind: ServiceAccount
  name: rook-ceph-osd
  namespace: ` + namespace + `
`
	logger.Infof("creating the new resources that have been added since 1.0")
	return s.k8sh.ResourceOperation("apply", newResources)
}
