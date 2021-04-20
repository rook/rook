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
	"context"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	userid          = "rook-user"
	userdisplayname = "A rook RGW user"
	bucketname      = "smokebkt"
	ObjBody         = "Test Rook Object Data"
	ObjectKey1      = "rookObj1"
	ObjectKey2      = "rookObj2"
	ObjectKey3      = "rookObj3"
	contentType     = "plain/text"
	obcName         = "smoke-delete-bucket"
	region          = "us-east-1"
	maxObject       = "2"
)

// Smoke Test for ObjectStore - Test check the following operations on ObjectStore in order
// Create object store, Create User, Connect to Object Store, Create Bucket, Read/Write/Delete to bucket,
// Check issues in MGRs, Delete Bucket and Delete user
func runObjectE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string) {
	ctx := context.TODO()
	storeName := "teststore"
	defer objectStoreCleanUp(s, helper, k8sh, namespace, storeName)

	logger.Infof("Object Storage End To End Integration Test - Create Object Store, User,Bucket and read/write to bucket")
	logger.Infof("Running on Rook Cluster %s", namespace)
	clusterInfo := client.AdminClusterInfo(namespace)

	logger.Infof("Step 0 : Create Object Store")
	cobsErr := helper.ObjectClient.Create(namespace, storeName, 3)
	assert.Nil(s.T(), cobsErr)

	// check that ObjectStore is created
	logger.Infof("Check that RGW pods are Running")
	for i := 0; i < 24 && k8sh.CheckPodCountAndState("rook-ceph-rgw", namespace, 1, "Running") == false; i++ {
		logger.Infof("(%d) RGW pod check sleeping for 5 seconds ...", i)
		time.Sleep(5 * time.Second)
	}
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-rgw", namespace, 1, "Running"))
	logger.Infof("RGW pods are running")
	logger.Infof("Object store created successfully")

	// check that ObjectUser is created
	logger.Infof("Step 1 : Create Object Store User")
	createCephObjectUser(s, helper, k8sh, namespace, storeName, userid, true)

	logger.Infof("Done creating object store user")

	// Check object store status
	var i int
	for i = 0; i < 4 && helper.ObjectUserClient.UserSecretExists(namespace, storeName, userid) == false; i++ {
		objectStore, err := k8sh.RookClientset.CephV1().CephObjectStores(namespace).Get(ctx, storeName, metav1.GetOptions{})
		assert.Nil(s.T(), err)
		if objectStore.Status == nil || objectStore.Status.BucketStatus == nil {
			logger.Infof("(%d) bucket status check sleeping for 5 seconds ...", i)
			time.Sleep(5 * time.Second)
			continue
		}
		assert.Equal(s.T(), cephv1.ConditionConnected, objectStore.Status.BucketStatus.Health)
		// Info field has the endpoint in it
		assert.NotEmpty(s.T(), objectStore.Status.Info)
		assert.NotEmpty(s.T(), objectStore.Status.Info["endpoint"])
		break
	}
	assert.NotEqual(s.T(), i, 4)

	logger.Infof("Step 2 : Test Deleting User")
	dosuErr := helper.ObjectUserClient.Delete(namespace, userid)
	assert.Nil(s.T(), dosuErr)
	logger.Infof("Object store user deleted successfully")
	logger.Infof("Checking to see if the user secret has been deleted")
	for i = 0; i < 4 && helper.ObjectUserClient.UserSecretExists(namespace, storeName, userid) == true; i++ {
		logger.Infof("(%d) secret check sleeping for 5 seconds ...", i)
		time.Sleep(5 * time.Second)
	}
	assert.False(s.T(), helper.ObjectUserClient.UserSecretExists(namespace, storeName, userid))
	assert.NotEqual(s.T(), i, 4)

	logger.Infof("Check that MGRs are not in a crashloop")
	assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-mgr", namespace, 1, "Running"))
	logger.Infof("Ceph MGRs are running")

	// Testing creation/deletion of objects using Object Bucket Claim
	logger.Infof("Step 3 : Create Object Bucket Claim with reclaim policy delete")
	bucketStorageClassName := "rook-smoke-delete-bucket"
	cobErr := helper.BucketClient.CreateBucketStorageClass(namespace, storeName, bucketStorageClassName, "Delete", region)
	assert.Nil(s.T(), cobErr)
	cobcErr := helper.BucketClient.CreateObc(obcName, bucketStorageClassName, bucketname, maxObject, true)
	assert.Nil(s.T(), cobcErr)

	created := utils.Retry(12, 2*time.Second, "OBC is created", func() bool {
		return helper.BucketClient.CheckOBC(obcName, "bound")
	})
	assert.True(s.T(), created)

	logger.Infof("Check if bucket was created")
	context := k8sh.MakeContext()
	rgwcontext := rgw.NewContext(context, clusterInfo, storeName)
	var bkt rgw.ObjectBucket
	for i = 0; i < 4; i++ {
		b, code, err := rgw.GetBucket(rgwcontext, bucketname)
		if b != nil && err == nil {
			bkt = *b
			break
		}
		logger.Warningf("cannot get bucket %q, retrying... bucket: %v. code: %d, err: %v", bucketname, b, code, err)
		logger.Infof("(%d) check bucket exists, sleeping for 5 seconds ...", i)
		time.Sleep(5 * time.Second)
	}
	assert.NotEqual(s.T(), i, 4)
	assert.Equal(s.T(), bucketname, bkt.Name)
	logger.Infof("OBC, Secret and ConfigMap created")

	logger.Infof("Step 4 : Create s3 client")
	s3endpoint, _ := helper.ObjectClient.GetEndPointUrl(namespace, storeName)
	s3AccessKey, _ := helper.BucketClient.GetAccessKey(obcName)
	s3SecretKey, _ := helper.BucketClient.GetSecretKey(obcName)
	s3client, err := rgw.NewS3Agent(s3AccessKey, s3SecretKey, s3endpoint, true, nil)
	assert.Nil(s.T(), err)
	logger.Infof("endpoint (%s) Accesskey (%s) secret (%s)", s3endpoint, s3AccessKey, s3SecretKey)

	logger.Infof("Step 5 : Put Object on bucket")
	_, poErr := s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey1, contentType)
	assert.Nil(s.T(), poErr)

	logger.Infof("Step 6 : Get Object from bucket")
	read, err := s3client.GetObjectInBucket(bucketname, ObjectKey1)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), ObjBody, read)
	logger.Infof("Object Created and Retrieved on bucket successfully")

	logger.Infof("Step 7 : Testing Quota for the OBC")
	logger.Infof("Adding one more object to the bucket")
	_, poErr = s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey2, contentType)
	assert.Nil(s.T(), poErr)
	logger.Infof("Testing the max object limit")
	_, poErr = s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey3, contentType)
	assert.Error(s.T(), poErr)

	logger.Infof("Step 8 : Delete objects on bucket")
	_, delobjErr := s3client.DeleteObjectInBucket(bucketname, ObjectKey1)
	assert.Nil(s.T(), delobjErr)
	_, delobjErr = s3client.DeleteObjectInBucket(bucketname, ObjectKey2)
	assert.Nil(s.T(), delobjErr)
	logger.Infof("Objects deleted on bucket successfully")

	logger.Infof("Step 9 : Delete Object Bucket Claim")
	dobcErr := helper.BucketClient.DeleteObc(obcName, bucketStorageClassName, bucketname, maxObject, true)
	assert.Nil(s.T(), dobcErr)
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

	dobErr := helper.BucketClient.DeleteBucketStorageClass(namespace, storeName, bucketStorageClassName, "Delete", region)
	assert.Nil(s.T(), dobErr)
	logger.Infof("Delete Object Bucket Claim successfully")

	// TODO : Add case for brownfield/cleanup s3 client}
}

// Test Object StoreCreation on Rook that was installed via helm
func runObjectE2ETestLite(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, settings *installer.TestCephSettings, name string, replicaSize int, deleteStore bool) {
	logger.Infof("Object Storage End To End Integration Test - Create Object Store and check if rgw service is Running")
	logger.Infof("Running on Rook Cluster %s", settings.Namespace)

	logger.Infof("Step 1 : Create Object Store")
	err := helper.ObjectClient.Create(settings.Namespace, name, int32(replicaSize))
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

func objectStoreCleanUp(s suite.Suite, helper *clients.TestClient, k8sh *utils.K8sHelper, namespace, storeName string) {
	logger.Infof("Delete Object Store (will fail if users and buckets still exist)")
	err := helper.ObjectClient.Delete(namespace, storeName)
	assert.Nil(s.T(), err)
	logger.Infof("Done deleting object store")
}

func createCephObjectUser(
	s suite.Suite, helper *clients.TestClient, k8sh *utils.K8sHelper,
	namespace, storeName, userID string,
	checkPhase bool,
) {
	s.T().Helper()

	cosuErr := helper.ObjectUserClient.Create(namespace, userID, userdisplayname, storeName)
	assert.Nil(s.T(), cosuErr)
	logger.Infof("Waiting 5 seconds for the object user to be created")
	time.Sleep(5 * time.Second)
	logger.Infof("Checking to see if the user secret has been created")
	for i := 0; i < 6 && helper.ObjectUserClient.UserSecretExists(namespace, storeName, userID) == false; i++ {
		logger.Infof("(%d) secret check sleeping for 5 seconds ...", i)
		time.Sleep(5 * time.Second)
	}

	checkCephObjectUser(s, helper, k8sh, namespace, storeName, userID, checkPhase)
}

func checkCephObjectUser(
	s suite.Suite, helper *clients.TestClient, k8sh *utils.K8sHelper,
	namespace, storeName, userID string,
	checkPhase bool,
) {
	s.T().Helper()

	logger.Infof("checking object store \"%s/%s\" user %q", namespace, storeName, userID)
	assert.True(s.T(), helper.ObjectUserClient.UserSecretExists(namespace, storeName, userID))

	userInfo, err := helper.ObjectUserClient.GetUser(namespace, storeName, userID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), userID, userInfo.UserID)
	assert.Equal(s.T(), userdisplayname, *userInfo.DisplayName)

	if checkPhase {
		// status.phase doesn't exist before Rook v1.6
		phase, err := k8sh.GetResource("--namespace", namespace, "cephobjectstoreuser", userID, "--output", "jsonpath={.status.phase}")
		assert.NoError(s.T(), err)
		assert.Equal(s.T(), k8sutil.ReadyStatus, phase)
	}
}
