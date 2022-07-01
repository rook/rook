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
	"strings"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	filePodName = "file-test"
)

func fileSystemCSICloneTest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, storageClassName, systemNamespace string) {
	// create pvc and app
	pvcSize := "1Gi"
	pvcName := "parent-pvc"
	podName := "demo-pod"
	readOnly := false
	mountPoint := "/var/lib/test"
	logger.Infof("create a PVC")
	err := helper.FSClient.CreatePVC(defaultNamespace, pvcName, storageClassName, "ReadWriteOnce", pvcSize)
	require.NoError(s.T(), err)
	require.True(s.T(), k8sh.WaitUntilPVCIsBound(defaultNamespace, pvcName), "Make sure PVC is Bound")

	logger.Infof("bind PVC to application")
	err = helper.FSClient.CreatePod(podName, pvcName, defaultNamespace, mountPoint, readOnly)
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
	err = helper.FSClient.CreatePVCClone(defaultNamespace, clonePVCName, pvcName, storageClassName, "ReadWriteOnce", pvcSize)
	require.NoError(s.T(), err)
	require.True(s.T(), k8sh.WaitUntilPVCIsBound(defaultNamespace, clonePVCName), "Make sure PVC is Bound")

	clonePodName := "clone-pod"
	logger.Infof("bind PVC clone to application")
	err = helper.FSClient.CreatePod(clonePodName, clonePVCName, defaultNamespace, mountPoint, readOnly)
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

	err = helper.FSClient.DeletePVC(defaultNamespace, clonePVCName)
	assertNoErrorUnlessNotFound(s, err)
	assert.True(s.T(), k8sh.WaitUntilPVCIsDeleted(defaultNamespace, clonePVCName))

	// delete the parent PVC and app
	err = k8sh.DeletePod(k8sutil.DefaultNamespace, podName)
	require.NoError(s.T(), err)
	logger.Infof("delete parent pvc")

	err = helper.FSClient.DeletePVC(defaultNamespace, pvcName)
	assertNoErrorUnlessNotFound(s, err)
	assert.True(s.T(), k8sh.WaitUntilPVCIsDeleted(defaultNamespace, pvcName))
}

func fileSystemCSISnapshotTest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, storageClassName, namespace string) {
	logger.Infof("install snapshot CRD")
	err := k8sh.CreateSnapshotCRD()
	require.NoError(s.T(), err)

	logger.Infof("install snapshot controller")
	err = k8sh.CreateSnapshotController()
	require.NoError(s.T(), err)
	// cleanup the CRD and controller in defer to make sure the CRD and
	// controller are removed as block test also install CRD and controller.
	defer func() {
		logger.Infof("delete snapshot-controller")
		err = k8sh.DeleteSnapshotController()
		require.NoError(s.T(), err)

		logger.Infof("delete snapshot CRD")
		err = k8sh.DeleteSnapshotCRD()
		require.NoError(s.T(), err)
	}()
	logger.Infof("check snapshot controller is running")
	err = k8sh.WaitForSnapshotController(15)
	require.NoError(s.T(), err)
	// create snapshot class
	snapshotDeletePolicy := "Delete"
	snapshotClassName := "snapshot-testing"
	logger.Infof("create snapshotclass")
	err = helper.FSClient.CreateSnapshotClass(snapshotClassName, snapshotDeletePolicy, namespace)
	require.NoError(s.T(), err)
	// create pvc and app
	pvcSize := "1Gi"
	pvcName := "snap-pvc"
	podName := "demo-pod"
	readOnly := false
	mountPoint := "/var/lib/test"
	logger.Infof("create a PVC")
	err = helper.FSClient.CreatePVC(defaultNamespace, pvcName, storageClassName, "ReadWriteOnce", pvcSize)
	require.NoError(s.T(), err)
	require.True(s.T(), k8sh.WaitUntilPVCIsBound(defaultNamespace, pvcName), "Make sure PVC is Bound")

	logger.Infof("bind PVC to application")
	err = helper.FSClient.CreatePod(podName, pvcName, defaultNamespace, mountPoint, readOnly)
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
	err = helper.FSClient.CreateSnapshot(snapshotName, pvcName, snapshotClassName, defaultNamespace)
	require.NoError(s.T(), err)
	restorePVCName := "restore-block-pvc"
	// check snapshot is in ready state
	ready, err := k8sh.CheckSnapshotISReadyToUse(snapshotName, defaultNamespace, 15)
	require.NoError(s.T(), err)
	require.True(s.T(), ready, "make sure snapshot is in ready state")
	// create restore from snapshot and bind it to app
	logger.Infof("restore pvc to a new snapshot")
	err = helper.FSClient.CreatePVCRestore(defaultNamespace, restorePVCName, snapshotName, storageClassName, "ReadWriteOnce", pvcSize)
	require.NoError(s.T(), err)
	require.True(s.T(), k8sh.WaitUntilPVCIsBound(defaultNamespace, restorePVCName), "Make sure PVC is Bound")

	restorePodName := "restore-pod"
	logger.Infof("bind PVC Restore to application")
	err = helper.FSClient.CreatePod(restorePodName, restorePVCName, defaultNamespace, mountPoint, readOnly)
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

	err = helper.FSClient.DeletePVC(defaultNamespace, restorePVCName)
	assertNoErrorUnlessNotFound(s, err)
	assert.True(s.T(), k8sh.WaitUntilPVCIsDeleted(defaultNamespace, restorePVCName))

	// delete the snapshot
	logger.Infof("delete snapshot")

	err = helper.FSClient.DeleteSnapshot(snapshotName, pvcName, snapshotClassName, defaultNamespace)
	require.NoError(s.T(), err)
	logger.Infof("delete application pod")

	// delete the parent PVC and app
	err = k8sh.DeletePod(k8sutil.DefaultNamespace, podName)
	require.NoError(s.T(), err)
	logger.Infof("delete parent pvc")

	err = helper.FSClient.DeletePVC(defaultNamespace, pvcName)
	assertNoErrorUnlessNotFound(s, err)
	assert.True(s.T(), k8sh.WaitUntilPVCIsDeleted(defaultNamespace, pvcName))

	logger.Infof("delete snapshotclass")

	err = helper.FSClient.DeleteSnapshotClass(snapshotClassName, snapshotDeletePolicy, namespace)
	require.NoError(s.T(), err)
	logger.Infof("delete snapshot-controller")
}

// Smoke Test for File System Storage - Test check the following operations on Filesystem Storage in order
// Create,Mount,Write,Read,Unmount and Delete.
func runFileE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, settings *installer.TestCephSettings, filesystemName string, preserveFilesystemOnDelete bool) {
	defer fileTestDataCleanUp(helper, k8sh, s, filePodName, settings.Namespace, filesystemName)
	logger.Infof("Running on Rook Cluster %s", settings.Namespace)
	logger.Infof("File Storage End To End Integration Test - create, mount, write to, read from, and unmount")
	activeCount := 2
	createFilesystem(helper, k8sh, s, settings, filesystemName, activeCount)

	if preserveFilesystemOnDelete {
		_, err := k8sh.Kubectl("-n", settings.Namespace, "patch", "CephFilesystem", filesystemName, "--type=merge", "-p", `{"spec": {"preserveFilesystemOnDelete": true}}`)
		assert.NoError(s.T(), err)
	}

	// Create a test pod where CephFS is consumed without user creds
	storageClassName := "cephfs-storageclass"
	err := helper.FSClient.CreateStorageClass(filesystemName, settings.OperatorNamespace, settings.Namespace, storageClassName)
	assert.NoError(s.T(), err)
	createFilesystemConsumerPod(helper, k8sh, s, settings, filesystemName, storageClassName)

	// Test reading and writing to the first pod
	err = writeAndReadToFilesystem(helper, k8sh, s, settings.Namespace, filePodName, "test_file")
	assert.NoError(s.T(), err)

	t := s.T()
	ctx := context.TODO()

	// TODO: there is a regression here where MDSes don't actually scale down, and this test
	// wasn't catching it. Enabling this test causes the controller to enter into a new reconcile
	// loop and makes the next phase of the test take much longer than it should, making it flaky.
	// Rook issue https://github.com/rook/rook/issues/9857 is tracking this issue.
	// t.Run("filesystem should be able to be scaled down", func(t *testing.T) {
	// 	downscaleMetadataServers(helper, k8sh, t, settings.Namespace, filesystemName)
	// })

	subvolGroupName := "my-subvolume-group"
	t.Run("install CephFilesystemSubVolumeGroup", func(t *testing.T) {
		err = helper.FSClient.CreateSubvolumeGroup(filesystemName, subvolGroupName)
		assert.NoError(t, err)
	})

	t.Run("delete CephFilesystem should be blocked by csi volumes and CephFilesystemSubVolumeGroup", func(t *testing.T) {
		// NOTE: CephFilesystems do not set "Deleting" phase when they are deleting, so we can't
		// rely on that here

		err := k8sh.RookClientset.CephV1().CephFilesystems(settings.Namespace).Delete(
			ctx, filesystemName, metav1.DeleteOptions{})
		assert.NoError(t, err)

		var cond *cephv1.Condition
		err = wait.Poll(2*time.Second, 15*time.Second, func() (done bool, err error) {
			logger.Infof("waiting for CephFilesystem %q in namespace %q to have condition %q",
				filesystemName, settings.Namespace, cephv1.ConditionDeletionIsBlocked)
			fs, err := k8sh.RookClientset.CephV1().CephFilesystems(settings.Namespace).Get(
				ctx, filesystemName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			logger.Infof("conditions: %+v", fs.Status.Conditions)

			cond = cephv1.FindStatusCondition(fs.Status.Conditions, cephv1.ConditionDeletionIsBlocked)
			if cond != nil {
				logger.Infof("CephFilesystem %q in namespace %q has condition %q",
					filesystemName, settings.Namespace, cephv1.ConditionDeletionIsBlocked)
				return true, nil
			}

			return false, nil
		})
		assert.NoError(t, err)

		if cond == nil {
			return
		}
		logger.Infof("verifying CephFilesystem %q condition %q is correct: %+v",
			filesystemName, cephv1.ConditionDeletionIsBlocked, cond)

		assert.Equal(t, v1.ConditionTrue, cond.Status)
		assert.Equal(t, cephv1.ObjectHasDependentsReason, cond.Reason)
		// the CephFilesystemSubVolumeGroup and the "csi" subvolumegroup should both block deletion
		assert.Contains(t, cond.Message, "CephFilesystemSubVolumeGroups")
		assert.Contains(t, cond.Message, subvolGroupName)
		assert.Contains(t, cond.Message, "filesystem subvolume groups that contain subvolumes")
		assert.Contains(t, cond.Message, "csi")
	})

	t.Run("deleting CephFilesystemSubVolumeGroup should partially unblock CephFilesystem deletion", func(t *testing.T) {
		err = helper.FSClient.DeleteSubvolumeGroup(filesystemName, subvolGroupName)
		assert.NoError(t, err)

		var cond *cephv1.Condition
		err = wait.Poll(2*time.Second, 18*time.Second, func() (done bool, err error) {
			logger.Infof("waiting for CephFilesystem %q in namespace %q no longer be blocked by CephFilesystemSubVolumeGroups",
				filesystemName, settings.Namespace)
			fs, err := k8sh.RookClientset.CephV1().CephFilesystems(settings.Namespace).Get(
				ctx, filesystemName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			cond = cephv1.FindStatusCondition(fs.Status.Conditions, cephv1.ConditionDeletionIsBlocked)
			if cond == nil {
				logger.Warningf("could not find condition %q on CephFilesystem %q", cephv1.ConditionDeletionIsBlocked, filesystemName)
				return false, nil
			}

			if !strings.Contains(cond.Message, "CephFilesystemSubVolumeGroup") {
				logger.Infof("CephFilesystem %q deletion is no longer blocked by CephFilesystemSubVolumeGroups", filesystemName)
				return true, nil
			}

			return false, nil
		})
		assert.NoError(t, err)

		if cond == nil {
			return
		}
		logger.Infof("verifying CephFilesystem %q condition %q is correct: %+v",
			filesystemName, cephv1.ConditionDeletionIsBlocked, cond)

		assert.Equal(t, v1.ConditionTrue, cond.Status)
		assert.Equal(t, cephv1.ObjectHasDependentsReason, cond.Reason)
		// only the raw subvolumegroups should block deletion
		assert.Contains(t, cond.Message, "filesystem subvolume groups that contain subvolumes")
		assert.Contains(t, cond.Message, "csi")
	})

	t.Run("deleting filesystem consumer pod+pvc should fully unblock CephFilesystem deletion", func(t *testing.T) {
		// Cleanup the filesystem and its clients
		cleanupFilesystemConsumer(helper, k8sh, s, settings.Namespace, filePodName)

		err = wait.Poll(3*time.Second, 30*time.Second, func() (done bool, err error) {
			logger.Infof("waiting for CephFilesystem %q in namespace %q to be deleted", filesystemName, settings.Namespace)

			_, err = k8sh.RookClientset.CephV1().CephFilesystems(settings.Namespace).Get(
				ctx, filesystemName, metav1.GetOptions{})
			if err != nil && kerrors.IsNotFound(err) {
				return true, nil
			}

			return false, nil
		})

		logger.Infof("CephFilesystem %q in namespace %q was deleted successfully", filesystemName, settings.Namespace)
	})

	err = helper.FSClient.DeleteStorageClass(storageClassName)
	assertNoErrorUnlessNotFound(s, err)

	if preserveFilesystemOnDelete {
		fses, err := helper.FSClient.List(settings.Namespace)
		assert.NoError(s.T(), err)
		assert.Len(s.T(), fses, 1)
		assert.Equal(s.T(), fses[0].Name, filesystemName)

		err = helper.FSClient.Delete(filesystemName, settings.Namespace)
		assert.NoError(s.T(), err)
	}
}

func createFilesystemConsumerPod(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, settings *installer.TestCephSettings, filesystemName, storageClassName string) {
	err := createPodWithFilesystem(k8sh, s, settings, filePodName, filesystemName, storageClassName, false)
	require.NoError(s.T(), err)
	filePodRunning := k8sh.IsPodRunning(filePodName, settings.Namespace)
	require.True(s.T(), filePodRunning, "make sure file-test pod is in running state")
	logger.Infof("File system mounted successfully")
}

func writeAndReadToFilesystem(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace, podName, filename string) error {
	logger.Infof("Write to file system")
	message := "Test Data for file system storage"
	if err := k8sh.WriteToPod(namespace, podName, filename, message); err != nil {
		return err
	}

	return k8sh.ReadFromPod(namespace, podName, filename, message)
}

// func downscaleMetadataServers(helper *clients.TestClient, k8sh *utils.K8sHelper, t *testing.T, namespace, fsName string) {
// 	logger.Infof("downscaling file system metadata servers")
// 	err := helper.FSClient.ScaleDown(fsName, namespace)
// 	require.Nil(t, err)
// }

func cleanupFilesystemConsumer(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, podName string) {
	logger.Infof("Delete file System consumer")
	err := k8sh.DeletePod(namespace, podName)
	assert.Nil(s.T(), err)
	if !k8sh.IsPodTerminated(podName, namespace) {
		k8sh.PrintPodDescribe(namespace, podName)
		assert.Fail(s.T(), fmt.Sprintf("make sure %s pod is terminated", podName))
	}
	err = helper.FSClient.DeletePVC(namespace, podName)
	assertNoErrorUnlessNotFound(s, err)
	isdeleted := k8sh.WaitUntilPVCIsDeleted(namespace, podName)
	if !isdeleted {
		assert.Fail(s.T(), fmt.Sprintf("Failed to delete PVC %q", podName))
	}
	logger.Infof("File system consumer deleted")
}

// cleanupFilesystem cleans up the filesystem and checks if all mds pods are terminated before continuing
func cleanupFilesystem(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string) {
	logger.Infof("Deleting file system")
	err := helper.FSClient.Delete(filesystemName, namespace)
	assert.Nil(s.T(), err)
	logger.Infof("File system %s deleted", filesystemName)
}

// Test File System Creation on Rook that was installed on a custom namespace i.e. Namespace != "rook" and delete it again
func runFileE2ETestLite(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, settings *installer.TestCephSettings, filesystemName string) {
	logger.Infof("File Storage End to End Integration Test - create Filesystem and make sure mds pod is running")
	logger.Infof("Running on Rook Cluster %s", settings.Namespace)
	activeCount := 1
	createFilesystem(helper, k8sh, s, settings, filesystemName, activeCount)
	// Create a test pod where CephFS is consumed without user creds
	storageClassName := "cephfs-storageclass"
	err := helper.FSClient.CreateStorageClass(filesystemName, settings.OperatorNamespace, settings.Namespace, storageClassName)
	assert.NoError(s.T(), err)
	assert.NoError(s.T(), err)
	if !skipSnapshotTest(k8sh) {
		fileSystemCSISnapshotTest(helper, k8sh, s, storageClassName, settings.Namespace)
	}

	if !skipCloneTest(k8sh) {
		fileSystemCSICloneTest(helper, k8sh, s, storageClassName, settings.Namespace)
	}
	cleanupFilesystem(helper, k8sh, s, settings.Namespace, filesystemName)
	err = helper.FSClient.DeleteStorageClass(storageClassName)
	assertNoErrorUnlessNotFound(s, err)
}

func createFilesystem(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, settings *installer.TestCephSettings, filesystemName string, activeCount int) {
	logger.Infof("Create file System")
	fscErr := helper.FSClient.Create(filesystemName, settings.Namespace, activeCount)
	require.Nil(s.T(), fscErr)
	var err error

	var filesystemList []cephclient.CephFilesystem
	for i := 1; i <= 10; i++ {
		filesystemList, err = helper.FSClient.List(settings.Namespace)
		if err != nil {
			logger.Errorf("failed to list fs. trying again. %v", err)
			continue
		}
		logger.Debugf("filesystemList is %+v", filesystemList)
		if len(filesystemList) == 1 {
			logger.Infof("File system %s created", filesystemList[0].Name)
			break
		}
		logger.Infof("Waiting for file system %s to be created", filesystemName)
		time.Sleep(time.Second * 5)
	}
	logger.Debugf("filesystemList is %+v", filesystemList)
	require.Equal(s.T(), 1, len(filesystemList), "There should be one shared file system present")
}

func fileTestDataCleanUp(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, podName string, namespace string, filesystemName string) {
	logger.Infof("Cleaning up file system")
	err := k8sh.DeletePod(namespace, podName)
	assert.NoError(s.T(), err)
	err = helper.FSClient.Delete(filesystemName, namespace)
	assert.NoError(s.T(), err)
}

func createPodWithFilesystem(k8sh *utils.K8sHelper, s suite.Suite, settings *installer.TestCephSettings, podName, filesystemName, storageClassName string, mountUser bool) error {
	testPodManifest := getFilesystemCSITestPod(settings, podName, storageClassName)
	if err := k8sh.ResourceOperation("create", testPodManifest); err != nil {
		return fmt.Errorf("failed to create pod -- %s. %+v", testPodManifest, err)
	}
	return nil
}

func getFilesystemCSITestPod(settings *installer.TestCephSettings, podName, storageClassName string) string {
	claimName := podName
	return `
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ` + claimName + `
  namespace: ` + settings.Namespace + `
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: ` + storageClassName + `
---
apiVersion: v1
kind: Pod
metadata:
  name: ` + podName + `
  namespace: ` + settings.Namespace + `
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
       claimName: ` + claimName + `
       readOnly: false
  restartPolicy: Never
`
}

func waitForFilesystemActive(k8sh *utils.K8sHelper, clusterInfo *client.ClusterInfo, filesystemName string) error {
	command, args := cephclient.FinalizeCephCommandArgs("ceph", clusterInfo, []string{"fs", "status", filesystemName}, k8sh.MakeContext().ConfigDir)
	var stat string
	var err error

	logger.Infof("waiting for filesystem %q to be active", filesystemName)
	for i := 0; i < utils.RetryLoop; i++ {
		// run the ceph fs status command
		stat, err := k8sh.MakeContext().Executor.ExecuteCommandWithCombinedOutput(command, args...)
		if err != nil {
			logger.Warningf("failed to get filesystem %q status. %+v", filesystemName, err)
		}

		// as long as at least one mds is active, it's okay
		if strings.Contains(stat, "active") {
			logger.Infof("done waiting for filesystem %q to be active", filesystemName)
			return nil
		}
		logger.Infof("waiting for filesystem %q to be active. status=%s", filesystemName, stat)
		time.Sleep(utils.RetryInterval * time.Second)
	}
	return fmt.Errorf("gave up waiting to get filesystem %q status [err: %+v] Status returned:\n%s", filesystemName, err, stat)
}
