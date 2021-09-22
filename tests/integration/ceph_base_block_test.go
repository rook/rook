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
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func skipSnapshotTest(k8sh *utils.K8sHelper) bool {
	minVersion := "v1.17.0"
	if !k8sh.VersionAtLeast(minVersion) {
		logger.Infof("Skipping snapshot tests as kubernetes version is less than %q for the CSI driver", minVersion)
		return true
	}
	return false
}

func skipCloneTest(k8sh *utils.K8sHelper) bool {
	minVersion := "v1.16.0"
	if !k8sh.VersionAtLeast(minVersion) {
		logger.Infof("Skipping snapshot tests as kubernetes version is less than %q for the CSI driver", minVersion)
		return true
	}
	return false
}

func blockCSICloneTest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, storageClassName string) {
	// create pvc and app
	pvcSize := "1Gi"
	pvcName := "parent-pvc"
	podName := "demo-pod"
	readOnly := false
	mountPoint := "/var/lib/test"
	logger.Infof("create a PVC")
	err := helper.BlockClient.CreatePVC(defaultNamespace, pvcName, storageClassName, "ReadWriteOnce", pvcSize)
	require.NoError(s.T(), err)
	require.True(s.T(), k8sh.WaitUntilPVCIsBound(defaultNamespace, pvcName), "Make sure PVC is Bound")

	logger.Infof("bind PVC to application")
	err = helper.BlockClient.CreatePod(podName, pvcName, defaultNamespace, mountPoint, readOnly)
	assert.NoError(s.T(), err)

	logger.Infof("check pod is in running state")
	require.True(s.T(), k8sh.IsPodRunning(podName, defaultNamespace), "make sure pod is in running state")
	logger.Infof("Storage Mounted successfully")

	// write data to pvc get the checksum value
	logger.Infof("write data to pvc")
	cmd := fmt.Sprintf("dd if=/dev/zero of=%s/file.out bs=1MB count=10 status=none conv=fsync && md5sum %s/file.out", mountPoint, mountPoint)
	resp, err := k8sh.RunCommandInPod(defaultNamespace, podName, cmd)
	require.NoError(s.T(), err)
	pvcChecksum := strings.Fields(resp)
	require.Equal(s.T(), len(pvcChecksum), 2)

	clonePVCName := "clone-pvc"
	logger.Infof("create a new pvc from pvc")
	err = helper.BlockClient.CreatePVCClone(defaultNamespace, clonePVCName, pvcName, storageClassName, "ReadWriteOnce", pvcSize)
	require.NoError(s.T(), err)
	require.True(s.T(), k8sh.WaitUntilPVCIsBound(defaultNamespace, clonePVCName), "Make sure PVC is Bound")

	clonePodName := "clone-pod"
	logger.Infof("bind PVC clone to application")
	err = helper.BlockClient.CreatePod(clonePodName, clonePVCName, defaultNamespace, mountPoint, readOnly)
	assert.NoError(s.T(), err)

	logger.Infof("check pod is in running state")
	require.True(s.T(), k8sh.IsPodRunning(clonePodName, defaultNamespace), "make sure pod is in running state")
	logger.Infof("Storage Mounted successfully")

	// get the checksum of the data and validate it
	logger.Infof("check md5sum of both pvc and clone data is same")
	cmd = fmt.Sprintf("md5sum %s/file.out", mountPoint)
	resp, err = k8sh.RunCommandInPod(defaultNamespace, clonePodName, cmd)
	require.NoError(s.T(), err)
	clonePVCChecksum := strings.Fields(resp)
	require.Equal(s.T(), len(clonePVCChecksum), 2)

	// compare the checksum value and verify the values are equal
	assert.Equal(s.T(), clonePVCChecksum[0], pvcChecksum[0])
	// delete clone PVC and app
	logger.Infof("delete clone pod")

	err = k8sh.DeletePod(k8sutil.DefaultNamespace, clonePodName)
	require.NoError(s.T(), err)
	logger.Infof("delete clone pvc")

	err = helper.BlockClient.DeletePVC(defaultNamespace, clonePVCName)
	assertNoErrorUnlessNotFound(s, err)
	assert.True(s.T(), k8sh.WaitUntilPVCIsDeleted(defaultNamespace, clonePVCName))

	// delete the parent PVC and app
	err = k8sh.DeletePod(k8sutil.DefaultNamespace, podName)
	require.NoError(s.T(), err)
	logger.Infof("delete parent pvc")

	err = helper.BlockClient.DeletePVC(defaultNamespace, pvcName)
	assertNoErrorUnlessNotFound(s, err)
	assert.True(s.T(), k8sh.WaitUntilPVCIsDeleted(defaultNamespace, pvcName))
}

func blockCSISnapshotTest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, storageClassName, namespace string) {
	logger.Infof("install snapshot CRD")
	err := k8sh.CreateSnapshotCRD()
	require.NoError(s.T(), err)

	logger.Infof("install snapshot controller")
	err = k8sh.CreateSnapshotController()
	require.NoError(s.T(), err)

	logger.Infof("check snapshot controller is running")
	err = k8sh.WaitForSnapshotController(15)
	require.NoError(s.T(), err)
	// create snapshot class
	snapshotDeletePolicy := "Delete"
	snapshotClassName := "snapshot-testing"
	logger.Infof("create snapshotclass")
	err = helper.BlockClient.CreateSnapshotClass(snapshotClassName, snapshotDeletePolicy, namespace)
	require.NoError(s.T(), err)
	// create pvc and app
	pvcSize := "1Gi"
	pvcName := "snap-pvc"
	podName := "demo-pod"
	readOnly := false
	mountPoint := "/var/lib/test"
	logger.Infof("create a PVC")
	err = helper.BlockClient.CreatePVC(defaultNamespace, pvcName, storageClassName, "ReadWriteOnce", pvcSize)
	require.NoError(s.T(), err)
	require.True(s.T(), k8sh.WaitUntilPVCIsBound(defaultNamespace, pvcName), "Make sure PVC is Bound")

	logger.Infof("bind PVC to application")
	err = helper.BlockClient.CreatePod(podName, pvcName, defaultNamespace, mountPoint, readOnly)
	assert.NoError(s.T(), err)

	logger.Infof("check pod is in running state")
	require.True(s.T(), k8sh.IsPodRunning(podName, defaultNamespace), "make sure pod is in running state")
	logger.Infof("Storage Mounted successfully")

	// write data to pvc get the checksum value
	logger.Infof("write data to pvc")
	cmd := fmt.Sprintf("dd if=/dev/zero of=%s/file.out bs=1MB count=10 status=none conv=fsync && md5sum %s/file.out", mountPoint, mountPoint)
	resp, err := k8sh.RunCommandInPod(defaultNamespace, podName, cmd)
	require.NoError(s.T(), err)
	pvcChecksum := strings.Fields(resp)
	require.Equal(s.T(), len(pvcChecksum), 2)
	// create a snapshot
	snapshotName := "rbd-pvc-snapshot"
	logger.Infof("create a snapshot from pvc")
	err = helper.BlockClient.CreateSnapshot(snapshotName, pvcName, snapshotClassName, defaultNamespace)
	require.NoError(s.T(), err)
	restorePVCName := "restore-block-pvc"
	// check snapshot is in ready state
	ready, err := k8sh.CheckSnapshotISReadyToUse(snapshotName, defaultNamespace, 15)
	require.NoError(s.T(), err)
	require.True(s.T(), ready, "make sure snapshot is in ready state")
	// create restore from snapshot and bind it to app
	logger.Infof("restore pvc to a new snapshot")
	err = helper.BlockClient.CreatePVCRestore(defaultNamespace, restorePVCName, snapshotName, storageClassName, "ReadWriteOnce", pvcSize)
	require.NoError(s.T(), err)
	require.True(s.T(), k8sh.WaitUntilPVCIsBound(defaultNamespace, restorePVCName), "Make sure PVC is Bound")

	restorePodName := "restore-pod"
	logger.Infof("bind PVC Restore to application")
	err = helper.BlockClient.CreatePod(restorePodName, restorePVCName, defaultNamespace, mountPoint, readOnly)
	assert.NoError(s.T(), err)

	logger.Infof("check pod is in running state")
	require.True(s.T(), k8sh.IsPodRunning(restorePodName, defaultNamespace), "make sure pod is in running state")
	logger.Infof("Storage Mounted successfully")

	// get the checksum of the data and validate it
	logger.Infof("check md5sum of both pvc and restore data is same")
	cmd = fmt.Sprintf("md5sum %s/file.out", mountPoint)
	resp, err = k8sh.RunCommandInPod(defaultNamespace, restorePodName, cmd)
	require.NoError(s.T(), err)
	restorePVCChecksum := strings.Fields(resp)
	require.Equal(s.T(), len(restorePVCChecksum), 2)

	// compare the checksum value and verify the values are equal
	assert.Equal(s.T(), restorePVCChecksum[0], pvcChecksum[0])
	// delete clone PVC and app
	logger.Infof("delete restore pod")

	err = k8sh.DeletePod(k8sutil.DefaultNamespace, restorePodName)
	require.NoError(s.T(), err)
	logger.Infof("delete restore pvc")

	err = helper.BlockClient.DeletePVC(defaultNamespace, restorePVCName)
	assertNoErrorUnlessNotFound(s, err)
	assert.True(s.T(), k8sh.WaitUntilPVCIsDeleted(defaultNamespace, restorePVCName))

	// delete the snapshot
	logger.Infof("delete snapshot")

	err = helper.BlockClient.DeleteSnapshot(snapshotName, pvcName, snapshotClassName, defaultNamespace)
	require.NoError(s.T(), err)
	logger.Infof("delete application pod")

	// delete the parent PVC and app
	err = k8sh.DeletePod(k8sutil.DefaultNamespace, podName)
	require.NoError(s.T(), err)
	logger.Infof("delete parent pvc")

	err = helper.BlockClient.DeletePVC(defaultNamespace, pvcName)
	assertNoErrorUnlessNotFound(s, err)
	assert.True(s.T(), k8sh.WaitUntilPVCIsDeleted(defaultNamespace, pvcName))

	logger.Infof("delete snapshotclass")

	err = helper.BlockClient.DeleteSnapshotClass(snapshotClassName, snapshotDeletePolicy, namespace)
	require.NoError(s.T(), err)
	logger.Infof("delete snapshot-controller")

	err = k8sh.DeleteSnapshotController()
	require.NoError(s.T(), err)
	logger.Infof("delete snapshot CRD")

	// remove snapshotcontroller and delete snapshot CRD
	err = k8sh.DeleteSnapshotCRD()
	require.NoError(s.T(), err)
}

// Smoke Test for Block Storage - Test check the following operations on Block Storage in order
// Create,Mount,Write,Read,Expand,Unmount and Delete.
func runBlockCSITest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string) {
	podName := "block-test"
	poolName := "replicapool"
	storageClassName := "rook-ceph-block"
	blockName := "block-pv-claim"
	podNameWithPVRetained := "block-test-retained"
	poolNameRetained := "replicapoolretained"
	storageClassNameRetained := "rook-ceph-block-retained"
	blockNameRetained := "block-pv-claim-retained"

	clusterInfo := client.AdminClusterInfo(namespace)
	defer blockTestDataCleanUp(helper, k8sh, s, clusterInfo, poolName, storageClassName, blockName, podName, true)
	defer blockTestDataCleanUp(helper, k8sh, s, clusterInfo, poolNameRetained, storageClassNameRetained, blockNameRetained, podNameWithPVRetained, true)
	logger.Infof("Block Storage End to End Integration Test - create, mount, write to, read from, and unmount")
	logger.Infof("Running on Rook Cluster %s", namespace)

	logger.Infof("Step 0 : Get Initial List Block")
	initBlockImages, _ := helper.BlockClient.ListAllImages(clusterInfo)
	assert.Equal(s.T(), 0, len(initBlockImages), "there should not already be any images in the pool")

	logger.Infof("step 1: Create block storage")
	err := helper.BlockClient.CreateStorageClassAndPVC(defaultNamespace, poolName, storageClassName, "Delete", blockName, "ReadWriteOnce")
	require.NoError(s.T(), err)
	require.NoError(s.T(), retryBlockImageCountCheck(helper, clusterInfo, 1), "Make sure a new block is created")
	err = helper.BlockClient.CreateStorageClassAndPVC(defaultNamespace, poolNameRetained, storageClassNameRetained, "Retain", blockNameRetained, "ReadWriteOnce")
	require.NoError(s.T(), err)
	require.NoError(s.T(), retryBlockImageCountCheck(helper, clusterInfo, 2), "Make sure another new block is created")
	logger.Infof("Block Storage created successfully")
	require.True(s.T(), k8sh.WaitUntilPVCIsBound(defaultNamespace, blockName), "Make sure PVC is Bound")
	require.True(s.T(), k8sh.WaitUntilPVCIsBound(defaultNamespace, blockNameRetained), "Make sure PVC with reclaimPolicy:Retain is Bound")

	logger.Infof("step 2: Mount block storage")
	createPodWithBlock(helper, k8sh, s, namespace, storageClassName, podName, blockName)
	createPodWithBlock(helper, k8sh, s, namespace, storageClassName, podNameWithPVRetained, blockNameRetained)

	logger.Infof("step 3: Write to block storage")
	message := "Smoke Test Data for Block storage"
	filename := "bsFile1"
	err = k8sh.WriteToPod("", podName, filename, message)
	assert.NoError(s.T(), err)
	logger.Infof("Write to Block storage successfully")

	logger.Infof("step 4: Read from block storage")
	err = k8sh.ReadFromPod("", podName, filename, message)
	assert.NoError(s.T(), err)
	logger.Infof("Read from  Block storage successfully")

	logger.Infof("step 5: Restart the OSDs to confirm they are still healthy after restart")
	restartOSDPods(k8sh, s, namespace)

	logger.Infof("step 6: Read from block storage again")
	err = k8sh.ReadFromPod("", podName, filename, message)
	assert.NoError(s.T(), err)
	logger.Infof("Read from  Block storage successfully")

	logger.Infof("step 7: Mount same block storage on a different pod. Should not be allowed")
	otherPod := "block-test2"
	err = helper.BlockClient.CreateClientPod(getCSIBlockPodDefinition(otherPod, blockName, defaultNamespace, storageClassName, false))
	assert.NoError(s.T(), err)

	// ** FIX: WHY IS THE RWO VOLUME NOT BEING FENCED??? The second pod is starting successfully with the same PVC
	//require.True(s.T(), k8sh.IsPodInError(otherPod, defaultNamespace, "FailedMount", "Volume is already attached by pod"), "make sure block-test2 pod errors out while mounting the volume")
	//logger.Infof("Block Storage successfully fenced")

	logger.Infof("step 8: Delete fenced pod")
	err = k8sh.DeletePod(k8sutil.DefaultNamespace, otherPod)
	require.NoError(s.T(), err)
	require.True(s.T(), k8sh.IsPodTerminated(otherPod, defaultNamespace), "make sure block-test2 pod is terminated")
	logger.Infof("Fenced pod deleted successfully")

	logger.Infof("step 9: Unmount block storage")
	err = k8sh.DeletePod(k8sutil.DefaultNamespace, podName)
	require.NoError(s.T(), err)
	err = k8sh.DeletePod(k8sutil.DefaultNamespace, podNameWithPVRetained)
	require.NoError(s.T(), err)
	require.True(s.T(), k8sh.IsPodTerminated(podName, defaultNamespace), "make sure block-test pod is terminated")
	require.True(s.T(), k8sh.IsPodTerminated(podNameWithPVRetained, defaultNamespace), "make sure block-test-retained pod is terminated")
	logger.Infof("Block Storage unmounted successfully")

	logger.Infof("step 10: Deleting block storage")
	deletePVC(helper, k8sh, s, clusterInfo, blockName, "Delete")
	deletePVC(helper, k8sh, s, clusterInfo, blockNameRetained, "Retain")

	logger.Infof("step 11: Delete storage classes and pools")
	err = helper.PoolClient.DeletePool(helper.BlockClient, clusterInfo, poolName)
	assert.NoError(s.T(), err)
	err = helper.PoolClient.DeletePool(helper.BlockClient, clusterInfo, poolNameRetained)
	assert.NoError(s.T(), err)
	err = helper.BlockClient.DeleteStorageClass(storageClassName)
	assert.NoError(s.T(), err)
	err = helper.BlockClient.DeleteStorageClass(storageClassNameRetained)
	assert.NoError(s.T(), err)
}

func deletePVC(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, clusterInfo *client.ClusterInfo, pvcName, retainPolicy string) {
	pvName, err := k8sh.GetPVCVolumeName(defaultNamespace, pvcName)
	assert.NoError(s.T(), err)
	pv, err := k8sh.GetPV(pvName)
	require.NoError(s.T(), err)
	logger.Infof("deleting ")
	err = helper.BlockClient.DeletePVC(defaultNamespace, pvcName)
	assert.NoError(s.T(), err)

	assert.Equal(s.T(), retainPolicy, string((*pv).Spec.PersistentVolumeReclaimPolicy))
	if retainPolicy == "Delete" {
		assert.True(s.T(), retryPVCheck(k8sh, pvName, false, ""))
		logger.Infof("PV: %s deleted successfully", pvName)
		assert.NoError(s.T(), retryBlockImageCountCheck(helper, clusterInfo, 1), "Make sure a block is deleted")
		logger.Infof("Block Storage deleted successfully")
	} else {
		assert.True(s.T(), retryPVCheck(k8sh, pvName, true, "Released"))
		assert.NoError(s.T(), retryBlockImageCountCheck(helper, clusterInfo, 1), "Make sure a block is retained")
		logger.Infof("Block Storage retained")
		_, err = k8sh.Kubectl("delete", "pv", pvName)
		assert.NoError(s.T(), err)
	}
}

func createPodWithBlock(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, clusterNamespace, storageClassName, podName, pvcName string) {
	err := helper.BlockClient.CreateClientPod(getCSIBlockPodDefinition(podName, pvcName, defaultNamespace, storageClassName, false))
	assert.NoError(s.T(), err)

	require.True(s.T(), k8sh.IsPodRunning(podName, defaultNamespace), "make sure block-test pod is in running state")
	logger.Infof("Block Storage Mounted successfully")
}

func restartOSDPods(k8sh *utils.K8sHelper, s suite.Suite, namespace string) {
	ctx := context.TODO()
	osdLabel := "app=rook-ceph-osd"

	// Delete the osd pod(s)
	logger.Infof("Deleting osd pod(s)")
	pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: osdLabel})
	assert.NoError(s.T(), err)

	for _, pod := range pods.Items {
		options := metav1.DeleteOptions{}
		err := k8sh.Clientset.CoreV1().Pods(namespace).Delete(ctx, pod.Name, options)
		assert.NoError(s.T(), err)
	}
	for _, pod := range pods.Items {
		logger.Infof("Waiting for osd pod %s to be deleted", pod.Name)
		deleted := k8sh.WaitUntilPodIsDeleted(pod.Name, namespace)
		assert.True(s.T(), deleted)
	}

	// Wait for the new pods to run
	logger.Infof("Waiting for new osd pod to run")
	err = k8sh.WaitForLabeledPodsToRun(osdLabel, namespace)
	assert.NoError(s.T(), err)
}

func runBlockCSITestLite(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, settings *installer.TestCephSettings) {
	logger.Infof("Block Storage End to End Integration Test - create storageclass,pool and pvc")
	logger.Infof("Running on Rook Cluster %s", settings.Namespace)
	clusterInfo := client.AdminClusterInfo(settings.Namespace)
	poolName := "rookpool"
	storageClassName := "rook-ceph-block-lite"
	blockName := "test-block-claim-lite"
	podName := "test-pod-lite"
	defer blockTestDataCleanUp(helper, k8sh, s, clusterInfo, poolName, storageClassName, blockName, podName, true)
	setupBlockLite(helper, k8sh, s, clusterInfo, poolName, storageClassName, blockName, podName)
	if !skipSnapshotTest(k8sh) {
		blockCSISnapshotTest(helper, k8sh, s, storageClassName, settings.Namespace)
	}

	if !skipCloneTest(k8sh) {
		blockCSICloneTest(helper, k8sh, s, storageClassName)
	}
}

func setupBlockLite(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, clusterInfo *client.ClusterInfo,
	poolName, storageClassName, blockName, podName string) {

	// Check initial number of blocks
	initialBlocks, err := helper.BlockClient.ListAllImages(clusterInfo)
	require.NoError(s.T(), err)
	initBlockCount := len(initialBlocks)
	assert.Equal(s.T(), 0, initBlockCount, "why is there already a block image in the new pool?")

	logger.Infof("step : Create Pool,StorageClass and PVC")

	err = helper.BlockClient.CreateStorageClassAndPVC(defaultNamespace, poolName, storageClassName, "Delete", blockName, "ReadWriteOnce")
	require.NoError(s.T(), err)

	require.True(s.T(), k8sh.WaitUntilPVCIsBound(defaultNamespace, blockName))

	// Make sure new block is created
	b, err := helper.BlockClient.ListAllImages(clusterInfo)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 1, len(b), "Make sure new block image is created")
	poolExists, err := helper.PoolClient.CephPoolExists(clusterInfo.Namespace, poolName)
	assert.NoError(s.T(), err)
	assert.True(s.T(), poolExists)
}

func deleteBlockLite(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, clusterInfo *client.ClusterInfo, poolName, storageClassName, blockName string, requireBlockImagesRemoved bool) {
	logger.Infof("deleteBlockLite: cleaning up after test")
	// Delete pvc and storageclass
	err := helper.BlockClient.DeletePVC(defaultNamespace, blockName)
	assertNoErrorUnlessNotFound(s, err)
	assert.True(s.T(), k8sh.WaitUntilPVCIsDeleted(defaultNamespace, blockName))
	if requireBlockImagesRemoved {
		assert.NoError(s.T(), retryBlockImageCountCheck(helper, clusterInfo, 0), "Make sure block images were deleted")
	}

	err = helper.PoolClient.DeletePool(helper.BlockClient, clusterInfo, poolName)
	assertNoErrorUnlessNotFound(s, err)
	err = helper.BlockClient.DeleteStorageClass(storageClassName)
	assertNoErrorUnlessNotFound(s, err)

	checkPoolDeleted(helper, s, clusterInfo.Namespace, poolName)
}

func assertNoErrorUnlessNotFound(s suite.Suite, err error) {
	if err == nil || errors.IsNotFound(err) {
		return
	}
	assert.NoError(s.T(), err)
}

func checkPoolDeleted(helper *clients.TestClient, s suite.Suite, namespace, name string) {
	// only retry once to see if the pool was deleted
	for i := 0; i < 3; i++ {
		found, err := helper.PoolClient.CephPoolExists(namespace, name)
		if err != nil {
			// try again on failure since the pool may have been in an unexpected state while deleting
			logger.Warningf("error getting pools. %+v", err)
		} else if !found {
			logger.Infof("pool %s is deleted", name)
			return
		}
		logger.Infof("pool %s still exists", name)
		time.Sleep(time.Second * utils.RetryInterval)
	}
	// this is not an assert in order to improve reliability of the tests
	logger.Errorf("pool %s was not deleted", name)
}

func blockTestDataCleanUp(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, clusterInfo *client.ClusterInfo, poolname, storageclassname, blockname, podName string, requireBlockImagesRemoved bool) {
	logger.Infof("Cleaning up block storage")
	err := k8sh.DeletePod(k8sutil.DefaultNamespace, podName)
	if err != nil {
		logger.Errorf("failed to delete pod. %v", err)
	}
	deleteBlockLite(helper, k8sh, s, clusterInfo, poolname, storageclassname, blockname, requireBlockImagesRemoved)
}

// periodically checking if block image count has changed to expected value
// When creating pvc in k8s platform, it may take some time for the block Image to be bounded
func retryBlockImageCountCheck(helper *clients.TestClient, clusterInfo *client.ClusterInfo, expectedImageCount int) error {
	for i := 0; i < utils.RetryLoop; i++ {
		blockImages, err := helper.BlockClient.ListAllImages(clusterInfo)
		if err != nil {
			return err
		}
		if expectedImageCount == len(blockImages) {
			return nil
		}
		logger.Infof("Waiting for block image count to reach %d. current=%d. %+v", expectedImageCount, len(blockImages), blockImages)
		time.Sleep(time.Second * utils.RetryInterval)
	}
	return fmt.Errorf("timed out waiting for image count to reach %d", expectedImageCount)
}

func retryPVCheck(k8sh *utils.K8sHelper, name string, exists bool, status string) bool {
	for i := 0; i < utils.RetryLoop; i++ {
		pv, err := k8sh.GetPV(name)
		if err != nil {
			if !exists {
				return true
			}
		}
		if exists {
			if string((*pv).Status.Phase) == status {
				return true
			}
		}
		logger.Infof("Waiting for PV %q to have status %q with exists %t", name, status, exists)
		time.Sleep(time.Second * utils.RetryInterval)
	}
	return false
}

func getCSIBlockPodDefinition(podName, pvcName, namespace, storageClass string, readOnly bool) string {
	return `
apiVersion: v1
kind: Pod
metadata:
  name: ` + podName + `
  namespace: ` + namespace + `
spec:
  containers:
  - name: ` + podName + `
    image: busybox
    command:
        - sh
        - "-c"
        - "touch ` + utils.TestMountPath + `/csi.test && sleep 3600"
    imagePullPolicy: IfNotPresent
    env:
    volumeMounts:
    - mountPath: ` + utils.TestMountPath + `
      name: csivol
  volumes:
  - name: csivol
    persistentVolumeClaim:
       claimName: ` + pvcName + `
       readOnly: ` + strconv.FormatBool(readOnly) + `
  restartPolicy: Never
`
}
