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

	"os"
	"path/filepath"

	"github.com/rook/rook/pkg/model"
	rclient "github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/tests/framework/enums"
	"github.com/rook/rook/tests/framework/utils"
)

//RestAPIClient is wrapper for rook rest api client
type RestAPIClient struct {
	rrc *rclient.RookNetworkRestClient
}

//CreateRestAPIClient Create Rook REST API client
func CreateRestAPIClient(platform enums.RookPlatformType) *RestAPIClient {
	var endpoint string
	switch {
	case platform == enums.Kubernetes:
		rkh := utils.CreatK8sHelper()
		//Start rook_api_external server via nodePort if not it not already running.
		_, err := rkh.GetService("rook-api-external")
		if err != nil {
			path := filepath.Join(os.Getenv("GOPATH"), "src/github.com/rook/rook/tests/data/smoke/rook_api_external.yaml")
			_, err = rkh.ResourceOperation("create", path)
			if err != nil {
				panic(fmt.Errorf("failed to kubectl create %v: %+v", path, err))
			}
		}
		apiIP, err := rkh.GetPodHostID("rook-api", "rook")
		if err != nil {
			panic(fmt.Errorf("Host Ip for Rook-api service not found. %+v", err))
		}
		endpoint = "http://" + apiIP + ":30002"
	case platform == enums.StandAlone:
		endpoint = "http://localhost:8124"
	default:
		panic(fmt.Errorf("platfrom type %s not yet supported", platform))
	}

	httpclient := &http.Client{
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 0,
			}).Dial,
			DisableKeepAlives:     true,
			DisableCompression:    true,
			MaxIdleConnsPerHost:   1,
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}
	client := rclient.NewRookNetworkRestClient(endpoint, httpclient)

	return &RestAPIClient{client}
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

//GetStatusDetails retuns rook status details
func (a *RestAPIClient) GetStatusDetails() (model.StatusDetails, error) {
	return a.rrc.GetStatusDetails()
}

//CreateObjectStore creates object store
func (a *RestAPIClient) CreateObjectStore() (string, error) {
	return a.rrc.CreateObjectStore()
}

//GetObjectStoreConnectionInfo returns object store connection info
func (a *RestAPIClient) GetObjectStoreConnectionInfo() (*model.ObjectStoreConnectInfo, error) {
	return a.rrc.GetObjectStoreConnectionInfo()
}

//ListBuckets lists all buckets in object store
func (a *RestAPIClient) ListBuckets() ([]model.ObjectBucket, error) {
	return a.rrc.ListBuckets()
}

//ListObjectUsers returns all object store users
func (a *RestAPIClient) ListObjectUsers() ([]model.ObjectUser, error) {
	return a.rrc.ListObjectUsers()
}

//GetObjectUser returns a object user from object store
func (a *RestAPIClient) GetObjectUser(id string) (*model.ObjectUser, error) {
	return a.rrc.GetObjectUser(id)
}

//CreateObjectUser creates new  user in object store
func (a *RestAPIClient) CreateObjectUser(user model.ObjectUser) (*model.ObjectUser, error) {
	return a.rrc.CreateObjectUser(user)
}

//UpdateObjectUser updates user in object store
func (a *RestAPIClient) UpdateObjectUser(user model.ObjectUser) (*model.ObjectUser, error) {
	return a.rrc.UpdateObjectUser(user)

}

//DeleteObjectUser deletes user from object store
func (a *RestAPIClient) DeleteObjectUser(id string) error {
	return a.rrc.DeleteObjectUser(id)

}
