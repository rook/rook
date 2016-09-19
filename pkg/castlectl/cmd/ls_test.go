package cmd

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/quantum/castle/pkg/castlectl/client"
	"github.com/quantum/castle/pkg/castlectl/test"
)

func TestListNodes(t *testing.T) {
	c := &test.MockCastleRestClient{
		MockGetNodes: func() ([]client.Node, error) {
			nodes := []client.Node{
				{NodeID: "node1", IPAddress: "10.0.0.100", Storage: 100},
			}
			return nodes, nil
		},
	}

	out, err := listNodes(c)
	assert.Nil(t, err)
	assert.Equal(t, "{NodeID:node1 IPAddress:10.0.0.100 Storage:100}", out)
}

func TestListNodesError(t *testing.T) {
	c := &test.MockCastleRestClient{
		MockGetNodes: func() ([]client.Node, error) {
			return nil, fmt.Errorf("mock get nodes failed")
		},
	}

	out, err := listNodes(c)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}
