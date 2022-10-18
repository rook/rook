/*
Copyright 2022 The Rook Authors. All rights reserved.

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
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// Smoke Test for File System Storage for CephNFS - Test check the following operations on Filesystem Storage in order
// Create,Mount,Write,Read,Unmount and Delete.
func runNFSFileE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, s *suite.Suite, settings *installer.TestCephSettings, filesystemName string) {
	defer fileTestDataCleanUp(helper, k8sh, s, filePodName, settings.Namespace, filesystemName)
	logger.Infof("Running on Rook Cluster %s", settings.Namespace)
	logger.Infof("File Storage End To End Integration Test for CephNFS- create, mount, write to, read from, and unmount")
	activeCount := 1
	createFilesystem(helper, k8sh, s, settings, filesystemName, activeCount)

	nfsClusterName := "my-nfs"
	err := helper.NFSClient.Create(settings.Namespace, nfsClusterName, 1)
	require.Nil(s.T(), err)

	if settings.TestNFSCSI {
		storageClassName := "nfs-storageclass"
		err = helper.NFSClient.CreateStorageClass(filesystemName, nfsClusterName, settings.OperatorNamespace, settings.Namespace, storageClassName)
		assert.NoError(s.T(), err)
		createFilesystemConsumerPod(helper, k8sh, s, settings, filesystemName, storageClassName)

		// Test reading and writing to the first pod
		err = writeAndReadToFilesystem(helper, k8sh, s, settings.Namespace, filePodName, "test_file")
		assert.NoError(s.T(), err)

		cleanupFilesystemConsumer(helper, k8sh, s, settings.Namespace, filePodName)
		assert.NoError(s.T(), err)

		fileSystemCSISnapshotTest(helper, k8sh, s, storageClassName, settings.Namespace, true)

		fileSystemCSICloneTest(helper, k8sh, s, storageClassName, settings.Namespace)
	}

	err = helper.NFSClient.Delete(settings.Namespace, nfsClusterName)
	assert.Nil(s.T(), err)
	cleanupFilesystem(helper, k8sh, s, settings.Namespace, filesystemName)
}
