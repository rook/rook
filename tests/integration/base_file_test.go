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

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	fileMountPath = "/tmp/rookfs"
	filePodName   = "file-test"
)

// Smoke Test for File System Storage - Test check the following operations on Filesystem Storage in order
// Create,Mount,Write,Read,Unmount and Delete.
func runFileE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string) {
	defer fileTestDataCleanUp(helper, k8sh, s, filePodName, namespace, filesystemName)
	logger.Infof("Running on Rook Cluster %s", namespace)
	logger.Infof("File Storage End To End Integration Test - create, mount, write to, read from, and unmount")

	createFilesystem(helper, k8sh, s, namespace, filesystemName)
	createFilesystemConsumerPod(helper, k8sh, s, namespace, filesystemName)
	err := writeAndReadToFilesystem(helper, k8sh, s, namespace, filePodName, "test_file")
	require.Nil(s.T(), err)

	testNFSDaemons(helper, k8sh, s, namespace, filesystemName)

	downscaleMetadataServers(helper, k8sh, s, namespace, filesystemName)
	cleanupFilesystemConsumer(helper, k8sh, s, namespace, filesystemName, filePodName)
	cleanupFilesystem(helper, k8sh, s, namespace, filesystemName)
}

func testNFSDaemons(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string) {
	name := "my-nfs"
	err := helper.NFSClient.Create(namespace, name, filesystemName+"-data0", 2)
	require.Nil(s.T(), err)

	err = helper.NFSClient.Delete(namespace, name)
	assert.Nil(s.T(), err)
}

func createFilesystemConsumerPod(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string) {
	mtfsErr := podWithFilesystem(k8sh, s, filePodName, namespace, filesystemName, "create", getFilesystemTestPod)
	require.Nil(s.T(), mtfsErr)
	filePodRunning := k8sh.IsPodRunning(filePodName, namespace)
	if !filePodRunning {
		k8sh.PrintPodDescribe(namespace, filePodName)
		k8sh.PrintPodStatus(namespace)
		k8sh.PrintPodStatus(installer.SystemNamespace(namespace))
	}
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

func downscaleMetadataServers(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace, fsName string) {
	logger.Infof("downscaling file system metadata servers")
	err := helper.FSClient.ScaleDown(fsName, namespace)
	require.Nil(s.T(), err)
}

func cleanupFilesystemConsumer(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string, podName string) {
	logger.Infof("Delete file System consumer")
	err := k8sh.DeletePod(namespace, podName)
	require.Nil(s.T(), err)
	require.True(s.T(), k8sh.IsPodTerminated(podName, namespace), fmt.Sprintf("make sure %s pod is terminated", podName))
	logger.Infof("File system consumer deleted")
}

// cleanupFilesystem cleans up the filesystem and checks if all mds pods are teminated before continuing
func cleanupFilesystem(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string) {
	args := []string{"--grace-period=0", "-n", namespace, "deployment", "-l", fmt.Sprintf("rook_file_system=%s", filesystemName)}
	err := k8sh.DeleteResourceAndWait(false, args...)
	assert.Nil(s.T(), err, "force and no wait delete of rook file system deployments failed")

	logger.Infof("Deleting file system")
	err = helper.FSClient.Delete(filesystemName, namespace)
	require.Nil(s.T(), err)
	logger.Infof("File system %s deleted", filesystemName)
}

// Test File System Creation on Rook that was installed on a custom namespace i.e. Namespace != "rook" and delete it again
func runFileE2ETestLite(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string) {
	logger.Infof("File Storage End to End Integration Test - create Filesystem and make sure mds pod is running")
	logger.Infof("Running on Rook Cluster %s", namespace)
	createFilesystem(helper, k8sh, s, namespace, filesystemName)
	cleanupFilesystem(helper, k8sh, s, namespace, filesystemName)
}

func createFilesystem(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace, filesystemName string) {
	logger.Infof("Create file System")
	fscErr := helper.FSClient.Create(filesystemName, namespace)
	require.Nil(s.T(), fscErr)
	logger.Infof("File system %s created", filesystemName)

	filesystemList, _ := helper.FSClient.List(namespace)
	require.Equal(s.T(), 1, len(filesystemList), "There should be one shared file system present")
}

func fileTestDataCleanUp(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, podName string, namespace string, filesystemName string) {
	logger.Infof("Cleaning up file system")
	podWithFilesystem(k8sh, s, podName, namespace, filesystemName, "delete", getFilesystemTestPod)
	helper.FSClient.Delete(filesystemName, namespace)
}

func podWithFilesystem(
	k8sh *utils.K8sHelper,
	s suite.Suite,
	podName string,
	namespace string,
	filesystemName string,
	action string,
	testPod func(podName string, namespace string, filesystemName string, driverName string) string,
) error {
	driverName := installer.SystemNamespace(namespace)
	testPodManifest := testPod(podName, namespace, filesystemName, driverName)
	logger.Infof("creating test pod: %s", testPodManifest)
	if err := k8sh.ResourceOperation(action, testPodManifest); err != nil {
		return fmt.Errorf("failed to %s pod -- %s. %+v", action, testPodManifest, err)
	}
	return nil
}

func getFilesystemTestPod(podName string, namespace string, filesystemName string, driverName string) string {
	return `apiVersion: v1
kind: Pod
metadata:
  name: ` + podName + `
  namespace: ` + namespace + `
spec:
  containers:
  - name: ` + podName + `
    image: busybox
    command:
        - sleep
        - "3600"
    imagePullPolicy: IfNotPresent
    env:
    volumeMounts:
    - mountPath: "` + utils.TestMountPath + `"
      name: ` + filesystemName + `
  volumes:
  - name: ` + filesystemName + `
    flexVolume:
      driver: ceph.rook.io/` + driverName + `
      fsType: ceph
      options:
        fsName: ` + filesystemName + `
        clusterNamespace: ` + namespace + `
  restartPolicy: Never
`
}
