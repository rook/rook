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

func TestCephNFSSuite(t *testing.T) {
	s := new(NFSSuite)
	defer func(s *NFSSuite) {
		HandlePanics(recover(), s.TearDownSuite, s.T)
	}(s)
	suite.Run(t, s)
}

type NFSSuite struct {
	suite.Suite
	helper    *clients.TestClient
	settings  *installer.TestCephSettings
	installer *installer.CephInstaller
	k8sh      *utils.K8sHelper
}

func (s *NFSSuite) SetupSuite() {
	namespace := "nfs-ns"
	s.settings = &installer.TestCephSettings{
		ClusterName:               "nfs-cluster",
		Namespace:                 namespace,
		OperatorNamespace:         installer.SystemNamespace(namespace),
		StorageClassName:          installer.StorageClassName(),
		UseHelm:                   false,
		UsePVC:                    installer.UsePVC(),
		Mons:                      3,
		SkipOSDCreation:           false,
		EnableAdmissionController: true,
		ConnectionsEncrypted:      true,
		ConnectionsCompressed:     true,
		UseCrashPruner:            true,
		EnableVolumeReplication:   true,
		TestNFSCSI:                true,
		ChangeHostName:            true,
		RookVersion:               installer.LocalBuildTag,
		CephVersion:               installer.ReturnCephVersion(),
		SkipClusterCleanup:        true,
	}
	s.settings.ApplyEnvVars()
	s.installer, s.k8sh = StartTestCluster(s.T, s.settings)
	s.helper = clients.CreateTestClient(s.k8sh, s.installer.Manifests)
}

func (s *NFSSuite) AfterTest(suiteName, testName string) {
	s.installer.CollectOperatorLog(suiteName, testName)
}

func (s *NFSSuite) TearDownSuite() {
	s.installer.UninstallRook()
}

func (s *NFSSuite) TestNFSE2E_NFSTest() {
	runNFSFileE2ETest(s.helper, s.k8sh, &s.Suite, s.settings, "nfs-test")
}
