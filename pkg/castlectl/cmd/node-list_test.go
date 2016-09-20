package cmd

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/quantum/castle/pkg/castlectl/test"
	"github.com/quantum/castle/pkg/model"
)

func TestListNodes(t *testing.T) {
	c := &test.MockCastleRestClient{
		MockGetNodes: func() ([]model.Node, error) {
			nodes := []model.Node{
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
		MockGetNodes: func() ([]model.Node, error) {
			return nil, fmt.Errorf("mock get nodes failed")
		},
	}

	out, err := listNodes(c)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}
