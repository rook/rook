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
package client

import (
	"encoding/json"

	"github.com/rook/rook/pkg/model"
)

const (
	nodeQueryName = "node"
)

func (a *RookNetworkRestClient) GetNodes() ([]model.Node, error) {
	body, err := a.DoGet(nodeQueryName)
	if err != nil {
		return nil, err
	}

	var nodes []model.Node
	err = json.Unmarshal(body, &nodes)
	if err != nil {
		return nil, err
	}

	return nodes, nil
}
