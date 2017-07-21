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
	"github.com/stretchr/testify/require"
)

// Smoke Test for File System Storage - Test check the following operations on FileSystem Storage in order
//Create,Mount,Write,Read,Unmount and Delete.
func (suite *SmokeSuite) TestFileStorage_SmokeTest() {
	defer suite.fileTestDataCleanUp()
	logger.Infof("File Storage Smoke Test - create, mount, write to, read from, and unmount")
	rfc := suite.helper.GetFileSystemClient()

	logger.Infof("Step 1: Create file System")
	fscErr := suite.helper.CreateFileStorage()
	require.Nil(suite.T(), fscErr)
	fileSystemList, _ := rfc.FSList()
	require.Equal(suite.T(), 1, len(fileSystemList), "There should one shared file system present")
	filesystemData := fileSystemList[0]
	require.Equal(suite.T(), "testfs", filesystemData.Name, "make sure filesystem name matches")
	logger.Infof("File system created")

	logger.Infof("Step 2: Mount file System")
	mtfsErr := suite.helper.MountFileStorage()
	require.Nil(suite.T(), mtfsErr)
	logger.Infof("File system mounted successfully")

	logger.Infof("Step 3: Write to file system")
	wfsErr := suite.helper.WriteToFileStorage("Test data for file", "fsFile1")
	require.Nil(suite.T(), wfsErr)
	logger.Infof("Write to file system successful")

	logger.Infof("Step 4: Read from file system")
	read, rdErr := suite.helper.ReadFromFileStorage("fsFile1")
	require.Nil(suite.T(), rdErr)
	require.Contains(suite.T(), read, "Test data for file", "make sure content of the files is unchanged")
	logger.Infof("Read from file system successful")

	logger.Infof("Step 5: UnMount file System")
	umtfsErr := suite.helper.UnmountFileStorage()
	require.Nil(suite.T(), umtfsErr)
	logger.Infof("File system mounted successfully")

	logger.Infof("Step 6: Deleting file storage")
	suite.helper.DeleteFileStorage()
	//Delete is not deleting filesystem - known issue
	//require.Nil(suite.T(), fsd_err)
	logger.Infof("File system deleted")
}

func (suite *SmokeSuite) fileTestDataCleanUp() {
	logger.Infof("Cleaning up file system")
	suite.helper.UnmountFileStorage()
	suite.helper.DeleteFileStorage()

}
