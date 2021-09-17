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
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

var (
	userid                 = "rook-user"
	userdisplayname        = "A rook RGW user"
	bucketname             = "smokebkt"
	ObjBody                = "Test Rook Object Data"
	ObjectKey1             = "rookObj1"
	ObjectKey2             = "rookObj2"
	ObjectKey3             = "rookObj3"
	ObjectKey4             = "rookObj4"
	contentType            = "plain/text"
	obcName                = "smoke-delete-bucket"
	region                 = "us-east-1"
	maxObject              = "2"
	newMaxObject           = "3"
	bucketStorageClassName = "rook-smoke-delete-bucket"
	maxBucket              = 1
	maxSize                = "100000"
	userCap                = "read"
)

// Test Object StoreCreation on Rook that was installed via helm
func runObjectE2ETestLite(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, settings *installer.TestCephSettings, name string, replicaSize int, deleteStore bool) {
	logger.Infof("Object Storage End To End Integration Test - Create Object Store and check if rgw service is Running")
	logger.Infof("Running on Rook Cluster %s", settings.Namespace)

	logger.Infof("Step 1 : Create Object Store")
	err := helper.ObjectClient.Create(settings.Namespace, name, int32(replicaSize), false)
	assert.Nil(s.T(), err)

	logger.Infof("Step 2 : check rook-ceph-rgw service status and count")
	assert.True(s.T(), k8sh.IsPodInExpectedState("rook-ceph-rgw", settings.Namespace, "Running"),
		"Make sure rook-ceph-rgw is in running state")

	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-rgw", settings.Namespace, replicaSize, "Running"),
		"Make sure all rook-ceph-rgw pods are in Running state")

	assert.True(s.T(), k8sh.IsServiceUp("rook-ceph-rgw-"+name, settings.Namespace))

	if deleteStore {
		logger.Infof("Delete Object Store")
		err = helper.ObjectClient.Delete(settings.Namespace, name)
		assert.Nil(s.T(), err)
		logger.Infof("Done deleting object store")
	}
}
