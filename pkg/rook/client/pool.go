package client

import (
	"bytes"
	"encoding/json"

	"github.com/rook/rook/pkg/model"
)

const (
	PoolQueryName = "pool"
)

func (c *RookNetworkRestClient) GetPools() ([]model.Pool, error) {
	body, err := c.DoGet(PoolQueryName)
	if err != nil {
		return nil, err
	}

	var pools []model.Pool
	err = json.Unmarshal(body, &pools)
	if err != nil {
		return nil, err
	}

	return pools, nil
}

func (c *RookNetworkRestClient) CreatePool(newPool model.Pool) (string, error) {
	body, err := json.Marshal(newPool)
	if err != nil {
		return "", err
	}

	resp, err := c.DoPost(PoolQueryName, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	return string(resp), nil
}
