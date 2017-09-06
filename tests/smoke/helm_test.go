package smoke

import (
	"strings"
	"testing"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/enums"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"time"
)

var (
	hslogger = capnslog.NewPackageLogger("github.com/rook/rook", "helmSmokeTest")
)

func TestHelmSmokeSuite(t *testing.T) {
	suite.Run(t, new(HelmSmokeSuite))
}

type HelmSmokeSuite struct {
	suite.Suite
	helper          *clients.TestClient
	k8sh            *utils.K8sHelper
	installer       *installer.InstallHelper
	hh              *utils.HelmHelper
	pvcDef          string
	storageclassDef string
}

func (hs *HelmSmokeSuite) SetupSuite() {
	kh, err := utils.CreateK8sHelper()
	require.NoError(hs.T(), err)

	hs.k8sh = kh
	hs.hh = utils.NewHelmHelper()

	hs.installer = installer.NewK8sRookhelper(kh.Clientset)

	err = hs.installer.CreateK8sRookOperatorViaHelm()
	require.NoError(hs.T(), err)

	require.True(hs.T(), kh.IsPodInExpectedState("rook-operator", "rook", "Running"),
		"Make sure rook-operator is in running state")

	time.Sleep(10 * time.Second)

	err = hs.installer.CreateK8sRookCluster()
	require.NoError(hs.T(), err)

	err = hs.installer.CreateK8sRookToolbox()
	require.NoError(hs.T(), err)

	hs.helper, err = clients.CreateTestClient(enums.Kubernetes, kh)
	require.Nil(hs.T(), err)

	hs.pvcDef = `apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{.claimName}}
  annotations:
    volume.beta.kubernetes.io/storage-class: rook-block
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1M`

	hs.storageclassDef = `apiVersion: rook.io/v1alpha1
kind: Pool
metadata:
  name: {{.poolName}}
  namespace: rook
spec:
  replication:
    size: 1
  # For an erasure-coded pool, comment out the replication count above and uncomment the following settings.
  # Make sure you have enough OSDs to support the replica count or erasure code chunks.
  #erasureCode:
  #  codingChunks: 2
  #  dataChunks: 2
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: rook-block
provisioner: rook.io/block
parameters:
    pool: {{.poolName}}`
}

func (hs *HelmSmokeSuite) TearDownSuite() {
	if hs.T().Failed() {
		hs.k8sh.GetRookLogs("rook-operator", "default", hs.T().Name())
		hs.k8sh.GetRookLogs("rook-api", "rook", hs.T().Name())
		hs.k8sh.GetRookLogs("rook-ceph-mgr", "rook", hs.T().Name())
		hs.k8sh.GetRookLogs("rook-ceph-mon", "rook", hs.T().Name())
		hs.k8sh.GetRookLogs("rook-ceph-osd", "rook", hs.T().Name())
		hs.k8sh.GetRookLogs("rook-ceph-rgw", "rook", hs.T().Name())
		hs.k8sh.GetRookLogs("rook-ceph-mds", "rook", hs.T().Name())
	}
	hs.installer.UninstallRookFromK8sViaHelm()

}

//Test to make sure all rook components are installed and Running
func (hs *HelmSmokeSuite) TestRookInstallViaHelm() {

	assert.True(hs.T(), hs.k8sh.CheckPodCountAndState("rook-operator", "default", 1, "Running"),
		"Make sure there is 1 rook-operator present in Running state")
	assert.True(hs.T(), hs.k8sh.CheckPodCountAndState("rook-api", "rook", 1, "Running"),
		"Make sure there is 1 rook-api present in Running state")
	assert.True(hs.T(), hs.k8sh.CheckPodCountAndState("rook-ceph-mgr", "rook", 1, "Running"),
		"Make sure there is 1 rook-ceph-mgr present in Running state")
	assert.True(hs.T(), hs.k8sh.CheckPodCountAndState("rook-ceph-osd", "rook", 1, "Running"),
		"Make sure there is at lest 1 rook-ceph-osd present in Running state")
	assert.True(hs.T(), hs.k8sh.CheckPodCountAndState("rook-ceph-mon", "rook", 3, "Running"),
		"Make sure there are 3 rook-ceph-mon present in Running state")
}

//Test BlockCreation on Rook that was installed via Helm charts
func (hs *HelmSmokeSuite) TestBlockStoreOnRookInstalledViaHelm() {

	//Check initial number of blocks
	bc := hs.helper.GetBlockClient()
	initialBlocks, err := bc.BlockList()
	require.Nil(hs.T(), err)
	initBlockCount := len(initialBlocks)

	//deploy storageclass
	sc := map[string]string{
		"poolName": "helm-pool",
	}

	res1, err := hs.k8sh.ResourceOperationFromTemplate("create", hs.storageclassDef, sc)
	require.Contains(hs.T(), res1, "pool \"helm-pool\" created", "Make sure test pool is created")
	require.Contains(hs.T(), res1, "storageclass \"rook-block\" created", "Make sure storageclass is created")
	require.NoError(hs.T(), err)
	// see https://github.com/rook/rook/issues/767
	time.Sleep(10 * time.Second)

	// deploy pvc
	vc := map[string]string{
		"claimName": "helm-claim",
	}
	res2, err := hs.k8sh.ResourceOperationFromTemplate("create", hs.pvcDef, vc)
	require.Contains(hs.T(), res2, "persistentvolumeclaim \"helm-claim\" created", "Make sure pvc is created")
	require.NoError(hs.T(), err)

	require.True(hs.T(), hs.isPVCBound("helm-claim"))

	//Make sure  new block is created
	b, _ := bc.BlockList()
	require.Equal(hs.T(), initBlockCount+1, len(b), "Make sure new block image is created")

	//Delete pvc and storageclass
	_, err = hs.k8sh.ResourceOperationFromTemplate("delete", hs.storageclassDef, sc)
	require.NoError(hs.T(), err)
	_, err = hs.k8sh.ResourceOperationFromTemplate("delete", hs.pvcDef, vc)
	require.NoError(hs.T(), err)
	time.Sleep(2 * time.Second)

	b, _ = bc.BlockList()
	require.Equal(hs.T(), initBlockCount, len(b), "Make sure new block image is deleted")

}

//Test File System Creation on Rook that was installed via Helm charts
func (hs *HelmSmokeSuite) TestFileStoreOnRookInstalledViaHelm() {
	fc := hs.helper.GetFileSystemClient()

	//Create file System
	_, fscErr := fc.FSCreate(fileSystemName)
	require.Nil(hs.T(), fscErr)
	fileSystemList, _ := fc.FSList()
	require.Equal(hs.T(), 1, len(fileSystemList), "There should one shared file system present")

	//Make sure rook-ceph-mds pods are running
	require.True(hs.T(), hs.k8sh.IsPodInExpectedState("rook-ceph-mds", "rook", "Running"),
		"Make sure rook-operator is in running state")

	assert.True(hs.T(), hs.k8sh.CheckPodCountAndState("rook-ceph-mds", "rook", 1, "Running"),
		"Make sure there is 1 rook-operator present in Running state")

}

//Test Object StoreCreation on Rook that was installed via Helm charts
func (hs *HelmSmokeSuite) TestObjectStoreOnRookInstalledViaHelm() {
	//Create Object store
	oc := hs.helper.GetObjectClient()
	oc.ObjectCreate()

	//Make sure rook-ceph-rgw pods are running
	require.True(hs.T(), hs.k8sh.IsPodInExpectedState("rook-ceph-rgw", "rook", "Running"),
		"Make sure rook-operator is in running state")

	assert.True(hs.T(), hs.k8sh.CheckPodCountAndState("rook-ceph-rgw", "rook", 2, "Running"),
		"Make sure there is 1 rook-operator present in Running state")

	require.True(hs.T(), hs.k8sh.IsServiceUpInNameSpace("rook-ceph-rgw"))

}

func (hs *HelmSmokeSuite) isPVCBound(name string) bool {
	inc := 0
	for inc < utils.RetryLoop {
		status, _ := hs.k8sh.GetPVCStatus(name)
		if strings.TrimRight(status, "\n") == "'Bound'" {
			return true
		}
		time.Sleep(time.Second * utils.RetryInterval)
		inc++

	}
	return false
}
