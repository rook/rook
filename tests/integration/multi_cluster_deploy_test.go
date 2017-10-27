package integration

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// *************************************************************
// *** Major scenarios tested by the MultiClusterDeploySuite ***
// Setup
// - Two clusters started in different namespaces via the CRD
// Monitors
// - One mon in each cluster
// OSDs
// - Bluestore running on a directory
// Block
// - Create a pool in each cluster
// - Mount/unmount a block device through the dynamic provisioner
// File system
// - Create a file system via the REST API
// Object
// - Create the object store via the CRD
// *************************************************************
func TestMultiClusterDeploySuite(t *testing.T) {
	suite.Run(t, new(MultiClusterDeploySuite))
}

type MultiClusterDeploySuite struct {
	suite.Suite
	helper1     *clients.TestClient
	helper2     *clients.TestClient
	k8sh        *utils.K8sHelper
	installer   *installer.InstallHelper
	installData *installer.InstallData
	namespace1  string
	namespace2  string
}

//Deploy Multiple Rook clusters
func (mrc *MultiClusterDeploySuite) SetupSuite() {

	mrc.namespace1 = "mrc-n1"
	mrc.namespace2 = "mrc-n2"

	kh, err := utils.CreateK8sHelper(mrc.T)
	require.NoError(mrc.T(), err)

	mrc.k8sh = kh
	mrc.installer = installer.NewK8sRookhelper(kh.Clientset, mrc.T)
	mrc.installData = installer.NewK8sInstallData()

	err = mrc.installer.CreateK8sRookOperator(installer.SystemNamespace(mrc.namespace1))
	require.NoError(mrc.T(), err)

	require.True(mrc.T(), kh.IsPodInExpectedState("rook-operator", installer.SystemNamespace(mrc.namespace1), "Running"),
		"Make sure rook-operator is in running state")

	require.True(mrc.T(), kh.IsPodInExpectedState("rook-agent", installer.SystemNamespace(mrc.namespace1), "Running"),
		"Make sure rook-agent is in running state")

	time.Sleep(10 * time.Second)

	// start the two clusters in parallel
	logger.Infof("starting two clusters in parallel")
	errCh1 := make(chan error, 1)
	errCh2 := make(chan error, 1)
	go mrc.startCluster(mrc.namespace1, "bluestore", errCh1)
	go mrc.startCluster(mrc.namespace2, "filestore", errCh2)
	require.NoError(mrc.T(), <-errCh1)
	require.NoError(mrc.T(), <-errCh2)
	logger.Infof("finished starting clusters")

	mrc.helper1, err = clients.CreateTestClient(mrc.k8sh, mrc.namespace1)
	require.Nil(mrc.T(), err)

	mrc.helper2, err = clients.CreateTestClient(mrc.k8sh, mrc.namespace2)
	require.Nil(mrc.T(), err)

	// create a test pool in each cluster so that we get some PGs
	_, err = mrc.helper1.GetPoolClient().PoolCreate(model.Pool{
		Name:             "multi-cluster-install-pool1",
		ReplicatedConfig: model.ReplicatedPoolConfig{Size: 1}})
	require.Nil(mrc.T(), err)

	_, err = mrc.helper2.GetPoolClient().PoolCreate(model.Pool{
		Name:             "multi-cluster-install-pool2",
		ReplicatedConfig: model.ReplicatedPoolConfig{Size: 1}})
	require.Nil(mrc.T(), err)
}

func (mrc *MultiClusterDeploySuite) startCluster(namespace, store string, errCh chan error) {
	logger.Infof("starting cluster %s", namespace)
	if err := mrc.installer.CreateK8sRookCluster(namespace, store); err != nil {
		errCh <- fmt.Errorf("failed to create cluster %s. %+v", namespace, err)
		return
	}

	if err := mrc.installer.CreateK8sRookToolbox(namespace); err != nil {
		errCh <- fmt.Errorf("failed to create toolbox for %s. %+v", namespace, err)
		return
	}
	logger.Infof("succeeded starting cluster %s", namespace)
	errCh <- nil
}

func (mrc *MultiClusterDeploySuite) TearDownSuite() {
	if mrc.T().Failed() {
		gatherAllRookLogs(mrc.k8sh, mrc.Suite, mrc.installer.Env.HostType, installer.SystemNamespace(mrc.namespace1), mrc.namespace1)
		gatherAllRookLogs(mrc.k8sh, mrc.Suite, mrc.installer.Env.HostType, installer.SystemNamespace(mrc.namespace1), mrc.namespace2)
	}
	deleteArgs := []string{"delete", "-f", "-"}

	skipRookInstall := strings.EqualFold(mrc.installer.Env.SkipInstallRook, "true")
	if skipRookInstall {
		return
	}

	logger.Infof("Uninstalling All Rook Clusters")
	k8sHelp, err := utils.CreateK8sHelper(mrc.T)
	if err != nil {
		panic(err)
	}
	rookOperator := mrc.installData.GetRookOperator(installer.SystemNamespace(mrc.namespace1))

	//Delete rook operator
	_, err = mrc.k8sh.KubectlWithStdin(rookOperator, deleteArgs...)
	if err != nil {
		logger.Errorf("Rook operator cannot be deleted,err -> %v", err)
		panic(err)
	}

	//delete rook cluster
	mrc.installer.CleanupCluster(mrc.namespace1)
	mrc.installer.CleanupCluster(mrc.namespace2)

	// Delete crd/tpr
	if mrc.k8sh.VersionAtLeast("v1.7.0") {
		_, err = k8sHelp.DeleteResource([]string{"crd", "clusters.rook.io", "pools.rook.io", "objectstores.rook.io"})
		if err != nil {
			logger.Errorf("crd cannot be deleted")
			panic(err)
		}
	} else {
		_, err = k8sHelp.DeleteResource([]string{"thirdpartyresources", "cluster.rook.io", "pool.rook.io", "objectstore.rook.io"})
		if err != nil {
			logger.Errorf("tpr cannot be deleted")
			panic(err)
		}
	}

	mrc.k8sh.Clientset.RbacV1beta1().ClusterRoleBindings().Delete("anon-user-access", nil)
	logger.Infof("Rook clusters %s  and  %s uninstalled", mrc.namespace1, mrc.namespace2)
}

//Test to make sure all rook components are installed and Running
func (mrc *MultiClusterDeploySuite) TestInstallingMultipleRookClusters() {
	//Check if Rook cluster 1 is deployed successfully
	checkIfRookClusterIsInstalled(mrc.Suite, mrc.k8sh, installer.SystemNamespace(mrc.namespace1), mrc.namespace1, 1)
	checkIfRookClusterIsHealthy(mrc.Suite, mrc.helper1, mrc.namespace1)

	//Check if Rook cluster 2 is deployed successfully
	checkIfRookClusterIsInstalled(mrc.Suite, mrc.k8sh, installer.SystemNamespace(mrc.namespace1), mrc.namespace2, 1)
	checkIfRookClusterIsHealthy(mrc.Suite, mrc.helper2, mrc.namespace2)
}

//Test Block Store Creation on multiple rook clusters
func (mrc *MultiClusterDeploySuite) TestBlockStoreOnMultipleRookCluster() {
	runBlockE2ETestLite(mrc.helper1, mrc.k8sh, mrc.Suite, mrc.namespace1)
	runBlockE2ETestLite(mrc.helper2, mrc.k8sh, mrc.Suite, mrc.namespace2)
}

//Test Filesystem Creation on multiple rook clusters
func (mrc *MultiClusterDeploySuite) TestFileStoreOnMultiRookCluster() {
	runFileE2ETestLite(mrc.helper1, mrc.k8sh, mrc.Suite, mrc.namespace1, "test-fs-1")
	runFileE2ETestLite(mrc.helper2, mrc.k8sh, mrc.Suite, mrc.namespace2, "test-fs-2")
}

//Test Object Store Creation on multiple rook clusters
func (mrc *MultiClusterDeploySuite) TestObjectStoreOnMultiRookCluster() {
	runObjectE2ETestLite(mrc.helper1, mrc.k8sh, mrc.Suite, mrc.namespace1, "default-c1", 2)
	runObjectE2ETestLite(mrc.helper2, mrc.k8sh, mrc.Suite, mrc.namespace2, "default-c2", 1)
}
