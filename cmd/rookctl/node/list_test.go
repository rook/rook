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
package node

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/test"
)

func TestListNodes(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetNodes: func() ([]model.Node, error) {
			nodes := []model.Node{
				{
					NodeID:      "node1",
					ClusterName: "cluster1",
					PublicIP:    "187.1.2.3",
					PrivateIP:   "10.0.0.100",
					Storage:     100,
					LastUpdated: time.Duration(1) * time.Second,
					State:       model.Healthy,
					Location:    "root=default,dc=datacenter5",
				},
			}
			return nodes, nil
		},
	}

	out, err := listNodes(c)
	assert.Nil(t, err)

	expectedOut := "PUBLIC      PRIVATE      STATE     CLUSTER    SIZE      LOCATION                      UPDATED\n" +
		"187.1.2.3   10.0.0.100   OK        cluster1   100 B     root=default,dc=datacenter5   1s ago    \n"
	assert.Equal(t, expectedOut, out)
}

func TestListNodesError(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetNodes: func() ([]model.Node, error) {
			return nil, fmt.Errorf("mock get nodes failed")
		},
	}

	out, err := listNodes(c)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}
