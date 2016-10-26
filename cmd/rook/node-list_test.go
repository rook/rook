package rook

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
	assert.Equal(t, "PUBLIC      PRIVATE      STATE     CLUSTER    SIZE      LOCATION                      UPDATED   \n187.1.2.3   10.0.0.100   OK        cluster1   100 B     root=default,dc=datacenter5   1s ago    \n", out)
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
