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
	"net"
	"net/http"
	"time"

	"github.com/rook/rook/pkg/model"
	rclient "github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/tests/framework/utils"
)

//RestAPIClient is wrapper for rook rest api client
type RestAPIClient struct {
	rrc *rclient.RookNetworkRestClient
}

//CreateRestAPIClient Create Rook REST API client
func CreateRestAPIClient(k8sHelper *utils.K8sHelper, namespace string) *RestAPIClient {
	var endpoint string

	//Start rook_api_external server via nodePort if not it not already running.
	externalAPIservice := "rook-api-external-" + namespace
	_, err := k8sHelper.GetService(externalAPIservice, namespace)
	if err != nil {
		_, err = k8sHelper.ResourceOperation("create", getExternalAPIServiceDefinition(namespace))
		if err != nil {
			logger.Errorf("failed to kubectl create -f -  %v: %+v", getExternalAPIServiceDefinition(namespace), err)
			return nil
		}
	}
	apiIP, err := k8sHelper.GetPodHostID("rook-api", namespace)
	if err != nil {
		logger.Errorf("Host Ip for Rook-api service not found. %+v", err)
		return nil
	}
	nodePort, err := k8sHelper.GetServiceNodePort(externalAPIservice, namespace)
	if err != nil {
		logger.Errorf("port for external Rook-api service not found. %+v", err)
		return nil
	}
	endpoint = fmt.Sprintf("http://%s:%s", apiIP, nodePort)

	httpclient := &http.Client{
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   90 * time.Second,
				KeepAlive: 0,
			}).Dial,
			DisableKeepAlives:     true,
			DisableCompression:    true,
			MaxIdleConnsPerHost:   1,
			ResponseHeaderTimeout: 90 * time.Second,
		},
	}
	client := rclient.NewRookNetworkRestClient(endpoint, httpclient)

	//make sure rest client is up and available
	logger.Infof("ensuring API service is accessible by client on external endpoint %s", endpoint)
	inc := 0
	for inc < utils.RetryLoop {
		_, err := client.GetStatusDetails()
		if err == nil {
			//hack first get block image calls is intermittently failing in test env
			client.GetBlockImages()
			return &RestAPIClient{client}

		}
		inc++
		logger.Infof("failed to access API service, will retry: %+v", err)
		time.Sleep(time.Second * utils.RetryInterval)
	}

	logger.Info("Cannot access rook API - Abandoning run")
	return nil
}

//URL returns URL for rookAPI
func (a *RestAPIClient) URL() string {
	return a.rrc.RestURL
}

//GetNodes returns all rook nodes
func (a *RestAPIClient) GetNodes() ([]model.Node, error) {
	return a.rrc.GetNodes()
}

//GetPools returns all pools in rook
func (a *RestAPIClient) GetPools() ([]model.Pool, error) {
	return a.rrc.GetPools()
}

//CreatePool creates a new pool
func (a *RestAPIClient) CreatePool(pool model.Pool) (string, error) {
	return a.rrc.CreatePool(pool)
}

//GetBlockImages returns list of a block images
func (a *RestAPIClient) GetBlockImages() ([]model.BlockImage, error) {
	return a.rrc.GetBlockImages()
}

//CreateBlockImage creates a new block image in rook
func (a *RestAPIClient) CreateBlockImage(image model.BlockImage) (string, error) {
	return a.rrc.CreateBlockImage(image)
}

//DeleteBlockImage deletes a block image from rook
func (a *RestAPIClient) DeleteBlockImage(image model.BlockImage) (string, error) {
	return a.rrc.DeleteBlockImage(image)
}

//GetClientAccessInfo returns rook REST API client info
func (a *RestAPIClient) GetClientAccessInfo() (model.ClientAccessInfo, error) {
	return a.rrc.GetClientAccessInfo()
}

//GetFilesystems returns rook filesystem
func (a *RestAPIClient) GetFilesystems() ([]model.Filesystem, error) {
	return a.rrc.GetFilesystems()
}

//CreateFilesystem creates file system on rook
func (a *RestAPIClient) CreateFilesystem(fsmodel model.FilesystemRequest) (string, error) {
	return a.rrc.CreateFilesystem(fsmodel)
}

//DeleteFilesystem deletes file system from rook
func (a *RestAPIClient) DeleteFilesystem(fsmodel model.FilesystemRequest) (string, error) {
	return a.rrc.DeleteFilesystem(fsmodel)
}

//GetStatusDetails reruns rook status details
func (a *RestAPIClient) GetStatusDetails() (model.StatusDetails, error) {
	return a.rrc.GetStatusDetails()
}

//CreateObjectStore creates object store
func (a *RestAPIClient) CreateObjectStore(store model.ObjectStore) (string, error) {
	return a.rrc.CreateObjectStore(store)
}

//DeleteObjectStore creates object store
func (a *RestAPIClient) DeleteObjectStore(storeName string) error {
	return a.rrc.DeleteObjectStore(storeName)
}

//GetObjectStoreConnectionInfo returns object store connection info
func (a *RestAPIClient) GetObjectStoreConnectionInfo(storeName string) (*model.ObjectStoreConnectInfo, error) {
	return a.rrc.GetObjectStoreConnectionInfo(storeName)
}

//ListBuckets lists all buckets in object store
func (a *RestAPIClient) ListBuckets(storeName string) ([]model.ObjectBucket, error) {
	return a.rrc.ListBuckets(storeName)
}

//ListObjectUsers returns all object store users
func (a *RestAPIClient) ListObjectUsers(storeName string) ([]model.ObjectUser, error) {
	return a.rrc.ListObjectUsers(storeName)
}

//GetObjectUser returns a object user from object store
func (a *RestAPIClient) GetObjectUser(storeName, id string) (*model.ObjectUser, error) {
	return a.rrc.GetObjectUser(storeName, id)
}

//CreateObjectUser creates new  user in object store
func (a *RestAPIClient) CreateObjectUser(storeName string, user model.ObjectUser) (*model.ObjectUser, error) {
	return a.rrc.CreateObjectUser(storeName, user)
}

//UpdateObjectUser updates user in object store
func (a *RestAPIClient) UpdateObjectUser(storeName string, user model.ObjectUser) (*model.ObjectUser, error) {
	return a.rrc.UpdateObjectUser(storeName, user)

}

//DeleteObjectUser deletes user from object store
func (a *RestAPIClient) DeleteObjectUser(storeName, id string) error {
	return a.rrc.DeleteObjectUser(storeName, id)

}

func getExternalAPIServiceDefinition(namespace string) string {
	return `apiVersion: v1
kind: Service
metadata:
  name: rook-api-external-` + namespace + `
  namespace: ` + namespace + `
  labels:
    app: rook-api
    rook_cluster: ` + namespace + `
spec:
  ports:
  - name: rook-api
    port: 8124
    protocol: TCP
  selector:
    app: rook-api
    rook_cluster: ` + namespace + `
  sessionAffinity: None
  type: NodePort
`
}
