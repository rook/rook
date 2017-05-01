package clients

import (
	"errors"
	"github.com/dangula/rook/e2e/rook-test-framework/contracts"
)

type k8sRookObject struct {
	transportClient contracts.ITransportClient
}

var (
	objectStoreCrate         = []string{"rook", "object", "create"}
	objectStoreGetBuckets    = []string{"rook", "object", "bucket", "list"}
	objectStoreGetConnection = []string{"rook", "object", "connection", "REPLACE"}
	objectStoreCreateUser    = []string{"rook", "object", "user", "create", "USERID", "DISPALYNAME"}
	objectStoreDeleteUser    = []string{"rook", "object", "user", "delete", "USERID"}
	objectStoreGetUser       = []string{"rook", "object", "user", "get", "USERID"}
	objectStoreGetUsers      = []string{"rook", "object", "user", "list"}
	objectStoreUpdateUser    = []string{"rook", "object", "user", "update", "USERID", "--display-name", "DISPLAYNAME",
		"--email", "EMAILID"}
)

func CreateK8sRookObject(client contracts.ITransportClient) *k8sRookObject {
	return &k8sRookObject{transportClient: client}
}

//Function to create a object store in rook
//Input paramatres -None
//Output - output returned by rook cli and/or error
func (ro *k8sRookObject) Object_Create() (string, error) {
	out, err, status := ro.transportClient.Execute(objectStoreCrate, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unabale to create Object store")
	}
}

//Function to get Buckets present in rook object store
//Input paramatres - None
//Output - output returned by rook cli and/or error
func (ro *k8sRookObject) Object_Bucket_list() (string, error) {
	out, err, status := ro.transportClient.Execute(objectStoreGetBuckets, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unabale to get buckets")
	}
}

//Function to get connection information for a user
//Input paramatres - userId
//Output - output returned by rook cli and/or error
func (ro *k8sRookObject) Object_Connection(userid string) (string, error) {
	objectStoreGetConnection[3] = userid
	out, err, status := ro.transportClient.Execute(objectStoreGetConnection, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unabale to get buckets")
	}
}

//Function to create user on rook object store
//Input paramatres - userId and display Name
//Output - output returned by rook cli and/or error
func (ro *k8sRookObject) Object_Create_user(userid string, displayname string) (string, error) {
	objectStoreCreateUser[4] = userid
	objectStoreCreateUser[5] = displayname

	out, err, status := ro.transportClient.Execute(objectStoreCreateUser, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unabale to get buckets")
	}
}

//Function to delete user on rook object store
//Input paramatres - userId
//Output - output returned by rook cli and/or error
func (ro *k8sRookObject) Object_Delete_user(userid string) (string, error) {
	objectStoreDeleteUser[4] = userid

	out, err, status := ro.transportClient.Execute(objectStoreDeleteUser, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unabale to get buckets")
	}
}

//Function to get a user on rook object store
//Input paramatres - userId
//Output - output returned by rook cli and/or error
func (ro *k8sRookObject) Object_Get_user(userid string) (string, error) {

	objectStoreGetUser[4] = userid

	out, err, status := ro.transportClient.Execute(objectStoreGetUser, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unabale to get buckets")
	}
}

//Function to get all users on rook object store
//Input paramatres - none
//Output - output returned by rook cli and/or error
func (ro *k8sRookObject) Object_List_user() (string, error) {

	out, err, status := ro.transportClient.Execute(objectStoreGetUsers, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unabale to get buckets")
	}
}

//Function to update a user on rook object store
//Input paramatres - userId,display Name and email address
//Output - output returned by rook cli and/or error
func (ro *k8sRookObject) Object_Update_user(userid string, displayname string, emailid string) (string, error) {
	objectStoreUpdateUser[4] = userid
	objectStoreUpdateUser[6] = displayname
	objectStoreUpdateUser[8] = emailid

	out, err, status := ro.transportClient.Execute(objectStoreUpdateUser, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unabale to get buckets")
	}
}
