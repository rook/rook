package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// BucketListingSpec describes a request
type BucketListingSpec struct {
	UID          string `url:"uid"`
	GenerateStat *bool  `url:"stats"`
}

// ListUsersBuckets will return the list of all users buckets without stat
func (api *API) ListUsersBuckets(ctx context.Context, uid string) ([]string, error) {
	if uid == "" {
		return nil, errMissingUserID
	}

	generateStat := false
	listingSpec := BucketListingSpec{
		UID:          uid,
		GenerateStat: &generateStat,
	}

	body, err := api.call(ctx, http.MethodGet, "/bucket", valueToURLParams(listingSpec, []string{"uid", "stats"}))
	if err != nil {
		return nil, err
	}

	var s []string
	err = json.Unmarshal(body, &s)
	if err != nil {
		return nil, fmt.Errorf("%s. %s. %w", unmarshalError, string(body), err)
	}

	return s, nil
}

// ListUsersBucketsWithStat will return the list of all users buckets with stat
func (api *API) ListUsersBucketsWithStat(ctx context.Context, uid string) ([]Bucket, error) {
	if uid == "" {
		return nil, errMissingUserID
	}

	generateStat := true
	listingSpec := BucketListingSpec{
		UID:          uid,
		GenerateStat: &generateStat,
	}

	body, err := api.call(ctx, http.MethodGet, "/bucket", valueToURLParams(listingSpec, []string{"uid", "stats"}))
	if err != nil {
		return nil, err
	}

	ref := []Bucket{}
	err = json.Unmarshal(body, &ref)
	if err != nil {
		return nil, fmt.Errorf("%s. %s. %w", unmarshalError, string(body), err)
	}

	return ref, nil
}
