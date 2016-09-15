package client

import "encoding/json"

type Node struct {
	NodeID    string `json:"nodeID"`
	IPAddress string `json:"ipAddr"`
	Storage   uint64 `json:"storage"`
}

func (a *CastleNetworkRestClient) GetNodes() ([]Node, error) {
	body, err := a.DoGet("node")
	if err != nil {
		return nil, err
	}

	var nodes []Node
	err = json.Unmarshal(body, &nodes)
	if err != nil {
		return nil, err
	}

	return nodes, nil
}
