package smokeTest

import (
	"github.com/dangula/rook/e2e/rook-test-framework/enums"
	"testing"
	"github.com/stretchr/testify/assert"
	"fmt"
)

func TestObjectStorage_SmokeTest(t *testing.T) {

	t.Log("Object Storage Smoke Test")
	sc, _ := CreateSmokeTestClient(enums.Kubernetes)
	rh := sc.rookHelp
	oc := sc.GetObjectClient()

	str,gus_err :=oc.Object_List_user()
	assert.Nil(t,gus_err)
	users := rh.ParserObjectUserListData(str)
	fmt.Println(users)

	str,gu_err := oc.Object_Get_user("rook-user")
	assert.Nil(t,gu_err)
	user := rh.ParserObjectUserData(str)
	fmt.Println(user)


	str,gc_err := oc.Object_Connection("rook-user")
	assert.Nil(t,gc_err)
	conn := rh.ParserObjectConnectionData(str)
	fmt.Println(conn)

	str,bg_err := oc.Object_Bucket_list()
	assert.Nil(t,bg_err)
	buckets := rh.ParserObjectBucketListData(str)
	fmt.Println(buckets)

	//Need to figure out how to enable nodePort on rgw service and extract the port out
	//Need to do the following steps

	//TODO - Create object store
	//TODO - Create object store user
	//TODO - extract connection infromation and user it in s3 client
	// Using s3 client
	//TODO - Create a bucket and verify it exists
	//TODO - put objet on bucket, check bucket object count
	//TODO - get object from bucket, check content
	//TODO - delete object from bucket, check bucket object count
	//TODO - Delete bucket check if deleted
	//rook opeations
	//TODO - Delete user and make sure it is deleted



}

