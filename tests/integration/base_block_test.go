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
	"time"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
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
	podName := "block-test"

	defer blockTestDataCleanUp(helper, k8sh, namespace, poolName, storageClassName, blockName, podName)
	logger.Infof("Block Storage End to End Integration Test - create, mount, write to, read from, and unmount")
	logger.Infof("Running on Rook Cluster %s", namespace)
	rbc := helper.GetBlockClient()

	logger.Infof("Step 0 : Get Initial List Block")
	initBlockImages, _ := rbc.BlockList()

	logger.Infof("step 1: Create block storage")
	_, cbErr := installer.BlockResourceOperation(k8sh, installer.GetBlockPoolStorageClassAndPvcDef(namespace, poolName, storageClassName, blockName), "create")
	require.Nil(s.T(), cbErr)
	require.True(s.T(), retryBlockImageCountCheck(helper, len(initBlockImages), 1), "Make sure a new block is created")
	logger.Infof("Block Storage created successfully")
	require.True(s.T(), k8sh.WaitUntilPVCIsBound(defaultNamespace, blockName), "Make sure PVC is Bound")

	logger.Infof("step 2: Mount block storage")
	_, mtErr := rbc.BlockMap(getBlockPodDefintion(podName, blockName), blockMountPath)
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

	logger.Infof("step 5: Mount same block storage on a different pod. Should not be allowed")
	otherPod := "block-test2"
	_, mtErr = rbc.BlockMap(getBlockPodDefintion(otherPod, blockName), blockMountPath)
	require.Nil(s.T(), mtErr)
	require.True(s.T(), k8sh.IsPodInError(otherPod, defaultNamespace, "FailedMount", "Volume is already attached by pod"), "make sure block-test2 pod errors out while mounting the volume")
	logger.Infof("Block Storage successfully fenced")

	logger.Infof("step 6: Delete fenced pod")
	_, unmtErr := rbc.BlockUnmap(getBlockPodDefintion(otherPod, blockName), blockMountPath)
	require.Nil(s.T(), unmtErr)
	require.True(s.T(), k8sh.IsPodTerminated(otherPod, defaultNamespace), "make sure block-test2 pod is terminated")
	logger.Infof("Fenced pod deleted successfully")

	logger.Infof("step 7: Unmount block storage")
	_, unmtErr = rbc.BlockUnmap(getBlockPodDefintion(podName, blockName), blockMountPath)
	require.Nil(s.T(), unmtErr)
	require.True(s.T(), k8sh.IsPodTerminated(blockPodName, defaultNamespace), "make sure block-test pod is terminated")
	logger.Infof("Block Storage unmounted successfully")

	logger.Infof("step 8: Deleting block storage")
	_, dbErr := installer.BlockResourceOperation(k8sh, installer.GetBlockPoolStorageClassAndPvcDef(namespace, poolName, storageClassName, blockName), "delete")
	require.Nil(s.T(), dbErr)
	require.True(s.T(), retryBlockImageCountCheck(helper, len(initBlockImages), 0), "Make sure a block is deleted")
	logger.Infof("Block Storage deleted successfully")
}

func runBlockE2ETestLite(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, clusterNamespace string) {
	logger.Infof("Block Storage End to End Integration Test - create storageclass,pool and pvc")
	logger.Infof("Running on Rook Cluster %s", clusterNamespace)
	poolName := "rookpool"

	//Check initial number of blocks
	defer blockTestDataCleanUp(helper, k8sh, clusterNamespace, poolName, "rook-block", "test-block", "block-test")
	bc := helper.GetBlockClient()
	initialBlocks, err := bc.BlockList()
	require.Nil(s.T(), err)
	initBlockCount := len(initialBlocks)

	logger.Infof("step : Create Pool,StorageClass and PVC")

	volumeDef := installer.GetBlockPoolStorageClassAndPvcDef(clusterNamespace, poolName, "rook-block", "test-block-claim")
	res1, err := installer.BlockResourceOperation(k8sh, volumeDef, "create")
	require.Contains(s.T(), res1, fmt.Sprintf("pool \"%s\" created", poolName), "Make sure test pool is created")
	require.Contains(s.T(), res1, "storageclass \"rook-block\" created", "Make sure storageclass is created")
	require.Contains(s.T(), res1, "persistentvolumeclaim \"test-block-claim\" created", "Make sure pvc is created")
	require.NoError(s.T(), err)

	require.True(s.T(), k8sh.WaitUntilPVCIsBound(defaultNamespace, "test-block-claim"))

	//Make sure  new block is created
	b, _ := bc.BlockList()
	assert.Equal(s.T(), initBlockCount+1, len(b), "Make sure new block image is created")
	poolExists, err := foundPool(helper, poolName)
	assert.Nil(s.T(), err)
	assert.True(s.T(), poolExists)

	//Delete pvc and storageclass
	_, err = installer.BlockResourceOperation(k8sh, volumeDef, "delete")
	assert.NoError(s.T(), err)

	assert.True(s.T(), k8sh.WaitUntilPVCIsDeleted(defaultNamespace, "test-block-claim"))
	require.True(s.T(), retryBlockImageCountCheck(helper, initBlockCount, 0), "Make sure a new block is deleted")

	b, _ = bc.BlockList()
	assert.Equal(s.T(), initBlockCount, len(b), "Make sure new block image is deleted")

	checkPoolDeleted(helper, s, poolName)
}

func checkPoolDeleted(helper *clients.TestClient, s suite.Suite, name string) {
	i := 0
	for i < utils.RetryLoop {
		found, err := foundPool(helper, name)
		if err != nil {
			// try again on failure since the pool may have been in an unexpected state while deleting
			logger.Warningf("error getting pools. %+v", err)
		} else if !found {
			logger.Infof("pool %s is deleted", name)
			return
		}
		i++
		logger.Infof("pool %s still exists", name)
		time.Sleep(time.Second * utils.RetryInterval)
	}
	assert.Fail(s.T(), fmt.Sprintf("pool %s was not deleted", name))
}

func foundPool(helper *clients.TestClient, name string) (bool, error) {
	p := helper.GetPoolClient()
	pools, err := p.PoolList()
	if err != nil {
		return false, err
	}
	for _, pool := range pools {
		if name == pool.Name {
			return true, nil
		}
	}
	return false, nil
}

func blockTestDataCleanUp(helper *clients.TestClient, k8sh *utils.K8sHelper, namespace, poolname, storageclassname, blockname, podname string) {
	logger.Infof("Cleaning up block storage")
	helper.GetBlockClient().BlockUnmap(getBlockPodDefintion(podname, blockname), blockMountPath)
	installer.BlockResourceOperation(k8sh, installer.GetBlockPoolStorageClassAndPvcDef(namespace, poolname, storageclassname, blockname), "delete")
	cleanupDynamicBlockStorage(helper)
}

// periodically checking if block image count has changed to expected value
// When creating pvc in k8s platform, it may take some time for the block Image to be bounded
func retryBlockImageCountCheck(helper *clients.TestClient, imageCount, expectedChange int) bool {
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
func cleanupDynamicBlockStorage(helper *clients.TestClient) {
	// Delete storage pool, storage class and pvc
	blockImagesList, _ := helper.GetBlockClient().BlockList()
	for _, blockImage := range blockImagesList {
		helper.GetRestAPIClient().DeleteBlockImage(blockImage)
	}

}

func getBlockPodDefintion(podname, blockName string) string {
	return `apiVersion: v1
kind: Pod
metadata:
  name: ` + podname + `
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
