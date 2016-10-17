package client

import (
	"encoding/json"

	"github.com/quantum/castle/pkg/model"
)

const (
	NodeQueryName = "node"
)

func (a *CastleNetworkRestClient) GetNodes() ([]model.Node, error) {
	body, err := a.DoGet(NodeQueryName)
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
