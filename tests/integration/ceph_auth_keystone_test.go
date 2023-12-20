package integration

import (
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"testing"

	"github.com/stretchr/testify/suite"
)

// ***************************************************
// *** Major scenarios tested by the TestKeystoneAuthSuite ***
// Setup
// - An CephObject store will be created in a test cluster
// Access
// - Create a container
// - Put a file in a container
// - Get a file from the container
// - Try to get the file without valid credentials
// ***************************************************
func TestCephKeystoneAuthSuite(t *testing.T) {
	s := new(KeystoneAuthSuite)
	defer func(s *KeystoneAuthSuite) {
		HandlePanics(recover(), s.TearDownSuite, s.T)
	}(s)
	suite.Run(t, s)
}

type KeystoneAuthSuite struct {
	suite.Suite
	helper    *clients.TestClient
	installer *installer.CephInstaller
	settings  *installer.TestCephSettings
	k8shelper *utils.K8sHelper
}

func (h *KeystoneAuthSuite) SetupSuite() {
	namespace := "keystoneauth-ns"
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
		CephVersion:               installer.QuincyVersion,
		SkipClusterCleanup:        false,
		SkipCleanupPolicy:         false,
	}
	h.settings.ApplyEnvVars()
	h.installer, h.k8shelper = StartTestCluster(h.T, h.settings)

	// install yaook-keystone here
	InstallKeystoneInTestCluster(h.k8shelper)

	h.helper = clients.CreateTestClient(h.k8shelper, h.installer.Manifests)
}

func (h *KeystoneAuthSuite) TearDownSuite() {
	h.installer.UninstallRook()
	// TODO: cleanup yaook-keystone here
	CleanUpKeystoneInTestCluster(h.k8shelper)
}

func (h *KeystoneAuthSuite) AfterTest(suiteName, testName string) {
	h.installer.CollectOperatorLog(suiteName, testName)
}

// Test Object StoreCreation on Rook that was installed via helm
func (h *KeystoneAuthSuite) TestObjectStoreOnRookInstalledViaHelmUsingKeystone() {
	deleteStore := true
	tls := false
	swiftAndKeystone := true
	// TODO: Find out whether this is enough or whether there are other objectstore related tests
	runObjectE2ETestLite(h.T(), h.helper, h.k8shelper, h.installer, h.settings.Namespace, "default", 3, deleteStore, tls, swiftAndKeystone)
}
