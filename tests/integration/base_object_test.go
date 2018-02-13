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
	"errors"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	userid          = "rook-user"
	userdisplayname = "A rook RGW user"
	bucketname      = "smokebkt"
	objBody         = "Test Rook Object Data"
	objectKey       = "rookObj1"
	contentType     = "plain/text"
)

// Smoke Test for ObjectStore - Test check the following operations on ObjectStore in order
// Create object store, Create User, Connect to Object Store, Create Bucket, Read/Write/Delete to bucket,
// Check issues in MGRs, Delete Bucket and Delete user
func runObjectE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string) {
	storeName := "teststore"
	defer objectTestDataCleanUp(helper, k8sh, namespace, storeName)
	oc := helper.GetObjectClient()

	logger.Infof("Object Storage End To End Integration Test - Create Object Store, User,Bucket and read/write to bucket")
	logger.Infof("Running on Rook Cluster %s", namespace)

	logger.Infof("Step 0 : Create Object Store")
	cobsErr := oc.ObjectCreate(namespace, storeName, 3, true, k8sh)
	require.Nil(s.T(), cobsErr)
	logger.Infof("Object store created successfully")

	logger.Infof("Step 1 : Create Object Store User")
	initialUsers, _ := oc.ObjectListUser(storeName)
	_, cosuErr := oc.ObjectCreateUser(storeName, userid, userdisplayname)
	require.Nil(s.T(), cosuErr)
	usersAfterCreate, _ := oc.ObjectListUser(storeName)
	require.Equal(s.T(), len(initialUsers)+1, len(usersAfterCreate), "Make sure user list count is increased by 1")
	getuserData, guErr := oc.ObjectGetUser(storeName, userid)
	require.Nil(s.T(), guErr)
	require.Equal(s.T(), userid, getuserData.UserID, "Check user id returned")
	require.Equal(s.T(), userdisplayname, *getuserData.DisplayName, "Check user name returned")
	logger.Infof("Object store user created successfully")

	logger.Infof("Step 2 : Get connection information")
	conninfo, conninfoError := oc.ObjectGetUser(storeName, userid)
	require.Nil(s.T(), conninfoError)
	s3endpoint, _ := k8sh.GetRGWServiceURL(storeName, namespace)
	s3client := utils.CreateNewS3Helper(s3endpoint, *conninfo.AccessKey, *conninfo.SecretKey)

	logger.Infof("Step 3 : Create bucket")
	initialBuckets, _ := oc.ObjectBucketList(storeName)
	s3client.CreateBucket(bucketname)
	BucketsAfterCreate, _ := oc.ObjectBucketList(storeName)
	require.Equal(s.T(), len(initialBuckets)+1, len(BucketsAfterCreate), "Make sure new bucket is created")
	bkt, _ := getBucket(bucketname, BucketsAfterCreate)
	require.Equal(s.T(), bucketname, bkt.Name)
	require.Equal(s.T(), userid, bkt.Owner)
	logger.Infof("Bucket created in Object store successfully")

	logger.Infof("Step 4 : Put Object on bucket")
	initObjSize, initObjNum, _ := getBucketSizeAndObjectes(bucketname, BucketsAfterCreate)
	require.Equal(s.T(), uint64(0), initObjSize)
	require.Equal(s.T(), uint64(0), initObjNum)
	_, poErr := s3client.PutObjectInBucket(bucketname, objBody, objectKey, contentType)
	require.Nil(s.T(), poErr)
	BucketsAfterPut, _ := oc.ObjectBucketList(storeName)
	ObjSize, ObjNum, _ := getBucketSizeAndObjectes(bucketname, BucketsAfterPut)
	require.NotEmpty(s.T(), ObjSize)
	require.Equal(s.T(), uint64(1), ObjNum)
	logger.Infof("Object Created on bucket successfully")

	logger.Infof("Step 5 : Put Object from bucket")
	read, err := s3client.GetObjectInBucket(bucketname, objectKey)
	require.Nil(s.T(), err)
	require.Equal(s.T(), objBody, read)
	logger.Infof("Object retrieved from bucket successfully")

	logger.Infof("Step 6 : Delete Object on bucket")
	_, delobjErr := s3client.DeleteObjectInBucket(bucketname, objectKey)
	require.Nil(s.T(), delobjErr)
	BucketsAfterOjbDelete, _ := oc.ObjectBucketList(storeName)
	ObjSize1, ObjNum1, _ := getBucketSizeAndObjectes(bucketname, BucketsAfterOjbDelete)
	require.Equal(s.T(), uint64(0), ObjSize1)
	require.Equal(s.T(), uint64(0), ObjNum1)
	logger.Infof("Object deleted on bucket successfully")

	logger.Infof("Step 7 : Delete bucket")
	_, bkdelErr := s3client.DeleteBucket(bucketname)
	require.Nil(s.T(), bkdelErr)
	BucketsAfterDelete, _ := oc.ObjectBucketList(storeName)
	require.Equal(s.T(), len(initialBuckets), len(BucketsAfterDelete), "Make sure new bucket is deleted")
	logger.Infof("Bucket  deleted successfully")

	logger.Infof("Step 7 : Delete  User")
	usersBeforeDelete, _ := oc.ObjectListUser(storeName)
	oc.ObjectDeleteUser(storeName, userid)
	usersAfterDelete, _ := oc.ObjectListUser(storeName)
	require.Equal(s.T(), len(usersBeforeDelete)-1, len(usersAfterDelete), "Make sure user list count is reducd by 1")
	logger.Infof("Object store user deleted successfully")

	logger.Infof("Step 9: Check that MGRs are not in a crashloop")
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-mgr", namespace, 1, "Running"))
	logger.Infof("Ceph MGRs are running alright")

	logger.Infof("Step 10: Delete Object Store")
	dobsErr := oc.ObjectDelete(namespace, storeName, 1, true, k8sh)
	require.Nil(s.T(), dobsErr)
	logger.Infof("Object store deleted successfully")

}

//Test Object StoreCreation on Rook that was installed via helm
func runObjectE2ETestLite(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, name string, replicaSize int) {
	logger.Infof("Object Storage End To End Integration Test - Create Object Store and check if rgw service is Running")
	logger.Infof("Running on Rook Cluster %s", namespace)

	logger.Infof("Step 1 : Create Object Store")
	oc := helper.GetObjectClient()
	err := oc.ObjectCreate(namespace, name, int32(replicaSize), false, k8sh)
	require.Nil(s.T(), err)

	logger.Infof("Step 2 : check rook-ceph-rgw service status and count")
	require.True(s.T(), k8sh.IsPodInExpectedState("rook-ceph-rgw", namespace, "Running"),
		"Make sure rook-ceph-rgw is in running state")

	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-rgw", namespace, replicaSize, "Running"),
		"Make sure all rook-ceph-rgw pods are in Running state")

	require.True(s.T(), k8sh.IsServiceUp("rook-ceph-rgw-"+name, namespace))

}

func objectTestDataCleanUp(helper *clients.TestClient, k8sh *utils.K8sHelper, namespace, storeName string) {
	logger.Infof("Cleaning up object store")
	oc := helper.GetObjectClient()
	userinfo, err := oc.ObjectGetUser(storeName, userid)
	if err != nil {
		return //when user is not found
	}
	s3endpoint, _ := k8sh.GetRGWServiceURL(storeName, namespace)
	s3client := utils.CreateNewS3Helper(s3endpoint, *userinfo.AccessKey, *userinfo.SecretKey)
	s3client.DeleteObjectInBucket(bucketname, objectKey)
	s3client.DeleteBucket(bucketname)
	oc.ObjectDeleteUser(storeName, userid)
}

func getBucket(bucketname string, bucketList []model.ObjectBucket) (model.ObjectBucket, error) {

	for _, bucket := range bucketList {
		if bucket.Name == bucketname {
			return bucket, nil
		}
	}

	return model.ObjectBucket{}, errors.New("Bucket not found")
}

func getBucketSizeAndObjectes(bucketname string, bucketList []model.ObjectBucket) (uint64, uint64, error) {
	bkt, err := getBucket(bucketname, bucketList)
	if err != nil {
		return 0, 0, errors.New("Bucket not found")
	}
	return bkt.Size, bkt.NumberOfObjects, nil
}
