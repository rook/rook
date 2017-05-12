package clients

import (
	"github.com/rook/rook/e2e/framework/contracts"
	"github.com/rook/rook/pkg/model"
)

type ObjectOperation struct {
	restClient contracts.RestAPIOperator
}

func CreateObjectOperation(rookRestClient contracts.RestAPIOperator) *ObjectOperation {
	return &ObjectOperation{restClient: rookRestClient}
}

//Function to create a object store in rook
//Input paramatres -None
//Output - output returned by rook Rest API client
func (ro *ObjectOperation) ObjectCreate() (string, error) {
	return ro.restClient.CreateObjectStore()

}

//Function to get Buckets present in rook object store
//Input paramatres - None
//Output - output returned by rook Rest API client
func (ro *ObjectOperation) ObjectBucketList() ([]model.ObjectBucket, error) {
	return ro.restClient.ListBuckets()

}

//Function to get connection information for a user
//Input paramatres - None
//Output - output returned by rook Rest API client
func (ro *ObjectOperation) ObjectConnection() (*model.ObjectStoreConnectInfo, error) {
	return ro.restClient.GetObjectStoreConnectionInfo()

}

//Function to create user on rook object store
//Input paramatres - userId and display Name
//Output - output returned by rook Rest API client
func (ro *ObjectOperation) ObjectCreateUser(userid string, displayname string) (*model.ObjectUser, error) {
	objectUser := model.ObjectUser{UserID: userid, DisplayName: &displayname}
	return ro.restClient.CreateObjectUser(objectUser)

}

//Function to update a user on rook object store
//Input paramatres - userId,display Name and email address
//Output - output returned by rook Rest API client
func (ro *ObjectOperation) ObjectUpdateUser(userid string, displayname string, emailid string) (*model.ObjectUser, error) {
	objectUser := model.ObjectUser{UserID: userid, DisplayName: &displayname, Email: &emailid}
	return ro.restClient.UpdateObjectUser(objectUser)
}

//Function to delete user on rook object store
//Input paramatres - userId
//Output - output returned by rook Rest API client
func (ro *ObjectOperation) ObjectDeleteUser(userid string) error {
	return ro.restClient.DeleteObjectUser(userid)
}

//Function to get a user on rook object store
//Input paramatres - userId
//Output - output returned by rook Rest API client
func (ro *ObjectOperation) ObjectGetUser(userid string) (*model.ObjectUser, error) {
	return ro.restClient.GetObjectUser(userid)
}

//Function to get all users on rook object store
//Input paramatres - none
//Output - output returned by rook Rest API client
func (ro *ObjectOperation) ObjectListUser() ([]model.ObjectUser, error) {
	return ro.restClient.ListObjectUsers()
}
