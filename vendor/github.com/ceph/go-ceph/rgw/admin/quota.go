package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// QuotaSpec describes an object store quota for a user or a bucket
// Only user's quota are supported
type QuotaSpec struct {
	UID        string `json:"user_id" url:"uid"`
	Bucket     string `json:"bucket" url:"bucket"`
	QuotaType  string `url:"quota-type"`
	Enabled    *bool  `json:"enabled" url:"enabled"`
	CheckOnRaw bool   `json:"check_on_raw"`
	MaxSize    *int64 `json:"max_size" url:"max-size"`
	MaxSizeKb  *int   `json:"max_size_kb" url:"max-size-kb"`
	MaxObjects *int64 `json:"max_objects" url:"max-objects"`
}

// GetUserQuota will return the quota for a user
func (api *API) GetUserQuota(ctx context.Context, quota QuotaSpec) (QuotaSpec, error) {
	// Always for quota type to user
	quota.QuotaType = "user"

	if quota.UID == "" {
		return QuotaSpec{}, errMissingUserID
	}

	body, err := api.call(ctx, http.MethodGet, "/user?quota", valueToURLParams(quota, []string{"uid", "quota-type"}))
	if err != nil {
		return QuotaSpec{}, err
	}

	ref := QuotaSpec{}
	err = json.Unmarshal(body, &ref)
	if err != nil {
		return QuotaSpec{}, fmt.Errorf("%s. %s. %w", unmarshalError, string(body), err)
	}

	return ref, nil
}

// SetUserQuota sets quota to a user
// Global quotas (https://docs.ceph.com/en/latest/radosgw/admin/#reading-writing-global-quotas) are not surfaced in the Admin Ops API
// So this library cannot expose it yet
func (api *API) SetUserQuota(ctx context.Context, quota QuotaSpec) error {
	// Always for quota type to user
	quota.QuotaType = "user"

	if quota.UID == "" {
		return errMissingUserID
	}

	_, err := api.call(ctx, http.MethodPut, "/user?quota", valueToURLParams(quota, []string{"uid", "quota-type", "enabled", "max-size", "max-size-kb", "max-objects"}))
	if err != nil {
		return err
	}

	return nil
}
