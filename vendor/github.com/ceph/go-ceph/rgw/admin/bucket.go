package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Bucket describes an object store bucket
type Bucket struct {
	Bucket            string  `json:"bucket" url:"bucket"`
	NumShards         *uint64 `json:"num_shards"`
	Tenant            string  `json:"tenant"`
	Zonegroup         string  `json:"zonegroup"`
	PlacementRule     string  `json:"placement_rule"`
	ExplicitPlacement struct {
		DataPool      string `json:"data_pool"`
		DataExtraPool string `json:"data_extra_pool"`
		IndexPool     string `json:"index_pool"`
	} `json:"explicit_placement"`
	ID                string     `json:"id"`
	Marker            string     `json:"marker"`
	IndexType         string     `json:"index_type"`
	Versioned         *bool      `json:"versioned"`           // reef
	VersioningEnabled *bool      `json:"versioning_enabled"`  // reef
	Versioning        *string    `json:"versioning"`          // quincy, squid+
	ObjectLockEnabled bool       `json:"object_lock_enabled"` // quincy+
	Owner             string     `json:"owner"`
	Ver               string     `json:"ver"`
	MasterVer         string     `json:"master_ver"`
	Mtime             string     `json:"mtime"`
	CreationTime      *time.Time `json:"creation_time"`
	MaxMarker         string     `json:"max_marker"`
	Usage             struct {
		RgwMain      RgwUsage `json:"rgw.main"`
		RgwMultimeta RgwUsage `json:"rgw.multimeta"`
	} `json:"usage"`
	BucketQuota QuotaSpec `json:"bucket_quota"`
	Policy      *bool     `url:"policy"`
	PurgeObject *bool     `url:"purge-objects"`
}

// RgwUsage contains usage statistics.
type RgwUsage struct {
	Size           *uint64 `json:"size"`
	SizeActual     *uint64 `json:"size_actual"`
	SizeUtilized   *uint64 `json:"size_utilized"`
	SizeKb         *uint64 `json:"size_kb"`
	SizeKbActual   *uint64 `json:"size_kb_actual"`
	SizeKbUtilized *uint64 `json:"size_kb_utilized"`
	NumObjects     *uint64 `json:"num_objects"`
}

// Policy describes a bucket policy
type Policy struct {
	ACL struct {
		ACLUserMap []struct {
			User string `json:"user"`
			ACL  *int   `json:"acl"`
		} `json:"acl_user_map"`
		ACLGroupMap []interface{} `json:"acl_group_map"`
		GrantMap    []struct {
			ID    string `json:"id"`
			Grant struct {
				Type struct {
					Type int `json:"type"`
				} `json:"type"`
				ID         string `json:"id"`
				Email      string `json:"email"`
				Permission struct {
					Flags int `json:"flags"`
				} `json:"permission"`
				Name    string `json:"name"`
				Group   *int   `json:"group"`
				URLSpec string `json:"url_spec"`
			} `json:"grant"`
		} `json:"grant_map"`
	} `json:"acl"`
	Owner struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"owner"`
}

// ListBuckets will return the list of all buckets present in the object store
func (api *API) ListBuckets(ctx context.Context) ([]string, error) {
	body, err := api.call(ctx, http.MethodGet, "/bucket", nil)
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

// GetBucketInfo will return various information about a specific token
func (api *API) GetBucketInfo(ctx context.Context, bucket Bucket) (Bucket, error) {
	body, err := api.call(ctx, http.MethodGet, "/bucket", valueToURLParams(bucket, []string{"bucket", "uid", "stats"}))
	if err != nil {
		return Bucket{}, err
	}

	ref := Bucket{}
	err = json.Unmarshal(body, &ref)
	if err != nil {
		return Bucket{}, fmt.Errorf("%s. %s. %w", unmarshalError, string(body), err)
	}

	return ref, nil
}

// GetBucketPolicy - https://docs.ceph.com/en/latest/radosgw/adminops/#get-bucket-or-object-policy
func (api *API) GetBucketPolicy(ctx context.Context, bucket Bucket) (Policy, error) {
	policy := true
	bucket.Policy = &policy

	// valid parameters not supported by go-ceph: object
	body, err := api.call(ctx, http.MethodGet, "/bucket?policy", valueToURLParams(bucket, []string{"bucket"}))
	if err != nil {
		return Policy{}, err
	}

	ref := Policy{}
	err = json.Unmarshal(body, &ref)
	if err != nil {
		return Policy{}, fmt.Errorf("%s. %s. %w", unmarshalError, string(body), err)
	}

	return ref, nil
}

// RemoveBucket will remove a given token from the object store
func (api *API) RemoveBucket(ctx context.Context, bucket Bucket) error {
	_, err := api.call(ctx, http.MethodDelete, "/bucket", valueToURLParams(bucket, []string{"bucket", "purge-objects"}))
	if err != nil {
		return err
	}

	return nil
}
