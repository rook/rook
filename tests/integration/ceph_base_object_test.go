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

	"time"

	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	userid           = "rook-user"
	userdisplayname  = "A rook RGW user"
	bucketname       = "smokebkt"
	objBody          = "Test Rook Object Data"
	objectKey        = "rookObj1"
	contentType      = "plain/text"
	storageClassName = "rook-smoke-delete-bucket"
	obcName          = "smoke-delete-bucket"
	region           = "us-east-1"
)

// Smoke Test for ObjectStore - Test check the following operations on ObjectStore in order
// Create object store, Create User, Connect to Object Store, Create Bucket, Read/Write/Delete to bucket,
// Check issues in MGRs, Delete Bucket and Delete user
func runObjectE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string) {
	storeName := "teststore"
	defer objectTestDataCleanUp(helper, k8sh, namespace, storeName)

	logger.Infof("Object Storage End To End Integration Test - Create Object Store, User,Bucket and read/write to bucket")
	logger.Infof("Running on Rook Cluster %s", namespace)

	logger.Infof("Step 0 : Create Object Store")
	cobsErr := helper.ObjectClient.Create(namespace, storeName, 3)
	// check that ObjectStore is created
	require.Nil(s.T(), cobsErr)
	logger.Infof("Object store created successfully")

	logger.Infof("Step 1 : Create Object Store User")
	cosuErr := helper.ObjectUserClient.Create(namespace, userid, userdisplayname, storeName)
	// check that ObjectUser is created
	require.Nil(s.T(), cosuErr)
	logger.Infof("Waiting 10 seconds to ensure user was created")
	time.Sleep(10 * time.Second)
	logger.Infof("Checking to see if the user secret has been created")
	i := 0
	for i = 0; i < 4 && helper.ObjectUserClient.UserSecretExists(namespace, storeName, userid) == false; i++ {
		logger.Infof("(%d) secret check sleeping for 5 seconds ...", i)
		time.Sleep(5 * time.Second)
	}
	assert.True(s.T(), helper.ObjectUserClient.UserSecretExists(namespace, storeName, userid))
	userInfo, err := helper.ObjectUserClient.GetUser(namespace, storeName, userid)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), userid, userInfo.UserID)
	assert.Equal(s.T(), userdisplayname, *userInfo.DisplayName)
	logger.Infof("Done creating object store user")

	logger.Infof("Step 2 : Test Deleting User")
	dosuErr := helper.ObjectUserClient.Delete(namespace, userid)
	require.Nil(s.T(), dosuErr)
	logger.Infof("Object store user deleted successfully")
	logger.Infof("Checking to see if the user secret has been deleted")
	i = 0
	for i = 0; i < 4 && helper.ObjectUserClient.UserSecretExists(namespace, storeName, userid) == true; i++ {
		logger.Infof("(%d) secret check sleeping for 5 seconds ...", i)
		time.Sleep(5 * time.Second)
	}
	assert.False(s.T(), helper.ObjectUserClient.UserSecretExists(namespace, storeName, userid))

	logger.Infof("Check that MGRs are not in a crashloop")
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-mgr", namespace, 1, "Running"))
	logger.Infof("Ceph MGRs are running")

	// Testing creation/deletion of objects using Object Bucket Claim
	logger.Infof("Step 3 : Create Object Bucket Claim with reclaim policy delete")
	cobErr := helper.BucketClient.CreateBucketStorageClass(namespace, storeName, storageClassName, "Delete", region)
	require.Nil(s.T(), cobErr)
	cobcErr := helper.BucketClient.CreateObc(obcName, storageClassName, bucketname, true)
	require.Nil(s.T(), cobcErr)

	for i = 0; i < 4 && !helper.BucketClient.CheckOBC(obcName, "created"); i++ {
		logger.Infof("(%d) obc created check, sleeping for 5 seconds ...", i)
		time.Sleep(5 * time.Second)
	}
	require.NotEqual(s.T(), i, 4)

	logger.Infof("Check if bucket was created")
	context := k8sh.MakeContext()
	rgwcontext := rgw.NewContext(context, storeName, namespace)
	var bkt rgw.ObjectBucket
	for i = 0; i < 4; i++ {
		b, _, err := rgw.GetBucket(rgwcontext, bucketname)
		if b != nil && err == nil {
			bkt = *b
			break
		}
		logger.Infof("(%d) check bucket exists, sleeping for 5 seconds ...", i)
		time.Sleep(5 * time.Second)
	}
	require.Equal(s.T(), bkt.Name, bucketname)
	logger.Infof("OBC, Secret and ConfigMap created")

	logger.Infof("Step 4 : Create s3 client")
	s3endpoint, _ := helper.ObjectClient.GetEndPointUrl(namespace, storeName)
	s3AccessKey, _ := helper.BucketClient.GetAccessKey(obcName)
	s3SecretKey, _ := helper.BucketClient.GetSecretKey(obcName)
	s3client := utils.CreateNewS3Helper(s3endpoint, s3AccessKey, s3SecretKey)
	logger.Infof("endpoint (%s) Accesskey (%s) secret (%s)", s3endpoint, s3AccessKey, s3SecretKey)

	logger.Infof("Step 5 : Put Object on bucket")
	_, poErr := s3client.PutObjectInBucket(bucketname, objBody, objectKey, contentType)
	require.Nil(s.T(), poErr)

	logger.Infof("Step 6 : Get Object from bucket")
	read, err := s3client.GetObjectInBucket(bucketname, objectKey)
	require.Nil(s.T(), err)
	require.Equal(s.T(), objBody, read)
	logger.Infof("Object Created and Retrieved on bucket successfully")

	logger.Infof("Step 7 : Delete object on bucket")
	_, delobjErr := s3client.DeleteObjectInBucket(bucketname, objectKey)
	require.Nil(s.T(), delobjErr)
	logger.Infof("Object deleted on bucket successfully")

	logger.Infof("Step 8 : Delete Object Bucket Claim")
	dobcErr := helper.BucketClient.DeleteObc(obcName, storageClassName, bucketname, true)
	require.Nil(s.T(), dobcErr)
	logger.Infof("Checking to see if the obc, secret and cm have all been deleted")
	for i = 0; i < 4 && !helper.BucketClient.CheckOBC(obcName, "deleted"); i++ {
		logger.Infof("(%d) obc deleted check, sleeping for 5 seconds ...", i)
		time.Sleep(5 * time.Second)
	}
	assert.NotEqual(s.T(), i, 4)

	logger.Infof("ensure bucket was deleted")
	var rgwErr int
	for i = 0; i < 4; i++ {
		_, rgwErr, _ = rgw.GetBucket(rgwcontext, bucketname)
		if rgwErr == rgw.RGWErrorNotFound {
			break
		}
		logger.Infof("(%d) check bucket deleted, sleeping for 5 seconds ...", i)
		time.Sleep(5 * time.Second)
	}
	assert.NotEqual(s.T(), i, 4)
	assert.Equal(s.T(), rgwErr, rgw.RGWErrorNotFound)

	dobErr := helper.BucketClient.DeleteBucketStorageClass(namespace, storeName, storageClassName, "Delete", region)
	assert.Nil(s.T(), dobErr)
	logger.Infof("Delete Object Bucket Claim successfully")

	// TODO : Add case for brownfield/cleanup s3 client

	logger.Infof("Delete Object Store")
	dobsErr := helper.ObjectClient.Delete(namespace, storeName)
	assert.Nil(s.T(), dobsErr)
	logger.Infof("Done deleting object store")
}

// Test Object StoreCreation on Rook that was installed via helm
func runObjectE2ETestLite(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, name string, replicaSize int) {
	logger.Infof("Object Storage End To End Integration Test - Create Object Store and check if rgw service is Running")
	logger.Infof("Running on Rook Cluster %s", namespace)

	logger.Infof("Step 1 : Create Object Store")
	err := helper.ObjectClient.Create(namespace, name, int32(replicaSize))
	require.Nil(s.T(), err)

	logger.Infof("Step 2 : check rook-ceph-rgw service status and count")
	require.True(s.T(), k8sh.IsPodInExpectedState("rook-ceph-rgw", namespace, "Running"),
		"Make sure rook-ceph-rgw is in running state")

	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-rgw", namespace, replicaSize, "Running"),
		"Make sure all rook-ceph-rgw pods are in Running state")

	require.True(s.T(), k8sh.IsServiceUp("rook-ceph-rgw-"+name, namespace))

}

func objectTestDataCleanUp(helper *clients.TestClient, k8sh *utils.K8sHelper, namespace, storeName string) {
	logger.Infof("FIX: Cleaning up object store")
	/*oc := helper.ObjectClient
	userinfo, err := helper.ObjectClient.ObjectGetUser(storeName, userid)
	if err != nil {
		return //when user is not found
	}
	s3endpoint, _ := k8sh.GetRGWServiceURL(storeName, namespace)
	s3client := utils.CreateNewS3Helper(s3endpoint, *userinfo.AccessKey, *userinfo.SecretKey)
	s3client.DeleteObjectInBucket(bucketname, objectKey)
	s3client.DeleteBucket(bucketname)
	helper.ObjectClient.DeleteUser(storeName, userid)*/
}

func getBucket(bucketname string, bucketList []rgw.ObjectBucket) (rgw.ObjectBucket, error) {
	for _, bucket := range bucketList {
		if bucket.Name == bucketname {
			return bucket, nil
		}
	}
	return rgw.ObjectBucket{}, errors.New("Bucket not found")
}

func getBucketSizeAndObjects(bucketname string, bucketList []rgw.ObjectBucket) (uint64, uint64, error) {
	bkt, err := getBucket(bucketname, bucketList)
	if err != nil {
		return 0, 0, errors.New("Bucket not found")
	}
	return bkt.Size, bkt.NumberOfObjects, nil
}
