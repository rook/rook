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

	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/kubernetes/pkg/util/version"
)

var (
	fileMountPath = "/tmp/rookfs"
	filePodName   = "file-test"
)

// Smoke Test for File System Storage - Test check the following operations on Filesystem Storage in order
//Create,Mount,Write,Read,Unmount and Delete.
func runFileE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string) {
	defer fileTestDataCleanUp(helper, k8sh, s, filePodName, namespace, filesystemName, fileMountPath)
	logger.Infof("Running on Rook Cluster %s", namespace)
	logger.Infof("File Storage End To End Integration Test - create, mount, write to, read from, and unmount")

	logger.Infof("Step 1: Create file System")
	fscErr := helper.FSClient.Create(filesystemName, namespace)
	require.Nil(s.T(), fscErr)
	filesystemList, _ := helper.FSClient.List(namespace)
	require.Equal(s.T(), 1, len(filesystemList), "There should one shared file system present")
	filesystemData := filesystemList[0]
	require.Equal(s.T(), filesystemName, filesystemData.Name, "make sure filesystem name matches")
	logger.Infof("File system created")

	logger.Infof("Step 2: Mount file System")
	mtfsErr := podWithFilesystem(k8sh, s, filePodName, namespace, filesystemName, fileMountPath, "create")
	require.Nil(s.T(), mtfsErr)
	filePodRunning := k8sh.IsPodRunning(filePodName, namespace)
	if !filePodRunning {
		k8sh.PrintPodDescribe(filePodName, namespace)
		k8sh.PrintPodStatus(namespace)
		k8sh.PrintPodStatus(installer.SystemNamespace(namespace))
	}
	require.True(s.T(), filePodRunning, "make sure file-test pod is in running state")
	logger.Infof("File system mounted successfully")

	logger.Infof("Step 3: Write to file system")
	_, wfsErr := helper.FSClient.Write(filePodName, fileMountPath, "Smoke Test Data for file system storage", "fsFile1", namespace)
	require.Nil(s.T(), wfsErr)
	logger.Infof("Write to file system successful")

	logger.Infof("Step 4: Read from file system")
	read, rdErr := helper.FSClient.Read(filePodName, fileMountPath, "fsFile1", namespace)
	require.Nil(s.T(), rdErr)
	assert.Contains(s.T(), read, "Smoke Test Data for file system storage", "make sure content of the files is unchanged")
	logger.Infof("Read from file system successful")

	logger.Infof("Step 5: UnMount file System")
	umtfsErr := podWithFilesystem(k8sh, s, filePodName, namespace, filesystemName, fileMountPath, "delete")
	require.Nil(s.T(), umtfsErr)
	require.True(s.T(), k8sh.IsPodTerminated(filePodName, namespace), "make sure file-test pod is terminated")
	logger.Infof("File system mounted successfully")

	logger.Infof("Step 6: Deleting file storage")
	helper.FSClient.Delete(filesystemName, namespace)
	//Delete is not deleting filesystem - known issue
	//require.Nil(suite.T(), fsd_err)
	logger.Infof("File system deleted")
}

//Test File System Creation on Rook that was installed on a custom namespace i.e. Namespace != "rook"
func runFileE2ETestLite(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string) {
	logger.Infof("File Storage End to End Integration Test - create Filesystem and make sure mds pod is running")
	logger.Infof("Running on Rook Cluster %s", namespace)

	logger.Infof("Step 1: Create file System")
	fscErr := helper.FSClient.Create(filesystemName, namespace)
	require.Nil(s.T(), fscErr)
	filesystemList, _ := helper.FSClient.List(namespace)
	logger.Infof("Retrieved file systems: %+v", filesystemList)
	require.Equal(s.T(), 1, len(filesystemList), "There should one shared file system present")
}

func fileTestDataCleanUp(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, podname string, namespace string, filesystemName string, fileMountPath string) {
	logger.Infof("Cleaning up file system")
	podWithFilesystem(k8sh, s, podname, namespace, filesystemName, fileMountPath, "delete")
	helper.FSClient.Delete(filesystemName, namespace)
}

func podWithFilesystem(k8sh *utils.K8sHelper, s suite.Suite, podname string, namespace string, filesystemName string, fileMountPath string, action string) error {
	driverName := installer.SystemNamespace(namespace)
	v := version.MustParseSemantic(k8sh.GetK8sServerVersion())
	if v.LessThan(version.MustParseSemantic("1.10.0")) {
		// k8s 1.10 and newer requires the new driver name to avoid conflicts in the test
		driverName = flexvolume.FlexDriverName
	}

	testPod := getFilesystemTestPod(podname, namespace, filesystemName, fileMountPath, driverName)
	logger.Infof("creating test pod: %s", testPod)
	_, err := k8sh.ResourceOperation(action, testPod)
	if err != nil {
		return fmt.Errorf("failed to %s pod -- %s. %+v", action, testPod, err)
	}
	return nil
}

func getFilesystemTestPod(podname, namespace, filesystemName, fileMountPath, driverName string) string {
	return `apiVersion: v1
kind: Pod
metadata:
  name: ` + podname + `
  namespace: ` + namespace + `
spec:
  containers:
  - name: ` + podname + `
    image: busybox
    command:
        - sleep
        - "3600"
    imagePullPolicy: IfNotPresent
    env:
    volumeMounts:
    - mountPath: "` + fileMountPath + `"
      name: ` + filesystemName + `
  volumes:
  - name: ` + filesystemName + `
    flexVolume:
      driver: ceph.rook.io/` + driverName + `
      fsType: ceph
      options:
        fsName: ` + filesystemName + `
        clusterName: ` + namespace + `
  restartPolicy: Never
`
}
