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
	"strconv"
	"time"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/api/apps/v1beta1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	blockMountPath = "/tmp/rook1"
	blockPodName   = "block-test"
)

// Smoke Test for Block Storage - Test check the following operations on Block Storage in order
//Create,Mount,Write,Read,Unmount and Delete.
func runBlockE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string) {
	poolName := "replicapool"
	storageClassName := "rook-ceph-block"
	blockName := "block-pv-claim"
	podName := "block-test"

	defer blockTestDataCleanUp(helper, k8sh, namespace, poolName, storageClassName, blockName, podName)
	logger.Infof("Block Storage End to End Integration Test - create, mount, write to, read from, and unmount")
	logger.Infof("Running on Rook Cluster %s", namespace)

	logger.Infof("Step 0 : Get Initial List Block")
	initBlockImages, _ := helper.BlockClient.List(namespace)

	logger.Infof("step 1: Create block storage")
	_, cbErr := installer.BlockResourceOperation(k8sh, installer.GetBlockPoolStorageClassAndPvcDef(namespace, poolName, storageClassName, blockName, "ReadWriteOnce"), "create")
	require.Nil(s.T(), cbErr)
	require.True(s.T(), retryBlockImageCountCheck(helper, len(initBlockImages), 1, namespace), "Make sure a new block is created")
	logger.Infof("Block Storage created successfully")
	require.True(s.T(), k8sh.WaitUntilPVCIsBound(defaultNamespace, blockName), "Make sure PVC is Bound")

	logger.Infof("step 2: Mount block storage")
	_, mtErr := helper.BlockClient.BlockMap(getBlockPodDefintion(podName, blockName, false), blockMountPath)
	require.Nil(s.T(), mtErr)
	crdName, err := k8sh.GetVolumeResourceName(defaultNamespace, blockName)
	require.Nil(s.T(), err)
	require.True(s.T(), k8sh.IsVolumeResourcePresent(installer.SystemNamespace(namespace), crdName), fmt.Sprintf("make sure Volume %s is created", crdName))
	require.True(s.T(), k8sh.IsPodRunning(blockPodName, defaultNamespace), "make sure block-test pod is in running state")
	logger.Infof("Block Storage Mounted successfully")

	logger.Infof("step 3: Write to block storage")
	_, wtErr := helper.BlockClient.Write(blockPodName, blockMountPath, "Smoke Test Data form Block storage", "bsFile1", "")
	require.Nil(s.T(), wtErr)
	logger.Infof("Write to Block storage successfully")

	logger.Infof("step 4: Read from block storage")
	read, rErr := helper.BlockClient.Read(blockPodName, blockMountPath, "bsFile1", "")
	require.Nil(s.T(), rErr)
	assert.Contains(s.T(), read, "Smoke Test Data form Block storage", "make sure content of the files is unchanged")
	logger.Infof("Read from  Block storage successfully")

	logger.Infof("step 5: Restart the OSDs to confirm they are still healthy after restart")
	restartOSDPods(k8sh, s, namespace)

	logger.Infof("step 6: Read from block storage again")
	read, rErr = helper.BlockClient.Read(blockPodName, blockMountPath, "bsFile1", "")
	require.Nil(s.T(), rErr)
	assert.Contains(s.T(), read, "Smoke Test Data form Block storage", "make sure content of the files is unchanged")
	logger.Infof("Read from  Block storage successfully")

	logger.Infof("step 7: Mount same block storage on a different pod. Should not be allowed")
	otherPod := "block-test2"
	_, mtErr = helper.BlockClient.BlockMap(getBlockPodDefintion(otherPod, blockName, false), blockMountPath)
	require.Nil(s.T(), mtErr)
	require.True(s.T(), k8sh.IsPodInError(otherPod, defaultNamespace, "FailedMount", "Volume is already attached by pod"), "make sure block-test2 pod errors out while mounting the volume")
	logger.Infof("Block Storage successfully fenced")

	logger.Infof("step 8: Delete fenced pod")
	_, unmtErr := helper.BlockClient.BlockUnmap(getBlockPodDefintion(otherPod, blockName, false), blockMountPath)
	require.Nil(s.T(), unmtErr)
	require.True(s.T(), k8sh.IsPodTerminated(otherPod, defaultNamespace), "make sure block-test2 pod is terminated")
	logger.Infof("Fenced pod deleted successfully")

	logger.Infof("step 9: Unmount block storage")
	_, unmtErr = helper.BlockClient.BlockUnmap(getBlockPodDefintion(podName, blockName, false), blockMountPath)
	require.Nil(s.T(), unmtErr)
	require.True(s.T(), k8sh.IsVolumeResourceAbsent(installer.SystemNamespace(namespace), crdName), fmt.Sprintf("make sure Volume %s is deleted", crdName))
	require.True(s.T(), k8sh.IsPodTerminated(blockPodName, defaultNamespace), "make sure block-test pod is terminated")
	logger.Infof("Block Storage unmounted successfully")

	logger.Infof("step 10: Deleting block storage")
	_, dbErr := installer.BlockResourceOperation(k8sh, installer.GetBlockPoolStorageClassAndPvcDef(namespace, poolName, storageClassName, blockName, "ReadWriteOnce"), "delete")
	require.Nil(s.T(), dbErr)
	require.True(s.T(), retryBlockImageCountCheck(helper, len(initBlockImages), 0, namespace), "Make sure a block is deleted")
	logger.Infof("Block Storage deleted successfully")
}

func restartOSDPods(k8sh *utils.K8sHelper, s suite.Suite, namespace string) {
	osdLabel := "app=rook-ceph-osd"

	// Delete the osd pod(s)
	logger.Infof("Deleting osd pod(s)")
	pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(metav1.ListOptions{LabelSelector: osdLabel})
	for _, pod := range pods.Items {
		options := metav1.DeleteOptions{}
		err = k8sh.Clientset.CoreV1().Pods(namespace).Delete(pod.Name, &options)
		assert.Nil(s.T(), err)

		logger.Infof("Waiting for osd pod %s to be deleted", pod.Name)
		deleted := k8sh.WaitUntilPodIsDeleted(pod.Name, namespace)
		assert.True(s.T(), deleted)
	}

	// Wait for the new pods to run
	logger.Infof("Waiting for new osd pod to run")
	err = k8sh.WaitForLabeledPodToRun(osdLabel, namespace)
	assert.Nil(s.T(), err)
}

func runBlockE2ETestLite(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, clusterNamespace string) {
	logger.Infof("Block Storage End to End Integration Test - create storageclass,pool and pvc")
	logger.Infof("Running on Rook Cluster %s", clusterNamespace)
	poolName := "rookpool"

	//Check initial number of blocks
	defer blockTestDataCleanUp(helper, k8sh, clusterNamespace, poolName, "rook-ceph-block", "test-block", "block-test")
	bc := helper.BlockClient
	initialBlocks, err := bc.List(clusterNamespace)
	require.Nil(s.T(), err)
	initBlockCount := len(initialBlocks)

	logger.Infof("step : Create Pool,StorageClass and PVC")

	volumeDef := installer.GetBlockPoolStorageClassAndPvcDef(clusterNamespace, poolName, "rook-ceph-block", "test-block-claim", "ReadWriteOnce")
	res1, err := installer.BlockResourceOperation(k8sh, volumeDef, "create")
	assert.Contains(s.T(), res1, fmt.Sprintf("\"%s\" created", poolName), "Make sure test pool is created")
	assert.Contains(s.T(), res1, "\"rook-ceph-block\" created", "Make sure storageclass is created")
	assert.Contains(s.T(), res1, "\"test-block-claim\" created", "Make sure pvc is created")
	require.NoError(s.T(), err)

	require.True(s.T(), k8sh.WaitUntilPVCIsBound(defaultNamespace, "test-block-claim"))

	//Make sure  new block is created
	b, _ := bc.List(clusterNamespace)
	assert.Equal(s.T(), initBlockCount+1, len(b), "Make sure new block image is created")
	poolExists, err := helper.PoolClient.CephPoolExists(clusterNamespace, poolName)
	assert.Nil(s.T(), err)
	assert.True(s.T(), poolExists)

	//Delete pvc and storageclass
	_, err = installer.BlockResourceOperation(k8sh, volumeDef, "delete")
	assert.NoError(s.T(), err)

	assert.True(s.T(), k8sh.WaitUntilPVCIsDeleted(defaultNamespace, "test-block-claim"))
	require.True(s.T(), retryBlockImageCountCheck(helper, initBlockCount, 0, clusterNamespace), "Make sure a new block is deleted")

	b, _ = bc.List(clusterNamespace)
	assert.Equal(s.T(), initBlockCount, len(b), "Make sure new block image is deleted")

	checkPoolDeleted(helper, s, clusterNamespace, poolName)
}

func checkPoolDeleted(helper *clients.TestClient, s suite.Suite, namespace, name string) {
	i := 0
	for i < utils.RetryLoop {
		found, err := helper.PoolClient.CephPoolExists(namespace, name)
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

func blockTestDataCleanUp(helper *clients.TestClient, k8sh *utils.K8sHelper, namespace, poolname, storageclassname, blockname, podname string) {
	logger.Infof("Cleaning up block storage")
	helper.BlockClient.BlockUnmap(getBlockPodDefintion(podname, blockname, false), blockMountPath)
	installer.BlockResourceOperation(k8sh, installer.GetBlockPoolStorageClassAndPvcDef(namespace, poolname, storageclassname, blockname, "ReadWriteOnce"), "delete")
	cleanupDynamicBlockStorage(helper, namespace)
}

// periodically checking if block image count has changed to expected value
// When creating pvc in k8s platform, it may take some time for the block Image to be bounded
func retryBlockImageCountCheck(helper *clients.TestClient, imageCount, expectedChange int, namespace string) bool {
	inc := 0
	for inc < utils.RetryLoop {
		logger.Infof("Getting list of blocks (expecting %d)", (imageCount + expectedChange))
		blockImages, _ := helper.BlockClient.List(namespace)
		if imageCount+expectedChange == len(blockImages) {
			return true
		}
		time.Sleep(time.Second * utils.RetryInterval)
		inc++
	}
	return false
}

//CleanUpDymanicBlockStorage is helper method to clean up bock storage created by tests
func cleanupDynamicBlockStorage(helper *clients.TestClient, namespace string) {
	// Delete storage pool, storage class and pvc
	blockImagesList, _ := helper.BlockClient.List(namespace)
	for _, blockImage := range blockImagesList {
		helper.BlockClient.DeleteBlockImage(blockImage, namespace)
	}

}

func getBlockPodDefintion(podname, blockName string, readOnly bool) string {
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
          readOnly: ` + strconv.FormatBool(readOnly) + `
      restartPolicy: Never`
}

func getBlockStatefulSetAndServiceDefinition(namespace, statefulsetName, podname, StorageClassName string) (*v1.Service, *v1beta1.StatefulSet) {
	service := &v1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulsetName,
			Namespace: namespace,
			Labels: map[string]string{
				"app": statefulsetName,
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name: statefulsetName,
					Port: 80,
				},
			},
			ClusterIP: "None",
			Selector: map[string]string{
				"app": statefulsetName,
			},
		},
	}

	var replica int32 = 1

	statefulSet := &v1beta1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1beta1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      podname,
			Namespace: namespace,
		},
		Spec: v1beta1.StatefulSetSpec{
			ServiceName: statefulsetName,
			Replicas:    &replica,
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": statefulsetName,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:    statefulsetName,
							Image:   "busybox",
							Command: []string{"sleep", "3600"},
							Ports: []v1.ContainerPort{
								{
									ContainerPort: 80,
									Name:          podname,
								},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "rookpvc",
									MountPath: "/tmp/rook",
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []v1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rookpvc",
						Annotations: map[string]string{
							"volume.beta.kubernetes.io/storage-class": StorageClassName,
						},
					},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceStorage: *resource.NewQuantity(1.0, resource.BinarySI),
							},
						},
					},
				},
			},
		},
	}

	return service, statefulSet
}
