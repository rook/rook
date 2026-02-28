package admin

import (
	"context"
	"net/http"
)

// SetIndividualBucketQuota sets quota to a specific bucket
// https://docs.ceph.com/en/latest/radosgw/adminops/#set-quota-for-an-individual-bucket
func (api *API) SetIndividualBucketQuota(ctx context.Context, quota QuotaSpec) error {
	if quota.UID == "" {
		return errMissingUserID
	}

	if quota.Bucket == "" {
		return errMissingUserBucket
	}

	_, err := api.call(ctx, http.MethodPut, "/bucket?quota", valueToURLParams(quota, []string{"bucket", "uid", "enabled", "max-size", "max-size-kb", "max-objects"}))
	if err != nil {
		return err
	}

	return nil
}
