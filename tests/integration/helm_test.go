package integration

import (
	"testing"

	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
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
// - Create a file system via the REST API
// Object
// - Create the object store via the CRD
// ***************************************************
func TestHelmSuite(t *testing.T) {
	s := new(HelmSuite)
	defer func(s *HelmSuite) {
		HandlePanics(recover(), s.o, s.T)
	}(s)
	suite.Run(t, s)
}

type HelmSuite struct {
	suite.Suite
	helper    *clients.TestClient
	k8sh      *utils.K8sHelper
	installer *installer.InstallHelper
	hh        *utils.HelmHelper
	o         contracts.TestOperator
	namespace string
}

func (hs *HelmSuite) SetupSuite() {
	hs.namespace = "helm-ns"
	kh, err := utils.CreateK8sHelper(hs.T)
	require.NoError(hs.T(), err)

	hs.k8sh = kh
	hs.hh = utils.NewHelmHelper()

	hs.installer = installer.NewK8sRookhelper(kh.Clientset, hs.T)
	hs.o = NewBaseTestOperations(hs.installer, hs.T, hs.namespace, true)

	err = hs.installer.CreateK8sRookOperatorViaHelm(hs.namespace)
	require.NoError(hs.T(), err)

	require.True(hs.T(), kh.IsPodInExpectedState("rook-operator", hs.namespace, "Running"),
		"Make sure rook-operator is in running state")

	require.True(hs.T(), kh.IsPodInExpectedState("rook-agent", hs.namespace, "Running"),
		"Make sure rook-agent is in running state")

	time.Sleep(10 * time.Second)

	err = hs.installer.CreateK8sRookCluster(hs.namespace, "bluestore")
	require.NoError(hs.T(), err)

	err = hs.installer.CreateK8sRookToolbox(hs.namespace)
	require.NoError(hs.T(), err)

	hs.helper, err = clients.CreateTestClient(kh, hs.namespace)
	require.Nil(hs.T(), err)
}

func (hs *HelmSuite) TearDownSuite() {
	hs.o.TearDown()
}

//Test to make sure all rook components are installed and Running
func (hs *HelmSuite) TestRookInstallViaHelm() {
	checkIfRookClusterIsInstalled(hs.Suite, hs.k8sh, hs.namespace, hs.namespace, 1)
}

//Test BlockCreation on Rook that was installed via Helm
func (hs *HelmSuite) TestBlockStoreOnRookInstalledViaHelm() {
	runBlockE2ETestLite(hs.helper, hs.k8sh, hs.Suite, hs.namespace)
}

//Test File System Creation on Rook that was installed via helm
func (hs *HelmSuite) TestFileStoreOnRookInstalledViaHelm() {
	runFileE2ETestLite(hs.helper, hs.k8sh, hs.Suite, hs.namespace, "testfs")
}

//Test Object StoreCreation on Rook that was installed via helm
func (hs *HelmSuite) TestObjectStoreOnRookInstalledViaHelm() {
	runObjectE2ETestLite(hs.helper, hs.k8sh, hs.Suite, hs.namespace, "default", 3)
}
