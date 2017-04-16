package smokeTest

import (
	"github.com/dangula/rook/e2e/rook-test-framework/enums"
	"github.com/stretchr/testify/assert"
	"testing"
	"errors"
	"github.com/dangula/rook/e2e/rook-test-framework/utils"
)

var (
	userid          = "rook-user"
	userdisplayname = "A rook RGW user"
	bucketname      = "smokebkt"
	objBody         = "Test Rook Object Data"
	objectKey       = "rookObj1"
	contentType     = "plain/text"
)

func setUpBeforeTest() {
	sc, _ := CreateSmokeTestClient(enums.Kubernetes)
	sc.CreateObjectStore()
}

func TestObjectStorage_SmokeTest(t *testing.T) {
	setUpBeforeTest()
	t.Log("Object Storage Smoke Test")
	sc, _ := CreateSmokeTestClient(enums.Kubernetes)
	defer objetTestcleanup()

	port, perr := sc.GetRgwPort()
	assert.Nil(t, perr)

	t.Log("Step 0 : Create Object Store")
	_, cobs_err := sc.CreateObjectStore()
	assert.Nil(t, cobs_err)
	t.Log("Object store created successfully")

	t.Log("Step 1 : Create Object Store User")
	initialUsers, _ := sc.GetObjectStoreUsers()
	_, cosu_err := sc.CreateObjectStoreUser()
	assert.Nil(t, cosu_err)
	usersAfterCrate, _ := sc.GetObjectStoreUsers()
	assert.Equal(t, len(initialUsers)+1, len(usersAfterCrate), "Make sure user list count is increaded by 1")
	getuserData, gu_err := sc.GetObjectStoreUser(userid)
	assert.Nil(t, gu_err)
	assert.Equal(t, userid, getuserData.UserId, "Check user id returned")
	assert.Equal(t, userdisplayname, getuserData.DisplayName, "Check user name returned")
	t.Log("Object store user created successfully")

	t.Log("Step 2 : Get connection information")
	conninfo, conninfo_error := sc.GetObjectStoreConnection(userid)
	assert.Nil(t, conninfo_error)

	//TODO - need to figure out how to expose rgw endpoint and get host ip
	s3endpoint := "http://172.17.4.201:" + port
	s3client := utils.CreateNewS3Helper(s3endpoint, conninfo.AwsAccessKey, conninfo.AwsSecretKey)

	// Using s3 client

	t.Log("Step 3 : Create bucket")
	initialBuckets, _ := sc.GetObjectStoreBucketList()
	s3client.CreateBucket(bucketname)
	BucketsAfterCreate, _ := sc.GetObjectStoreBucketList()
	assert.Equal(t, len(initialBuckets)+1, len(BucketsAfterCreate), "Make sure new bucket is created")
	bkt, _ := getBucket(bucketname, BucketsAfterCreate)
	assert.Equal(t, bucketname, bkt.Name)
	assert.Equal(t, userid, bkt.Owner)
	t.Log("Bucket created in Object store successfully")

	t.Log("Step 4 : Put Object on bucket")
	initObjSize, initObjNum, _ := getBucketSizeAndObjectes(bucketname, BucketsAfterCreate)
	assert.Equal(t, 0, initObjSize)
	assert.Equal(t, 0, initObjNum)
	_, po_err := s3client.PutObjectInBucket(bucketname, objBody, objectKey, contentType)
	assert.Nil(t, po_err)
	BucketsAfterPut, _ := sc.GetObjectStoreBucketList()
	ObjSize, ObjNum, _ := getBucketSizeAndObjectes(bucketname, BucketsAfterPut)
	assert.NotEmpty(t, 0, ObjSize)
	assert.Equal(t, 1, ObjNum)
	t.Log("Object Created on bucket successfully")

	t.Log("Step 5 : Put Object from bucket")
	read, err := s3client.GetObjectInBucket(bucketname, objectKey)
	assert.Nil(t, err)
	assert.Equal(t, objBody, read)
	t.Log("Object retrived from bucket successfully")

	//TODO - delete object from bucket, check bucket object count
	t.Log("Step 6 : Delete Object on bucket")
	_, delobj_err := s3client.DeleteObjectInBucket(bucketname, objectKey)
	assert.Nil(t, delobj_err)
	BucketsAfterOjbDelete, _ := sc.GetObjectStoreBucketList()
	ObjSize1, ObjNum1, _ := getBucketSizeAndObjectes(bucketname, BucketsAfterOjbDelete)
	assert.Equal(t, 0, ObjSize1)
	assert.Equal(t, 0, ObjNum1)
	t.Log("Object deleted on bucket successfully")

	//TODO - Delete bucket check if deleted
	t.Log("Step 6 : Delete  bucket")
	_, bkdel_err := s3client.DeleteBucket(bucketname)
	assert.Nil(t, bkdel_err)
	BucketsAfterDelete, _ := sc.GetObjectStoreBucketList()
	assert.Equal(t, len(initialBuckets), len(BucketsAfterDelete), "Make sure new bucket is deleted")
	t.Log("Bucket  deleted successfully")

	//rook opeations
	t.Log("Step 7 : Delete  User")
	usersBeforeDelete, _ := sc.GetObjectStoreUsers()
	_, dosu_err := sc.DeleteObjectStoreUser()
	assert.Nil(t, dosu_err)
	usersAfterDelete, _ := sc.GetObjectStoreUsers()
	assert.Equal(t, len(usersBeforeDelete)-1, len(usersAfterDelete), "Make sure user list count is reducd by 1")
	t.Log("Object store user created successfully")

}

func objetTestcleanup() {
	sc, _ := CreateSmokeTestClient(enums.Kubernetes)
	conninfo, _ := sc.GetObjectStoreConnection(userid)
	port, _ := sc.GetRgwPort()
	//TODO - need to figure out how to expose rgw endpoint and get host ip
	s3endpoint := "http://172.17.4.201:" + port
	s3client := utils.CreateNewS3Helper(s3endpoint, conninfo.AwsAccessKey, conninfo.AwsSecretKey)
	s3client.DeleteObjectInBucket(bucketname, objectKey)
	s3client.DeleteBucket(bucketname)
	sc.DeleteObjectStoreUser()
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
