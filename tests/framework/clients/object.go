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

package clients

import (
	"fmt"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/utils"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook/tests", "clients")

//ObjectOperation is wrapper for k8s rook object operations
type ObjectOperation struct {
	restClient contracts.RestAPIOperator
}

//CreateObjectOperation creates new rook object client
func CreateObjectOperation(rookRestClient contracts.RestAPIOperator) *ObjectOperation {
	return &ObjectOperation{restClient: rookRestClient}
}

//ObjectCreate Function to create a object store in rook
//Input paramatres -None
//Output - output returned by rook Rest API client
func (ro *ObjectOperation) ObjectCreate(namespace, storeName string, replicaCount int32, callAPI bool, k8sh *utils.K8sHelper) error {
	store := model.ObjectStore{Name: storeName, Gateway: model.Gateway{Instances: replicaCount, Port: model.RGWPort}}
	store.DataConfig.ReplicatedConfig.Size = 1
	store.MetadataConfig.ReplicatedConfig.Size = 1
	if callAPI {
		logger.Infof("creating the object store via the rest API")
		_, err := ro.restClient.CreateObjectStore(store)
		return err
	}

	logger.Infof("creating the object store via CRD")
	storeSpec := fmt.Sprintf(`apiVersion: rook.io/v1alpha1
kind: Objectstore
metadata:
  name: %s
  namespace: %s
spec:
  metadataPool:
    replicated:
      size: 1
  dataPool:
    replicated:
      size: 1
  gateway:
    type: s3
    sslCertificateRef: 
    port: %d
    securePort:
    instances: %d
    allNodes: false
`, store.Name, namespace, store.Gateway.Port, store.Gateway.Instances)

	if _, err := k8sh.ResourceOperation("create", storeSpec); err != nil {
		return err
	}

	if !k8sh.IsPodWithLabelRunning(fmt.Sprintf("rook_object_store=%s", storeName), namespace) {
		return fmt.Errorf("rgw did not start via crd")
	}
	return nil
}

//ObjectBucketList Function to get Buckets present in rook object store
//Input paramatres - None
//Output - output returned by rook Rest API client
func (ro *ObjectOperation) ObjectBucketList(storeName string) ([]model.ObjectBucket, error) {
	return ro.restClient.ListBuckets(storeName)

}

//ObjectConnection Function to get connection information for a user
//Input paramatres - None
//Output - output returned by rook Rest API client
func (ro *ObjectOperation) ObjectConnection(storeName string) (*model.ObjectStoreConnectInfo, error) {
	return ro.restClient.GetObjectStoreConnectionInfo(storeName)

}

//ObjectCreateUser Function to create user on rook object store
//Input paramatres - userId and display Name
//Output - output returned by rook Rest API client
func (ro *ObjectOperation) ObjectCreateUser(storeName, userid string, displayname string) (*model.ObjectUser, error) {
	objectUser := model.ObjectUser{UserID: userid, DisplayName: &displayname}
	return ro.restClient.CreateObjectUser(storeName, objectUser)

}

//ObjectUpdateUser Function to update a user on rook object store
//Input paramatres - userId,display Name and email address
//Output - output returned by rook Rest API client
func (ro *ObjectOperation) ObjectUpdateUser(storeName, userid string, displayname string, emailid string) (*model.ObjectUser, error) {
	objectUser := model.ObjectUser{UserID: userid, DisplayName: &displayname, Email: &emailid}
	return ro.restClient.UpdateObjectUser(storeName, objectUser)
}

//ObjectDeleteUser Function to delete user on rook object store
//Input paramatres - userId
//Output - output returned by rook Rest API client
func (ro *ObjectOperation) ObjectDeleteUser(storeName, userid string) error {
	return ro.restClient.DeleteObjectUser(storeName, userid)
}

//ObjectGetUser Function to get a user on rook object store
//Input paramatres - userId
//Output - output returned by rook Rest API client
func (ro *ObjectOperation) ObjectGetUser(storeName, userid string) (*model.ObjectUser, error) {
	return ro.restClient.GetObjectUser(storeName, userid)
}

//ObjectListUser Function to get all users on rook object store
//Input paramatres - none
//Output - output returned by rook Rest API client
func (ro *ObjectOperation) ObjectListUser(storeName string) ([]model.ObjectUser, error) {
	return ro.restClient.ListObjectUsers(storeName)
}
