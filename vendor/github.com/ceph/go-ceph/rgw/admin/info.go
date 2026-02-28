//go:build !(nautilus || octopus || pacific)
// +build !nautilus,!octopus,!pacific

package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// StorageBackend struct
type StorageBackend struct {
	Name      string `json:"name"`
	ClusterID string `json:"cluster_id"`
}

// Info struct
type Info struct {
	InfoSpec struct {
		StorageBackends []StorageBackend `json:"storage_backends"`
	} `json:"info"`
}

// GetInfo - https://docs.ceph.com/en/latest/radosgw/adminops/#info
func (api *API) GetInfo(ctx context.Context) (Info, error) {
	body, err := api.call(ctx, http.MethodGet, "/info", nil)
	if err != nil {
		return Info{}, err
	}
	i := Info{}
	err = json.Unmarshal(body, &i)
	if err != nil {
		return Info{}, fmt.Errorf("%s. %s. %w", unmarshalError, string(body), err)
	}

	return i, nil
}
