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

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/enums"
	"github.com/rook/rook/tests/framework/transport"
)

var (
	VERSION_CMD = []string{"rook", "version"}
)

type TestClient struct {
	platform        enums.RookPlatformType
	transportClient contracts.ITransportClient
	blockClient     contracts.BlockOperator
	fsClient        contracts.FileSystemOperator
	objectClient    contracts.ObjectOperator
	poolClient      contracts.PoolOperator
	restClient      contracts.RestAPIOperator
}

const (
	unable_to_check_rook_status_msg = "Unable to check rook status - please check of rook is up and running"
)

func CreateTestClient(platform enums.RookPlatformType) (*TestClient, error) {
	var transportClient contracts.ITransportClient
	var block_client contracts.BlockOperator
	var fs_client contracts.FileSystemOperator
	var object_client contracts.ObjectOperator
	var pool_client contracts.PoolOperator
	rookRestClient := CreateRestAPIClient(platform)

	switch {
	case platform == enums.Kubernetes:
		transportClient = transport.CreateNewk8sTransportClient()
		block_client = CreateK8BlockOperation(transportClient, rookRestClient)
		fs_client = CreateK8sFileSystemOperation(transportClient, rookRestClient)
		object_client = CreateObjectOperation(rookRestClient)
		pool_client = CreatePoolClient(rookRestClient)
	case platform == enums.StandAlone:
		transportClient = nil //TODO- Not yet implemented
		block_client = nil    //TODO- Not yet implemented
		fs_client = nil       //TODO- Not yet implemented
		object_client = nil   //TODO- Not yet implemented
		pool_client = nil     //TODO- Not yet implemented
	default:
		return &TestClient{}, fmt.Errorf("Unsupported Rook Platform Type")
	}

	return &TestClient{
		platform,
		transportClient,
		block_client,
		fs_client,
		object_client,
		pool_client,
		rookRestClient,
	}, nil

}

func (c TestClient) Status() (model.StatusDetails, error) {
	return c.restClient.GetStatusDetails()
}

func (c TestClient) Version() (string, error) {
	out, err, status := c.transportClient.Execute(VERSION_CMD, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, fmt.Errorf(unable_to_check_rook_status_msg)
	}
}

func (c TestClient) Node() ([]model.Node, error) {
	return c.restClient.GetNodes()
}

func (c TestClient) GetTransportClient() contracts.ITransportClient {
	return c.transportClient
}

func (c TestClient) GetBlockClient() contracts.BlockOperator {
	return c.blockClient
}

func (c TestClient) GetFileSystemClient() contracts.FileSystemOperator {
	return c.fsClient
}

func (c TestClient) GetObjectClient() contracts.ObjectOperator {
	return c.objectClient
}

func (c TestClient) GetPoolClient() contracts.PoolOperator {
	return c.poolClient
}

func (c TestClient) GetRestAPIClient() contracts.RestAPIOperator {
	return c.restClient
}
