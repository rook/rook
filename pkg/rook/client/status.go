package client

import (
	"encoding/json"

	"github.com/rook/rook/pkg/model"
)

const (
	statusQueryName = "status"
)

func (a *RookNetworkRestClient) GetStatusDetails() (model.StatusDetails, error) {
	body, err := a.DoGet(statusQueryName)
	if err != nil {
		return model.StatusDetails{}, err
	}

	var statusDetails model.StatusDetails
	err = json.Unmarshal(body, &statusDetails)
	if err != nil {
		return model.StatusDetails{}, err
	}

	return statusDetails, nil
}
