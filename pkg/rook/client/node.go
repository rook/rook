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
