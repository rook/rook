package smoke

import (
	"errors"
	"testing"

	"github.com/rook/rook/e2e/framework/enums"
	"github.com/rook/rook/e2e/framework/manager"
	"github.com/rook/rook/e2e/framework/utils"
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

func TestObjectStorageSmokeSuite(t *testing.T) {
	suite.Run(t, new(ObjectStorageTestSuite))
}

type ObjectStorageTestSuite struct {
	suite.Suite
	rookPlatform enums.RookPlatformType
	k8sVersion   enums.K8sVersion
	rookTag      string
	helper       *SmokeTestHelper
}

func (suite *ObjectStorageTestSuite) SetupTest() {

	var err error

	suite.rookPlatform, err = enums.GetRookPlatFormTypeFromString(env.Platform)

	require.Nil(suite.T(), err)

	suite.k8sVersion, err = enums.GetK8sVersionFromString(env.K8sVersion)

	require.Nil(suite.T(), err)

	suite.rookTag = env.RookTag

	require.NotEmpty(suite.T(), suite.rookTag, "RookTag parameter is required")

	err, rookInfra := rook_test_infra.GetRookTestInfraManager(suite.rookPlatform, true, suite.k8sVersion)

	require.Nil(suite.T(), err)

	rookInfra.ValidateAndSetupTestPlatform()

	err, _ = rookInfra.InstallRook(suite.rookTag)

	require.Nil(suite.T(), err)

	suite.helper, err = CreateSmokeTestClient(rookInfra.GetRookPlatform())
	require.Nil(suite.T(), err)

}
func (suite *ObjectStorageTestSuite) TestObjectStorage_SmokeTest() {

	suite.T().Log("Object Storage Smoke Test - Create Object Store, User,Bucket and read/write to bucket")

	defer objectTestcleanup(suite.helper)

	suite.T().Log("Step 0 : Create Object Store")
	_, cobs_err := suite.helper.CreateObjectStore()
	assert.Nil(suite.T(), cobs_err)
	suite.T().Log("Object store created successfully")

	suite.T().Log("Step 1 : Create Object Store User")
	initialUsers, _ := suite.helper.GetObjectStoreUsers()
	_, cosu_err := suite.helper.CreateObjectStoreUser()
	assert.Nil(suite.T(), cosu_err)
	usersAfterCrate, _ := suite.helper.GetObjectStoreUsers()
	assert.Equal(suite.T(), len(initialUsers)+1, len(usersAfterCrate), "Make sure user list count is increaded by 1")
	getuserData, gu_err := suite.helper.GetObjectStoreUser(userid)
	assert.Nil(suite.T(), gu_err)
	assert.Equal(suite.T(), userid, getuserData.UserId, "Check user id returned")
	assert.Equal(suite.T(), userdisplayname, getuserData.DisplayName, "Check user name returned")
	suite.T().Log("Object store user created successfully")

	suite.T().Log("Step 2 : Get connection information")
	conninfo, conninfo_error := suite.helper.GetObjectStoreConnection(userid)
	assert.Nil(suite.T(), conninfo_error)

	s3endpoint, _ := suite.helper.GetRGWServiceUrl()
	s3client := utils.CreateNewS3Helper(s3endpoint, conninfo.AwsAccessKey, conninfo.AwsSecretKey)

	suite.T().Log("Step 3 : Create bucket")
	initialBuckets, _ := suite.helper.GetObjectStoreBucketList()
	s3client.CreateBucket(bucketname)
	BucketsAfterCreate, _ := suite.helper.GetObjectStoreBucketList()
	assert.Equal(suite.T(), len(initialBuckets)+1, len(BucketsAfterCreate), "Make sure new bucket is created")
	bkt, _ := getBucket(bucketname, BucketsAfterCreate)
	assert.Equal(suite.T(), bucketname, bkt.Name)
	assert.Equal(suite.T(), userid, bkt.Owner)
	suite.T().Log("Bucket created in Object store successfully")

	suite.T().Log("Step 4 : Put Object on bucket")
	initObjSize, initObjNum, _ := getBucketSizeAndObjectes(bucketname, BucketsAfterCreate)
	assert.Equal(suite.T(), 0, initObjSize)
	assert.Equal(suite.T(), 0, initObjNum)
	_, po_err := s3client.PutObjectInBucket(bucketname, objBody, objectKey, contentType)
	assert.Nil(suite.T(), po_err)
	BucketsAfterPut, _ := suite.helper.GetObjectStoreBucketList()
	ObjSize, ObjNum, _ := getBucketSizeAndObjectes(bucketname, BucketsAfterPut)
	assert.NotEmpty(suite.T(), ObjSize)
	assert.Equal(suite.T(), 1, ObjNum)
	suite.T().Log("Object Created on bucket successfully")

	suite.T().Log("Step 5 : Put Object from bucket")
	read, err := s3client.GetObjectInBucket(bucketname, objectKey)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), objBody, read)
	suite.T().Log("Object retrived from bucket successfully")

	suite.T().Log("Step 6 : Delete Object on bucket")
	_, delobj_err := s3client.DeleteObjectInBucket(bucketname, objectKey)
	assert.Nil(suite.T(), delobj_err)
	BucketsAfterOjbDelete, _ := suite.helper.GetObjectStoreBucketList()
	ObjSize1, ObjNum1, _ := getBucketSizeAndObjectes(bucketname, BucketsAfterOjbDelete)
	assert.Equal(suite.T(), 0, ObjSize1)
	assert.Equal(suite.T(), 0, ObjNum1)
	suite.T().Log("Object deleted on bucket successfully")

	suite.T().Log("Step 6 : Delete  bucket")
	_, bkdel_err := s3client.DeleteBucket(bucketname)
	assert.Nil(suite.T(), bkdel_err)
	BucketsAfterDelete, _ := suite.helper.GetObjectStoreBucketList()
	assert.Equal(suite.T(), len(initialBuckets), len(BucketsAfterDelete), "Make sure new bucket is deleted")
	suite.T().Log("Bucket  deleted successfully")

	suite.T().Log("Step 7 : Delete  User")
	usersBeforeDelete, _ := suite.helper.GetObjectStoreUsers()
	_, dosu_err := suite.helper.DeleteObjectStoreUser()
	assert.Nil(suite.T(), dosu_err)
	usersAfterDelete, _ := suite.helper.GetObjectStoreUsers()
	assert.Equal(suite.T(), len(usersBeforeDelete)-1, len(usersAfterDelete), "Make sure user list count is reducd by 1")
	suite.T().Log("Object store user created successfully")

}

func objectTestcleanup(h *SmokeTestHelper) {
	conninfo, _ := h.GetObjectStoreConnection(userid)
	s3endpoint, _ := h.GetRGWServiceUrl()
	s3client := utils.CreateNewS3Helper(s3endpoint, conninfo.AwsAccessKey, conninfo.AwsSecretKey)
	s3client.DeleteObjectInBucket(bucketname, objectKey)
	s3client.DeleteBucket(bucketname)
	h.DeleteObjectStoreUser()
}

func getBucket(bucketname string, bucketdict map[string]utils.ObjectBucketListData) (utils.ObjectBucketListData, error) {
	if val, ok := bucketdict[bucketname]; ok {
		return val, nil
	} else {
		return utils.ObjectBucketListData{}, errors.New("Bucket not found")
	}
}

func getBucketSizeAndObjectes(bucketname string, bucketdict map[string]utils.ObjectBucketListData) (int, int, error) {
	bkt, err := getBucket(bucketname, bucketdict)
	if err != nil {
		return 0, 0, errors.New("Bucket not found")
	}
	return bkt.Size, bkt.NumberOfObjects, nil
}
