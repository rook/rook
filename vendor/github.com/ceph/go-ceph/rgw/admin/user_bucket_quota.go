package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// GetBucketQuota - https://docs.ceph.com/en/latest/radosgw/adminops/#get-bucket-quota
func (api *API) GetBucketQuota(ctx context.Context, quota QuotaSpec) (QuotaSpec, error) {
	// Always for bucket quota type
	quota.QuotaType = "bucket"

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

// SetBucketQuota - https://docs.ceph.com/en/latest/radosgw/adminops/#set-bucket-quota
func (api *API) SetBucketQuota(ctx context.Context, quota QuotaSpec) error {
	// Always for bucket quota type
	quota.QuotaType = "bucket"

	if quota.UID == "" {
		return errMissingUserID
	}

	_, err := api.call(ctx, http.MethodPut, "/user?quota", valueToURLParams(quota, []string{"uid", "quota-type", "enabled", "max-size", "max-size-kb", "max-objects"}))
	if err != nil {
		return err
	}

	return nil
}
