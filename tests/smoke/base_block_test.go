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

package smoke

import (
	"time"

	"strings"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	blockMountPath = "/tmp/rook1"
	blockPodName   = "block-test"
)

// Smoke Test for Block Storage - Test check the following operations on Block Storage in order
//Create,Mount,Write,Read,Unmount and Delete.
func runBlockE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string) {
	poolName := "replicapool"
	storageClassName := "rook-block"
	blockName := "block-pv-claim"

	defer blockTestDataCleanUp(helper, k8sh, namespace, poolName, storageClassName, blockName)
	logger.Infof("Block Storage End to End Integration Test - create, mount, write to, read from, and unmount")
	logger.Infof("Running on Rook Cluster %s", namespace)
	rbc := helper.GetBlockClient()

	logger.Infof("Step 0 : Get Initial List Block")
	initBlockImages, _ := rbc.BlockList()

	logger.Infof("step 1: Create block storage")
	cbErr := blockStorageOperation(helper, k8sh, namespace, poolName, storageClassName, blockName, "create")
	require.Nil(s.T(), cbErr)
	require.True(s.T(), retryBlockImageCountCheck(helper, k8sh, len(initBlockImages), 1), "Make sure a new block is created")
	logger.Infof("Block Storage created successfully")
	require.True(s.T(), k8sh.WaitUntilPVCIsBound(blockName), "Make sure PVC is Bound")

	logger.Infof("step 2: Mount block storage")
	_, mtErr := rbc.BlockMap(getBlockPodDefintion(blockName), blockMountPath)
	require.Nil(s.T(), mtErr)
	require.True(s.T(), k8sh.IsPodRunning(blockPodName, defaultNamespace), "make sure block-test pod is in running state")
	logger.Infof("Block Storage Mounted successfully")

	logger.Infof("step 3: Write to block storage")
	_, wtErr := rbc.BlockWrite(blockPodName, blockMountPath, "Smoke Test Data form Block storage", "bsFile1", "")
	require.Nil(s.T(), wtErr)
	logger.Infof("Write to Block storage successfully")

	logger.Infof("step 4: Read from  block storage")
	read, rErr := rbc.BlockRead(blockPodName, blockMountPath, "bsFile1", "")
	require.Nil(s.T(), rErr)
	require.Contains(s.T(), read, "Smoke Test Data form Block storage", "make sure content of the files is unchanged")
	logger.Infof("Read from  Block storage successfully")

	logger.Infof("step 5: Unmount block storage")
	_, unmtErr := rbc.BlockUnmap(getBlockPodDefintion(blockName), blockMountPath)
	require.Nil(s.T(), unmtErr)
	require.True(s.T(), k8sh.IsPodTerminated(blockPodName, defaultNamespace), "make sure block-test pod is terminated")
	logger.Infof("Block Storage unmounted successfully")

	logger.Infof("step 6: Deleting block storage")
	dbErr := blockStorageOperation(helper, k8sh, namespace, poolName, storageClassName, blockName, "delete")
	require.Nil(s.T(), dbErr)
	require.True(s.T(), retryBlockImageCountCheck(helper, k8sh, len(initBlockImages), 0), "Make sure a block is deleted")
	logger.Infof("Block Storage deleted successfully")
}

func runBlockE2ETestLite(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, clusterNamespace string) {
	logger.Infof("Block Storage End to End Integration Test - create storageclass,pool and pvc")
	logger.Infof("Running on Rook Cluster %s", clusterNamespace)

	//Check initial number of blocks
	defer blockTestDataCleanUp(helper, k8sh, clusterNamespace, "rookpool", "rook-block", "test-block")
	bc := helper.GetBlockClient()
	initialBlocks, err := bc.BlockList()
	require.Nil(s.T(), err)
	initBlockCount := len(initialBlocks)

	logger.Infof("step 1: Create StorageClass and pool")
	sc := map[string]string{}
	storageclass := getBlockPoolAndStorageClassDefinition(k8sh, clusterNamespace, "rookpool", "rook-block")
	res1, err := k8sh.ResourceOperationFromTemplate("create", storageclass, sc)
	require.Contains(s.T(), res1, "pool \"rookpool\" created", "Make sure test pool is created")
	require.Contains(s.T(), res1, "storageclass \"rook-block\" created", "Make sure storageclass is created")
	require.NoError(s.T(), err)
	// see https://github.com/rook/rook/issues/767
	time.Sleep(10 * time.Second)

	logger.Infof("step 2: Create pvc")
	pvc := getPvcDefinition("test-block-claim", "rook-block")
	res2, err := k8sh.ResourceOperationFromTemplate("create", pvc, sc)
	require.Contains(s.T(), res2, "persistentvolumeclaim \"test-block-claim\" created", "Make sure pvc is created")
	require.NoError(s.T(), err)

	require.True(s.T(), isPVCBound(k8sh, "test-block-claim"))

	//Make sure  new block is created
	b, _ := bc.BlockList()
	require.Equal(s.T(), initBlockCount+1, len(b), "Make sure new block image is created")

	//Delete pvc and storageclass
	_, err = k8sh.ResourceOperationFromTemplate("delete", storageclass, sc)
	require.NoError(s.T(), err)
	_, err = k8sh.ResourceOperationFromTemplate("delete", pvc, sc)
	require.NoError(s.T(), err)
	time.Sleep(2 * time.Second)

	b, _ = bc.BlockList()
	require.Equal(s.T(), initBlockCount, len(b), "Make sure new block image is deleted")

}

func blockTestDataCleanUp(helper *clients.TestClient, k8sh *utils.K8sHelper, namespace string, poolname string, storageclassname string, blockname string) {
	logger.Infof("Cleaning up block storage")
	helper.GetBlockClient().BlockUnmap(getBlockPodDefintion(blockname), blockMountPath)
	blockStorageOperation(helper, k8sh, namespace, poolname, storageclassname, blockname, "delete")
	cleanUpDymanicBlockStorage(helper)
}

func blockStorageOperation(helper *clients.TestClient, k8sh *utils.K8sHelper, namespace string, poolname string, storageclassname string, blockname string, action string) error {
	poolStorageClassDef := getBlockPoolAndStorageClassDefinition(k8sh, namespace, poolname, storageclassname)
	pvcDef := getPvcDefinition(blockname, storageclassname)
	k8sh.ResourceOperation(action, poolStorageClassDef)
	// see https://github.com/rook/rook/issues/767
	time.Sleep(10 * time.Second)
	_, err := k8sh.ResourceOperation(action, pvcDef)

	return err
}

// periodically checking if block image count has changed to expected value
// When creating pvc in k8s platform, it may take some time for the block Image to be bounded
func retryBlockImageCountCheck(helper *clients.TestClient, k8sh *utils.K8sHelper, imageCount, expectedChange int) bool {
	inc := 0
	for inc < utils.RetryLoop {
		logger.Infof("Getting list of blocks (expecting %d)", (imageCount + expectedChange))
		blockImages, _ := helper.GetBlockClient().BlockList()
		if imageCount+expectedChange == len(blockImages) {
			return true
		}
		time.Sleep(time.Second * utils.RetryInterval)
		inc++
	}
	return false
}

//CleanUpDymanicBlockStorage is helper method to clean up bock storage created by tests
func cleanUpDymanicBlockStorage(helper *clients.TestClient) {
	// Delete storage pool, storage class and pvc
	blockImagesList, _ := helper.GetBlockClient().BlockList()
	for _, blockImage := range blockImagesList {
		helper.GetRestAPIClient().DeleteBlockImage(blockImage)
	}

}

func getBlockPoolAndStorageClassDefinition(k8sh *utils.K8sHelper, namespace string, poolName string, storageclassName string) string {
	k8sVersion := k8sh.GetK8sServerVersion()
	if strings.Contains(k8sVersion, "v1.5") {
		return `apiVersion: rook.io/v1alpha1
kind: Pool
metadata:
  name: ` + poolName + `
  namespace: ` + namespace + `
spec:
  replicated:
    size: 1
  # For an erasure-coded pool, comment out the replication size above and uncomment the following settings.
  # Make sure you have enough OSDs to support the replica size or erasure code chunks.
  #erasureCoded:
  #  codingChunks: 2
  #  dataChunks: 2
---
apiVersion: storage.k8s.io/v1beta1
kind: StorageClass
metadata:
   name: ` + storageclassName + `
provisioner: rook.io/block
parameters:
    pool: ` + poolName + `
    clusterName: ` + namespace + `
    clusterNamespace: ` + namespace

	}
	return `apiVersion: rook.io/v1alpha1
kind: Pool
metadata:
  name: ` + poolName + `
  namespace: ` + namespace + `
spec:
  replicated:
    size: 1
  # For an erasure-coded pool, comment out the replication count above and uncomment the following settings.
  # Make sure you have enough OSDs to support the replica count or erasure code chunks.
  #erasureCoded:
  #  codingChunks: 2
  #  dataChunks: 2
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: ` + storageclassName + `
provisioner: rook.io/block
parameters:
    pool: ` + poolName + `
    clusterName: ` + namespace + `
    clusterNamespace: ` + namespace
}

func getPvcDefinition(blockName string, storageclassName string) string {
	return `apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ` + blockName + `
spec:
  storageClassName: ` + storageclassName + `
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1M`
}

func getBlockPodDefintion(blockName string) string {
	return `apiVersion: v1
kind: Pod
metadata:
  name: block-test
spec:
      containers:
      - image: busybox
        name: block-test1
        command:
          - sleep
          - "3600"
        imagePullPolicy: IfNotPresent
        volumeMounts:
        - name: block-persistent-storage
          mountPath: /tmp/rook1
      volumes:
      - name: block-persistent-storage
        persistentVolumeClaim:
          claimName: ` + blockName + `
      restartPolicy: Never`
}

func isPVCBound(k8sh *utils.K8sHelper, name string) bool {
	inc := 0
	for inc < utils.RetryLoop {
		status, _ := k8sh.GetPVCStatus(name)
		if strings.TrimRight(status, "\n") == "'Bound'" {
			return true
		}
		time.Sleep(time.Second * utils.RetryInterval)
		inc++

	}
	return false
}
