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
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	fileMountPath = "/tmp/rookfs"
	filePodName   = "file-test"
)

// Smoke Test for File System Storage - Test check the following operations on FileSystem Storage in order
//Create,Mount,Write,Read,Unmount and Delete.
func runFileE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, fileSystemName string) {
	defer fileTestDataCleanUp(helper, k8sh, s, filePodName, namespace, fileSystemName, fileMountPath)
	logger.Infof("Running on Rook Cluster %s", namespace)
	logger.Infof("File Storage End To End Integration Test - create, mount, write to, read from, and unmount")
	rfc := helper.GetFileSystemClient()

	logger.Infof("Step 1: Create file System")
	_, fscErr := rfc.FSCreate(fileSystemName)
	require.Nil(s.T(), fscErr)
	fileSystemList, _ := rfc.FSList()
	require.Equal(s.T(), 1, len(fileSystemList), "There should one shared file system present")
	filesystemData := fileSystemList[0]
	require.Equal(s.T(), fileSystemName, filesystemData.Name, "make sure filesystem name matches")
	logger.Infof("File system created")

	logger.Infof("Step 2: Mount file System")
	mtfsErr := podWithFileSystem(k8sh, s, filePodName, namespace, fileSystemName, fileMountPath, "create")
	require.Nil(s.T(), mtfsErr)
	require.True(s.T(), k8sh.IsPodRunning(filePodName, namespace), "make sure file-test pod is in running state")
	logger.Infof("File system mounted successfully")

	logger.Infof("Step 3: Write to file system")
	_, wfsErr := rfc.FSWrite(filePodName, fileMountPath, "Smoke Test Data for file system storage", "fsFile1", namespace)
	require.Nil(s.T(), wfsErr)
	logger.Infof("Write to file system successful")

	logger.Infof("Step 4: Read from file system")
	read, rdErr := rfc.FSRead(filePodName, fileMountPath, "fsFile1", namespace)
	require.Nil(s.T(), rdErr)
	require.Contains(s.T(), read, "Smoke Test Data for file system storage", "make sure content of the files is unchanged")
	logger.Infof("Read from file system successful")

	logger.Infof("Step 5: UnMount file System")
	umtfsErr := podWithFileSystem(k8sh, s, filePodName, namespace, fileSystemName, fileMountPath, "delete")
	require.Nil(s.T(), umtfsErr)
	require.True(s.T(), k8sh.IsPodTerminated(filePodName, namespace), "make sure file-test pod is terminated")
	logger.Infof("File system mounted successfully")

	logger.Infof("Step 6: Deleting file storage")
	rfc.FSDelete(fileSystemName)
	//Delete is not deleting filesystem - known issue
	//require.Nil(suite.T(), fsd_err)
	logger.Infof("File system deleted")
}

//Test File System Creation on Rook that was installed on a custom namespace i.e. Namespace != "rook"
func runFileE2ETestLite(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, fileSystemName string) {
	logger.Infof("File Storage End to End Integration Test - create FileSystem and make sure mds pod is running")
	logger.Infof("Running on Rook Cluster %s", namespace)
	fc := helper.GetFileSystemClient()

	logger.Infof("Step 1: Create file System")
	_, fscErr := fc.FSCreate(fileSystemName)
	require.Nil(s.T(), fscErr)
	fileSystemList, _ := fc.FSList()
	require.Equal(s.T(), 1, len(fileSystemList), "There should one shared file system present")

	logger.Infof("Step 2: Make sure rook-ceph-mds pod is running")
	require.True(s.T(), k8sh.IsPodInExpectedState("rook-ceph-mds", namespace, "Running"),
		"Make sure rook-ceph-mds is in running state")

	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-mds", namespace, 1, "Running"),
		"Make sure there is 1 rook-ceph-mds present in Running state")

}

func fileTestDataCleanUp(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, podname string, namespace string, fileSystemName string, fileMountPath string) {
	logger.Infof("Cleaning up file system")
	podWithFileSystem(k8sh, s, podname, namespace, fileSystemName, fileMountPath, "delete")
	helper.GetFileSystemClient().FSDelete(fileSystemName)

}

func podWithFileSystem(k8sh *utils.K8sHelper, s suite.Suite, podname string, namespace string, filesystemName string, fileMountPath string, action string) error {
	mons, err := k8sh.GetMonitorServices(namespace)
	require.Nil(s.T(), err)

	logger.Infof("mountFileStorage: Mons: %+v", mons)
	_, err = k8sh.ResourceOperationFromTemplate(action, getFileSystemTestPod(podname, namespace, filesystemName, fileMountPath), mons)
	if err != nil {
		return fmt.Errorf("failed to %s pod -- %s. %+v", action, getFileSystemTestPod(podname, namespace, filesystemName, fileMountPath), err)
	}
	return nil
}

func getFileSystemTestPod(podname string, namespace string, filesystemName string, fileMountPath string) string {
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
    cephfs:
      monitors:
      - {{.mon0}}
      - {{.mon1}}
      - {{.mon2}}
      user: admin
      secretRef:
        name: rook-admin
      readOnly: false
  restartPolicy: Never
`
}
