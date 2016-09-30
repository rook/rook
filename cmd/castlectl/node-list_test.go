package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/quantum/castle/pkg/castlectl/test"
	"github.com/quantum/castle/pkg/model"
)

func TestListNodes(t *testing.T) {
	c := &test.MockCastleRestClient{
		MockGetNodes: func() ([]model.Node, error) {
			nodes := []model.Node{
				{
					NodeID:      "node1",
					ClusterName: "cluster1",
					IPAddress:   "10.0.0.100",
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
	assert.Equal(t, "ADDRESS      STATE     CLUSTER    SIZE      LOCATION                      UPDATED   \n10.0.0.100   OK        cluster1   100 B     root=default,dc=datacenter5   1s ago    \n", out)
}

func TestListNodesError(t *testing.T) {
	c := &test.MockCastleRestClient{
		MockGetNodes: func() ([]model.Node, error) {
			return nil, fmt.Errorf("mock get nodes failed")
		},
	}

	out, err := listNodes(c)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}
