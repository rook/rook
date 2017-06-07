package smoke

import (
	"errors"
	"testing"

	"github.com/rook/rook/e2e/framework/utils"
	"github.com/rook/rook/e2e/tests"
	"github.com/rook/rook/pkg/model"
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

func TestObjectStorageSmokeSuite(t *testing.T) {
	suite.Run(t, new(ObjectStorageTestSuite))
}

type ObjectStorageTestSuite struct {
	suite.Suite
	helper *SmokeTestHelper
}

func (suite *ObjectStorageTestSuite) SetupTest() {

	var err error

	suite.helper, err = CreateSmokeTestClient(tests.Platform)
	require.Nil(suite.T(), err)

}

// Smoke Test for ObjectStore - Test check the following operations on ObjectStore in order
//Create object store, Create User, Connect to Object Store, Create Bucket, Read/Write/Delete to bucket,Delete Bucket and
//Delete user
func (suite *ObjectStorageTestSuite) TestObjectStorage_SmokeTest() {

	suite.T().Log("Object Storage Smoke Test - Create Object Store, User,Bucket and read/write to bucket")

	suite.T().Log("Step 0 : Create Object Store")
	_, cobs_err := suite.helper.CreateObjectStore()
	require.Nil(suite.T(), cobs_err)
	suite.T().Log("Object store created successfully")

	suite.T().Log("Step 1 : Create Object Store User")
	initialUsers, _ := suite.helper.GetObjectStoreUsers()
	_, cosu_err := suite.helper.CreateObjectStoreUser()
	require.Nil(suite.T(), cosu_err)
	usersAfterCrate, _ := suite.helper.GetObjectStoreUsers()
	require.Equal(suite.T(), len(initialUsers)+1, len(usersAfterCrate), "Make sure user list count is increaded by 1")
	getuserData, gu_err := suite.helper.GetObjectStoreUser(userid)
	require.Nil(suite.T(), gu_err)
	require.Equal(suite.T(), userid, getuserData.UserID, "Check user id returned")
	require.Equal(suite.T(), userdisplayname, *getuserData.DisplayName, "Check user name returned")
	suite.T().Log("Object store user created successfully")

	suite.T().Log("Step 2 : Get connection information")
	conninfo, conninfo_error := suite.helper.GetObjectStoreUser(userid)
	require.Nil(suite.T(), conninfo_error)

	s3endpoint, _ := suite.helper.GetRGWServiceUrl()
	s3client := utils.CreateNewS3Helper(s3endpoint, *conninfo.AccessKey, *conninfo.SecretKey)

	suite.T().Log("Step 3 : Create bucket")
	initialBuckets, _ := suite.helper.GetObjectStoreBucketList()
	s3client.CreateBucket(bucketname)
	BucketsAfterCreate, _ := suite.helper.GetObjectStoreBucketList()
	require.Equal(suite.T(), len(initialBuckets)+1, len(BucketsAfterCreate), "Make sure new bucket is created")
	bkt, _ := getBucket(bucketname, BucketsAfterCreate)
	require.Equal(suite.T(), bucketname, bkt.Name)
	require.Equal(suite.T(), userid, bkt.Owner)
	suite.T().Log("Bucket created in Object store successfully")

	suite.T().Log("Step 4 : Put Object on bucket")
	initObjSize, initObjNum, _ := getBucketSizeAndObjectes(bucketname, BucketsAfterCreate)
	require.Equal(suite.T(), uint64(0), initObjSize)
	require.Equal(suite.T(), uint64(0), initObjNum)
	_, po_err := s3client.PutObjectInBucket(bucketname, objBody, objectKey, contentType)
	require.Nil(suite.T(), po_err)
	BucketsAfterPut, _ := suite.helper.GetObjectStoreBucketList()
	ObjSize, ObjNum, _ := getBucketSizeAndObjectes(bucketname, BucketsAfterPut)
	require.NotEmpty(suite.T(), ObjSize)
	require.Equal(suite.T(), uint64(1), ObjNum)
	suite.T().Log("Object Created on bucket successfully")

	suite.T().Log("Step 5 : Put Object from bucket")
	read, err := s3client.GetObjectInBucket(bucketname, objectKey)
	require.Nil(suite.T(), err)
	require.Equal(suite.T(), objBody, read)
	suite.T().Log("Object retrived from bucket successfully")

	suite.T().Log("Step 6 : Delete Object on bucket")
	_, delobj_err := s3client.DeleteObjectInBucket(bucketname, objectKey)
	require.Nil(suite.T(), delobj_err)
	BucketsAfterOjbDelete, _ := suite.helper.GetObjectStoreBucketList()
	ObjSize1, ObjNum1, _ := getBucketSizeAndObjectes(bucketname, BucketsAfterOjbDelete)
	require.Equal(suite.T(), uint64(0), ObjSize1)
	require.Equal(suite.T(), uint64(0), ObjNum1)
	suite.T().Log("Object deleted on bucket successfully")

	suite.T().Log("Step 6 : Delete  bucket")
	_, bkdel_err := s3client.DeleteBucket(bucketname)
	require.Nil(suite.T(), bkdel_err)
	BucketsAfterDelete, _ := suite.helper.GetObjectStoreBucketList()
	require.Equal(suite.T(), len(initialBuckets), len(BucketsAfterDelete), "Make sure new bucket is deleted")
	suite.T().Log("Bucket  deleted successfully")

	suite.T().Log("Step 7 : Delete  User")
	usersBeforeDelete, _ := suite.helper.GetObjectStoreUsers()
	suite.helper.DeleteObjectStoreUser()
	usersAfterDelete, _ := suite.helper.GetObjectStoreUsers()
	require.Equal(suite.T(), len(usersBeforeDelete)-1, len(usersAfterDelete), "Make sure user list count is reducd by 1")
	suite.T().Log("Object store user deleted successfully")

}

func (s *ObjectStorageTestSuite) TearDownTest() {
	userinfo, err := s.helper.GetObjectStoreUser(userid)
	if err != nil {
		return //when user is not found
	}
	s3endpoint, _ := s.helper.GetRGWServiceUrl()
	s3client := utils.CreateNewS3Helper(s3endpoint, *userinfo.AccessKey, *userinfo.SecretKey)
	s3client.DeleteObjectInBucket(bucketname, objectKey)
	s3client.DeleteBucket(bucketname)
	s.helper.DeleteObjectStoreUser()

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
