package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ListBucketsWithStat will return the list of all buckets with stat (system admin API only)
func (api *API) ListBucketsWithStat(ctx context.Context) ([]Bucket, error) {
	generateStat := true
	listingSpec := BucketListingSpec{
		GenerateStat: &generateStat,
	}

	body, err := api.call(ctx, http.MethodGet, "/bucket", valueToURLParams(listingSpec, []string{"stats"}))
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
