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
	"errors"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
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
//Create object store, Create User, Connect to Object Store, Create Bucket, Read/Write/Delete to bucket,Delete Bucket and
//Delete user
func (suite *SmokeSuite) TestObjectStorage_SmokeTest() {
	defer suite.blockTestDataCleanUp()
	logger.Infof("Object Storage Smoke Test - Create Object Store, User,Bucket and read/write to bucket")

	logger.Infof("Step 0 : Create Object Store")
	_, cobsErr := suite.helper.CreateObjectStore()
	require.Nil(suite.T(), cobsErr)
	logger.Infof("Object store created successfully")

	logger.Infof("Step 1 : Create Object Store User")
	initialUsers, _ := suite.helper.GetObjectStoreUsers()
	_, cosuErr := suite.helper.CreateObjectStoreUser()
	require.Nil(suite.T(), cosuErr)
	usersAfterCrate, _ := suite.helper.GetObjectStoreUsers()
	require.Equal(suite.T(), len(initialUsers)+1, len(usersAfterCrate), "Make sure user list count is increaded by 1")
	getuserData, guErr := suite.helper.GetObjectStoreUser(userid)
	require.Nil(suite.T(), guErr)
	require.Equal(suite.T(), userid, getuserData.UserID, "Check user id returned")
	require.Equal(suite.T(), userdisplayname, *getuserData.DisplayName, "Check user name returned")
	logger.Infof("Object store user created successfully")

	logger.Infof("Step 2 : Get connection information")
	conninfo, conninfoError := suite.helper.GetObjectStoreUser(userid)
	require.Nil(suite.T(), conninfoError)

	s3endpoint, _ := suite.helper.GetRGWServiceURL()
	s3client := utils.CreateNewS3Helper(s3endpoint, *conninfo.AccessKey, *conninfo.SecretKey)

	logger.Infof("Step 3 : Create bucket")
	initialBuckets, _ := suite.helper.GetObjectStoreBucketList()
	s3client.CreateBucket(bucketname)
	BucketsAfterCreate, _ := suite.helper.GetObjectStoreBucketList()
	require.Equal(suite.T(), len(initialBuckets)+1, len(BucketsAfterCreate), "Make sure new bucket is created")
	bkt, _ := getBucket(bucketname, BucketsAfterCreate)
	require.Equal(suite.T(), bucketname, bkt.Name)
	require.Equal(suite.T(), userid, bkt.Owner)
	logger.Infof("Bucket created in Object store successfully")

	logger.Infof("Step 4 : Put Object on bucket")
	initObjSize, initObjNum, _ := getBucketSizeAndObjectes(bucketname, BucketsAfterCreate)
	require.Equal(suite.T(), uint64(0), initObjSize)
	require.Equal(suite.T(), uint64(0), initObjNum)
	_, poErr := s3client.PutObjectInBucket(bucketname, objBody, objectKey, contentType)
	require.Nil(suite.T(), poErr)
	BucketsAfterPut, _ := suite.helper.GetObjectStoreBucketList()
	ObjSize, ObjNum, _ := getBucketSizeAndObjectes(bucketname, BucketsAfterPut)
	require.NotEmpty(suite.T(), ObjSize)
	require.Equal(suite.T(), uint64(1), ObjNum)
	logger.Infof("Object Created on bucket successfully")

	logger.Infof("Step 5 : Put Object from bucket")
	read, err := s3client.GetObjectInBucket(bucketname, objectKey)
	require.Nil(suite.T(), err)
	require.Equal(suite.T(), objBody, read)
	logger.Infof("Object retrived from bucket successfully")

	logger.Infof("Step 6 : Delete Object on bucket")
	_, delobjErr := s3client.DeleteObjectInBucket(bucketname, objectKey)
	require.Nil(suite.T(), delobjErr)
	BucketsAfterOjbDelete, _ := suite.helper.GetObjectStoreBucketList()
	ObjSize1, ObjNum1, _ := getBucketSizeAndObjectes(bucketname, BucketsAfterOjbDelete)
	require.Equal(suite.T(), uint64(0), ObjSize1)
	require.Equal(suite.T(), uint64(0), ObjNum1)
	logger.Infof("Object deleted on bucket successfully")

	logger.Infof("Step 6 : Delete  bucket")
	_, bkdelErr := s3client.DeleteBucket(bucketname)
	require.Nil(suite.T(), bkdelErr)
	BucketsAfterDelete, _ := suite.helper.GetObjectStoreBucketList()
	require.Equal(suite.T(), len(initialBuckets), len(BucketsAfterDelete), "Make sure new bucket is deleted")
	logger.Infof("Bucket  deleted successfully")

	logger.Infof("Step 7 : Delete  User")
	usersBeforeDelete, _ := suite.helper.GetObjectStoreUsers()
	suite.helper.DeleteObjectStoreUser()
	usersAfterDelete, _ := suite.helper.GetObjectStoreUsers()
	require.Equal(suite.T(), len(usersBeforeDelete)-1, len(usersAfterDelete), "Make sure user list count is reducd by 1")
	logger.Infof("Object store user deleted successfully")

}

func (suite *SmokeSuite) objectTestDataCleanUp() {
	logger.Infof("Cleaning up object store")
	userinfo, err := suite.helper.GetObjectStoreUser(userid)
	if err != nil {
		return //when user is not found
	}
	s3endpoint, _ := suite.helper.GetRGWServiceURL()
	s3client := utils.CreateNewS3Helper(s3endpoint, *userinfo.AccessKey, *userinfo.SecretKey)
	s3client.DeleteObjectInBucket(bucketname, objectKey)
	s3client.DeleteBucket(bucketname)
	suite.helper.DeleteObjectStoreUser()

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
