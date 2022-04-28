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
	"testing"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/suite"
)

// ***************************************************
// *** Major scenarios tested by the TestHelmSuite ***
// Setup
// - A cluster created via the Helm chart
// Monitors
// - One mon
// OSDs
// - Bluestore running on a raw block device
// Block
// - Create a pool in the cluster
// - Mount/unmount a block device through the dynamic provisioner
// File system
// - Create a file system via the CRD
// Object
// - Create the object store via the CRD
// ***************************************************
func TestCephHelmSuite(t *testing.T) {
	s := new(HelmSuite)
	defer func(s *HelmSuite) {
		HandlePanics(recover(), s.TearDownSuite, s.T)
	}(s)
	suite.Run(t, s)
}

type HelmSuite struct {
	suite.Suite
	helper    *clients.TestClient
	installer *installer.CephInstaller
	settings  *installer.TestCephSettings
	k8shelper *utils.K8sHelper
}

func (h *HelmSuite) SetupSuite() {
	namespace := "helm-ns"
	h.settings = &installer.TestCephSettings{
		Namespace:                 namespace,
		OperatorNamespace:         namespace,
		StorageClassName:          "",
		UseHelm:                   true,
		UsePVC:                    false,
		Mons:                      1,
		SkipOSDCreation:           false,
		EnableAdmissionController: true,
		EnableDiscovery:           true,
		ChangeHostName:            true,
		ConnectionsEncrypted:      true,
		RookVersion:               installer.LocalBuildTag,
		CephVersion:               installer.PacificVersion,
	}
	h.settings.ApplyEnvVars()
	h.installer, h.k8shelper = StartTestCluster(h.T, h.settings)
	h.helper = clients.CreateTestClient(h.k8shelper, h.installer.Manifests)
}

func (h *HelmSuite) TearDownSuite() {
	h.installer.UninstallRook()
}

func (h *HelmSuite) AfterTest(suiteName, testName string) {
	h.installer.CollectOperatorLog(suiteName, testName)
}

// Test to make sure all rook components are installed and Running
func (h *HelmSuite) TestARookInstallViaHelm() {
	checkIfRookClusterIsInstalled(h.Suite, h.k8shelper, h.settings.Namespace, h.settings.Namespace, 1)
	checkIfRookClusterHasHealthyIngress(h.Suite, h.k8shelper, h.settings.Namespace)
}

// Test BlockCreation on Rook that was installed via Helm
func (h *HelmSuite) TestBlockStoreOnRookInstalledViaHelm() {
	runBlockCSITestLite(h.helper, h.k8shelper, h.Suite, h.settings)
}

// Test File System Creation on Rook that was installed via helm
func (h *HelmSuite) TestFileStoreOnRookInstalledViaHelm() {
	runFileE2ETestLite(h.helper, h.k8shelper, h.Suite, h.settings, "testfs")
}

// Test Object StoreCreation on Rook that was installed via helm
func (h *HelmSuite) TestObjectStoreOnRookInstalledViaHelm() {
	deleteStore := true
	tls := false
	runObjectE2ETestLite(h.T(), h.helper, h.k8shelper, h.installer, h.settings.Namespace, "default", 3, deleteStore, tls)
}
