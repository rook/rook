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
	"fmt"

	"github.com/stretchr/testify/require"
)

var (
	fileSystemName   = "testfs"
	fileMountPath    = "/tmp/rookfs"
	filePodName      = "file-test"
	filePodNamespace = "rook"
)

// Smoke Test for File System Storage - Test check the following operations on FileSystem Storage in order
//Create,Mount,Write,Read,Unmount and Delete.
func (suite *SmokeSuite) TestFileStorage_SmokeTest() {
	defer suite.fileTestDataCleanUp()
	logger.Infof("File Storage Smoke Test - create, mount, write to, read from, and unmount")
	rfc := suite.helper.GetFileSystemClient()

	logger.Infof("Step 1: Create file System")
	_, fscErr := rfc.FSCreate(fileSystemName)
	require.Nil(suite.T(), fscErr)
	fileSystemList, _ := rfc.FSList()
	require.Equal(suite.T(), 1, len(fileSystemList), "There should one shared file system present")
	filesystemData := fileSystemList[0]
	require.Equal(suite.T(), "testfs", filesystemData.Name, "make sure filesystem name matches")
	logger.Infof("File system created")

	logger.Infof("Step 2: Mount file System")
	mtfsErr := suite.podWithFileSystem("create")
	require.Nil(suite.T(), mtfsErr)
	require.True(suite.T(), suite.k8sh.IsPodRunningInNamespace(filePodName), "make sure file-test pod is in running state")
	logger.Infof("File system mounted successfully")

	logger.Infof("Step 3: Write to file system")
	_, wfsErr := rfc.FSWrite(filePodName, fileMountPath, "Smoke Test Data for file system storage", "fsFile1", filePodNamespace)
	require.Nil(suite.T(), wfsErr)
	logger.Infof("Write to file system successful")

	logger.Infof("Step 4: Read from file system")
	read, rdErr := rfc.FSRead(filePodName, fileMountPath, "fsFile1", filePodNamespace)
	require.Nil(suite.T(), rdErr)
	require.Contains(suite.T(), read, "Smoke Test Data for file system storage", "make sure content of the files is unchanged")
	logger.Infof("Read from file system successful")

	logger.Infof("Step 5: UnMount file System")
	umtfsErr := suite.podWithFileSystem("delete")
	require.Nil(suite.T(), umtfsErr)
	require.True(suite.T(), suite.k8sh.IsPodTerminatedInNamespace(filePodName), "make sure file-test pod is terminated")
	logger.Infof("File system mounted successfully")

	logger.Infof("Step 6: Deleting file storage")
	rfc.FSDelete(fileSystemName)
	//Delete is not deleting filesystem - known issue
	//require.Nil(suite.T(), fsd_err)
	logger.Infof("File system deleted")
}

func (suite *SmokeSuite) fileTestDataCleanUp() {
	logger.Infof("Cleaning up file system")
	suite.podWithFileSystem("delete")
	suite.helper.GetFileSystemClient().FSDelete(fileSystemName)

}

func (suite *SmokeSuite) podWithFileSystem(action string) error {
	mons, err := suite.k8sh.GetMonitorServices()
	require.Nil(suite.T(), err)

	logger.Infof("mountFileStorage: Mons: %+v", mons)
	_, err = suite.k8sh.ResourceOperationFromTemplate(action, getFileSystemTestPod(), mons)
	if err != nil {
		return fmt.Errorf("failed to %s pod -- %s. %+v", action, getFileSystemTestPod(), err)
	}
	return nil
}

func getFileSystemTestPod() string {
	return `apiVersion: v1
kind: Pod
metadata:
  name: file-test
  namespace: rook
spec:
  containers:
  - name: file-test1
    image: busybox
    command:
        - sleep
        - "3600"
    imagePullPolicy: IfNotPresent
    env:
    volumeMounts:
    - mountPath: "/tmp/rookfs"
      name: testfs
  volumes:
  - name: testfs
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
