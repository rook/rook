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

	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	"strings"
)

var (
	pvcName        = "block-pv-claim"
	blockMountPath = "/tmp/rook1"
	blockPodName   = "block-test"
)

// Smoke Test for Block Storage - Test check the following operations on Block Storage in order
//Create,Mount,Write,Read,Unmount and Delete.
func (suite *SmokeSuite) TestBlockStorage_SmokeTest() {
	defer suite.blockTestDataCleanUp()
	logger.Infof("Block Storage Smoke Test - create, mount, write to, read from, and unmount")
	rbc := suite.helper.GetBlockClient()

	logger.Infof("Step 0 : Get Initial List Block")
	initBlockImages, _ := rbc.BlockList()

	logger.Infof("step 1: Create block storage")
	cbErr := suite.blockStorageOperation("create")
	require.Nil(suite.T(), cbErr)
	require.True(suite.T(), suite.retryBlockImageCountCheck(len(initBlockImages), 1), "Make sure a new block is created")
	logger.Infof("Block Storage created successfully")
	require.True(suite.T(), suite.k8sh.WaitUntilPVCIsBound(pvcName), "Make sure PVC is Bound")

	logger.Infof("step 2: Mount block storage")
	_, mtErr := rbc.BlockMap(getBlockPodDefintion(), blockMountPath)
	require.Nil(suite.T(), mtErr)
	require.True(suite.T(), suite.k8sh.IsPodRunning(blockPodName), "make sure block-test pod is in running state")
	logger.Infof("Block Storage Mounted successfully")

	logger.Infof("step 3: Write to block storage")
	_, wtErr := rbc.BlockWrite(blockPodName, blockMountPath, "Smoke Test Data form Block storage", "bsFile1", "")
	require.Nil(suite.T(), wtErr)
	logger.Infof("Write to Block storage successfully")

	logger.Infof("step 4: Read from  block storage")
	read, rErr := rbc.BlockRead(blockPodName, blockMountPath, "bsFile1", "")
	require.Nil(suite.T(), rErr)
	require.Contains(suite.T(), read, "Smoke Test Data form Block storage", "make sure content of the files is unchanged")
	logger.Infof("Read from  Block storage successfully")

	logger.Infof("step 5: Unmount block storage")
	_, unmtErr := rbc.BlockUnmap(getBlockPodDefintion(), blockMountPath)
	require.Nil(suite.T(), unmtErr)
	require.True(suite.T(), suite.k8sh.IsPodTerminated(blockPodName), "make sure block-test pod is terminated")
	logger.Infof("Block Storage unmounted successfully")

	logger.Infof("step 6: Deleting block storage")
	dbErr := suite.blockStorageOperation("delete")
	require.Nil(suite.T(), dbErr)
	require.True(suite.T(), suite.retryBlockImageCountCheck(len(initBlockImages), 0), "Make sure a block is deleted")
	logger.Infof("Block Storage deleted successfully")

}

func (suite *SmokeSuite) blockTestDataCleanUp() {
	logger.Infof("Cleaning up block storage")
	suite.helper.GetBlockClient().BlockUnmap(getBlockPodDefintion(), blockMountPath)
	suite.blockStorageOperation("delete")
	suite.cleanUpDymanicBlockStorage()
}

func (suite *SmokeSuite) blockStorageOperation(action string) error {
	poolStorageClassDef := suite.getBlockPoolAndStorageClassDefinition()
	pvcDef := getPvcDefiniton()
	suite.k8sh.ResourceOperation(action, poolStorageClassDef)
	// see https://github.com/rook/rook/issues/767
	time.Sleep(10 * time.Second)
	_, err := suite.k8sh.ResourceOperation(action, pvcDef)

	return err
}

// periodically checking if block image count has changed to expected value
// When creating pvc in k8s platform, it may take some time for the block Image to be bounded
func (suite *SmokeSuite) retryBlockImageCountCheck(imageCount, expectedChange int) bool {
	inc := 0
	for inc < utils.RetryLoop {
		logger.Infof("Getting list of blocks (expecting %d)", (imageCount + expectedChange))
		blockImages, _ := suite.helper.GetBlockClient().BlockList()
		if imageCount+expectedChange == len(blockImages) {
			return true
		}
		time.Sleep(time.Second * utils.RetryInterval)
		inc++
	}
	return false
}

//CleanUpDymanicBlockStorage is helper method to clean up bock storage created by tests
func (suite *SmokeSuite) cleanUpDymanicBlockStorage() {
	// Delete storage pool, storage class and pvc
	blockImagesList, _ := suite.helper.GetBlockClient().BlockList()
	for _, blockImage := range blockImagesList {
		suite.helper.GetRestAPIClient().DeleteBlockImage(blockImage)
	}

}

func (suite *SmokeSuite) getBlockPoolAndStorageClassDefinition() string {
	k8sVersion := suite.k8sh.GetK8sServerVersion()
	if strings.Contains(k8sVersion, "v1.5") {
		return `apiVersion: rook.io/v1alpha1
kind: Pool
metadata:
  name: replicapool
  namespace: rook
spec:
  replication:
    size: 1
  # For an erasure-coded pool, comment out the replication size above and uncomment the following settings.
  # Make sure you have enough OSDs to support the replica size or erasure code chunks.
  #erasureCode:
  #  codingChunks: 2
  #  dataChunks: 2
---
apiVersion: storage.k8s.io/v1beta1
kind: StorageClass
metadata:
   name: rook-block
provisioner: rook.io/block
parameters:
  pool: replicapool`

	}
	return `apiVersion: rook.io/v1alpha1
kind: Pool
metadata:
  name: replicapool
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
    pool: replicapool`
}

func getPvcDefiniton() string {
	return `apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: block-pv-claim
  annotations:
    volume.beta.kubernetes.io/storage-class: rook-block
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1M`
}
func getBlockPodDefintion() string {
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
          claimName: block-pv-claim
      restartPolicy: Never`
}
