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

type RestAPIClient struct {
	rrc *rclient.RookNetworkRestClient
}

//Create Rook REST API client
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
		apiIp, err := rkh.GetPodHostId("rook-api", "rook")
		if err != nil {
			panic(fmt.Errorf("Host Ip for Rook-api service not found. %+v", err))
		}
		endpoint = "http://" + apiIp + ":30002"
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

func (a *RestAPIClient) URL() string {
	return a.rrc.RestURL
}

func (a *RestAPIClient) GetNodes() ([]model.Node, error) {
	return a.rrc.GetNodes()
}

func (a *RestAPIClient) GetPools() ([]model.Pool, error) {
	return a.rrc.GetPools()
}

func (a *RestAPIClient) CreatePool(pool model.Pool) (string, error) {
	return a.rrc.CreatePool(pool)
}

func (a *RestAPIClient) GetBlockImages() ([]model.BlockImage, error) {
	return a.rrc.GetBlockImages()
}
func (a *RestAPIClient) CreateBlockImage(image model.BlockImage) (string, error) {
	return a.rrc.CreateBlockImage(image)

}
func (a *RestAPIClient) DeleteBlockImage(image model.BlockImage) (string, error) {
	return a.rrc.DeleteBlockImage(image)
}
func (a *RestAPIClient) GetClientAccessInfo() (model.ClientAccessInfo, error) {
	return a.rrc.GetClientAccessInfo()
}
func (a *RestAPIClient) GetFilesystems() ([]model.Filesystem, error) {
	return a.rrc.GetFilesystems()
}
func (a *RestAPIClient) CreateFilesystem(fsmodel model.FilesystemRequest) (string, error) {
	return a.rrc.CreateFilesystem(fsmodel)
}
func (a *RestAPIClient) DeleteFilesystem(fsmodel model.FilesystemRequest) (string, error) {
	return a.rrc.DeleteFilesystem(fsmodel)
}
func (a *RestAPIClient) GetStatusDetails() (model.StatusDetails, error) {
	return a.rrc.GetStatusDetails()
}
func (a *RestAPIClient) CreateObjectStore() (string, error) {
	return a.rrc.CreateObjectStore()
}
func (a *RestAPIClient) GetObjectStoreConnectionInfo() (*model.ObjectStoreConnectInfo, error) {
	return a.rrc.GetObjectStoreConnectionInfo()
}
func (a *RestAPIClient) ListBuckets() ([]model.ObjectBucket, error) {
	return a.rrc.ListBuckets()
}
func (a *RestAPIClient) ListObjectUsers() ([]model.ObjectUser, error) {
	return a.rrc.ListObjectUsers()
}
func (a *RestAPIClient) GetObjectUser(id string) (*model.ObjectUser, error) {
	return a.rrc.GetObjectUser(id)
}
func (a *RestAPIClient) CreateObjectUser(user model.ObjectUser) (*model.ObjectUser, error) {
	return a.rrc.CreateObjectUser(user)
}
func (a *RestAPIClient) UpdateObjectUser(user model.ObjectUser) (*model.ObjectUser, error) {
	return a.rrc.UpdateObjectUser(user)

}
func (a *RestAPIClient) DeleteObjectUser(id string) error {
	return a.rrc.DeleteObjectUser(id)

}
