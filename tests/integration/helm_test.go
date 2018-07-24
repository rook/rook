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

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/suite"
)

var (
	hslogger = capnslog.NewPackageLogger("github.com/rook/rook", "helmSmokeTest")
)

// ***************************************************
// *** Major scenarios tested by the TestHelmSuite ***
// Setup
// - A cluster created via the Helm chart
// Monitors
// - One mon
// OSDs
// - Bluestore running on a directory
// Block
// - Create a pool in each cluster
// - Mount/unmount a block device through the dynamic provisioner
// File system
// - Create a file system via the CRD
// Object
// - Create the object store via the CRD
// ***************************************************
func TestHelmSuite(t *testing.T) {
	s := new(HelmSuite)
	defer func(s *HelmSuite) {
		HandlePanics(recover(), s.op, s.T)
	}(s)
	suite.Run(t, s)
}

type HelmSuite struct {
	suite.Suite
	helper    *clients.TestClient
	kh        *utils.K8sHelper
	op        contracts.Setup
	namespace string
}

func (hs *HelmSuite) SetupSuite() {
	hs.namespace = "helm-ns"
	hs.op, hs.kh = StartBaseTestOperations(hs.T, hs.namespace, "bluestore", true, false, 1)
	hs.helper = GetTestClient(hs.kh, hs.namespace, hs.op, hs.T)
}

func (hs *HelmSuite) TearDownSuite() {
	hs.op.TearDown()
}

//Test to make sure all rook components are installed and Running
func (hs *HelmSuite) TestRookInstallViaHelm() {
	checkIfRookClusterIsInstalled(hs.Suite, hs.kh, hs.namespace, hs.namespace, 1)
}

//Test BlockCreation on Rook that was installed via Helm
func (hs *HelmSuite) TestBlockStoreOnRookInstalledViaHelm() {
	runBlockE2ETestLite(hs.helper, hs.kh, hs.Suite, hs.namespace)
}

//Test File System Creation on Rook that was installed via helm
func (hs *HelmSuite) TestFileStoreOnRookInstalledViaHelm() {
	runFileE2ETestLite(hs.helper, hs.kh, hs.Suite, hs.namespace, "testfs")
}

//Test Object StoreCreation on Rook that was installed via helm
func (hs *HelmSuite) TestObjectStoreOnRookInstalledViaHelm() {
	runObjectE2ETestLite(hs.helper, hs.kh, hs.Suite, hs.namespace, "default", 3)
}
