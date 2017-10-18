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
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/utils"
)

var (
	versionCmd = []string{"rook", "version"}
)

//TestClient is a wrapper for test client, containing interfaces for all rook operations
type TestClient struct {
	blockClient  contracts.BlockOperator
	fsClient     contracts.FileSystemOperator
	objectClient contracts.ObjectOperator
	poolClient   contracts.PoolOperator
	restClient   contracts.RestAPIOperator
}

const (
	unableToCheckRookStatusMsg = "Unable to check rook status - please check of rook is up and running"
)

//CreateTestClient creates new instance of test client for a platform
func CreateTestClient(k8sHelper *utils.K8sHelper, namespace string) (*TestClient, error) {
	var blockClient contracts.BlockOperator
	var fsClient contracts.FileSystemOperator
	var objectClient contracts.ObjectOperator
	var poolClient contracts.PoolOperator
	rookRestClient := CreateRestAPIClient(k8sHelper, namespace)
	blockClient = CreateK8BlockOperation(k8sHelper, rookRestClient)
	fsClient = CreateK8sFilesystemOperation(k8sHelper, rookRestClient)
	objectClient = CreateObjectOperation(rookRestClient)
	poolClient = CreatePoolClient(rookRestClient)

	return &TestClient{
		blockClient,
		fsClient,
		objectClient,
		poolClient,
		rookRestClient,
	}, nil

}

//Status returns rook status details
func (c TestClient) Status() (model.StatusDetails, error) {
	return c.restClient.GetStatusDetails()
}

//Node returns list of rook nodes
func (c TestClient) Node() ([]model.Node, error) {
	return c.restClient.GetNodes()
}

//GetBlockClient returns Block client for platform in context
func (c TestClient) GetBlockClient() contracts.BlockOperator {
	return c.blockClient
}

//GetFileSystemClient returns fileSystem client for platform in context
func (c TestClient) GetFileSystemClient() contracts.FileSystemOperator {
	return c.fsClient
}

//GetObjectClient returns Object client for platform in context
func (c TestClient) GetObjectClient() contracts.ObjectOperator {
	return c.objectClient
}

//GetPoolClient returns pool client for platform in context
func (c TestClient) GetPoolClient() contracts.PoolOperator {
	return c.poolClient
}

//GetRestAPIClient returns RestAPI client for platform in context
func (c TestClient) GetRestAPIClient() contracts.RestAPIOperator {
	return c.restClient
}
