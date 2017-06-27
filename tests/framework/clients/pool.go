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
)

type PoolClient struct {
	transportClient contracts.ITransportClient
	restClient      contracts.RestAPIOperator
}

func CreatePoolClient(rookRestClient contracts.RestAPIOperator) *PoolClient {
	return &PoolClient{restClient: rookRestClient}
}

func (rp *PoolClient) PoolList() ([]model.Pool, error) {
	return rp.restClient.GetPools()
}

func (rp *PoolClient) PoolCreate(pool model.Pool) (string, error) {
	return rp.restClient.CreatePool(pool)
}
