/*
Copyright 2017 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package rgw

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
)

type rgwBucketStats struct {
	Bucket string `json:"bucket"`
	Usage  map[string]struct {
		Size            uint64 `json:"size"`
		NumberOfObjects uint64 `json:"num_objects"`
	}
}

func bucketStatsFromRGW(stats rgwBucketStats) model.ObjectBucketStats {
	s := model.ObjectBucketStats{Size: 0, NumberOfObjects: 0}
	for _, usage := range stats.Usage {
		s.Size = s.Size + usage.Size
		s.NumberOfObjects = s.NumberOfObjects + usage.NumberOfObjects
	}
	return s
}

func GetBucketStats(context *clusterd.Context, bucketName string, getClusterInfo func() (*mon.ClusterInfo, error)) (*model.ObjectBucketStats, bool, error) {
	result, err := RunAdminCommand(context, getClusterInfo,
		"bucket",
		"stats",
		"--bucket", bucketName)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get bucket stats: %+v", err)
	}

	if strings.Contains(result, "could not get bucket info") {
		return nil, true, fmt.Errorf("not found")
	}

	var rgwStats rgwBucketStats
	if err := json.Unmarshal([]byte(result), &rgwStats); err != nil {
		return nil, false, fmt.Errorf("failed to read buckets stats. %+v, result=%s", err, result)
	}

	stat := bucketStatsFromRGW(rgwStats)

	return &stat, false, nil
}

func GetBucketsStats(context *clusterd.Context, getClusterInfo func() (*mon.ClusterInfo, error)) (map[string]model.ObjectBucketStats, error) {
	result, err := RunAdminCommand(context, getClusterInfo,
		"bucket",
		"stats")
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %+v", err)
	}

	var rgwStats []rgwBucketStats
	if err := json.Unmarshal([]byte(result), &rgwStats); err != nil {
		return nil, fmt.Errorf("failed to read buckets stats. %+v, result=%s", err, result)
	}

	stats := map[string]model.ObjectBucketStats{}

	for _, rgwStat := range rgwStats {
		stats[rgwStat.Bucket] = bucketStatsFromRGW(rgwStat)
	}

	return stats, nil
}

func getBucketMetadata(context *clusterd.Context, bucket string, getClusterInfo func() (*mon.ClusterInfo, error)) (*model.ObjectBucketMetadata, bool, error) {
	result, err := RunAdminCommand(context, getClusterInfo,
		"metadata",
		"get",
		"bucket:"+bucket)
	if err != nil {
		return nil, false, fmt.Errorf("failed to list buckets: %+v", err)
	}

	if strings.Contains(result, "can't get key") {
		return nil, true, fmt.Errorf("not found")
	}

	var s struct {
		Data struct {
			Owner        string `json:"owner"`
			CreationTime string `json:"creation_time"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result), &s); err != nil {
		return nil, false, fmt.Errorf("failed to read buckets list. %+v, result=%s", err, result)
	}

	createdAt, err := time.Parse("2006-01-02 15:04:05.999999999Z", s.Data.CreationTime)
	if err != nil {
		return nil, false, fmt.Errorf("Error parsing date (%s): %+v", s.Data.CreationTime, err)
	}

	return &model.ObjectBucketMetadata{Owner: s.Data.Owner, CreatedAt: createdAt}, false, nil
}

func ListBuckets(context *clusterd.Context, getClusterInfo func() (*mon.ClusterInfo, error)) ([]model.ObjectBucket, error) {
	logger.Infof("Listing buckets")

	stats, err := GetBucketsStats(context, getClusterInfo)
	if err != nil {
		return nil, fmt.Errorf("Failed to get bucket stats: %+v", err)
	}

	buckets := []model.ObjectBucket{}

	for bucket, stat := range stats {
		metadata, _, err := getBucketMetadata(context, bucket, getClusterInfo)
		if err != nil {
			return nil, err
		}

		buckets = append(buckets, model.ObjectBucket{Name: bucket, ObjectBucketMetadata: model.ObjectBucketMetadata{Owner: metadata.Owner, CreatedAt: metadata.CreatedAt}, ObjectBucketStats: stat})
	}

	return buckets, nil
}

func GetBucket(context *clusterd.Context, bucket string, getClusterInfo func() (*mon.ClusterInfo, error)) (*model.ObjectBucket, int, error) {
	stat, notFound, err := GetBucketStats(context, bucket, getClusterInfo)
	if notFound {
		return nil, RGWErrorNotFound, fmt.Errorf("Bucket not found")
	}

	if err != nil {
		return nil, RGWErrorUnknown, fmt.Errorf("Failed to get bucket stats: %+v", err)
	}

	metadata, notFound, err := getBucketMetadata(context, bucket, getClusterInfo)
	if notFound {
		return nil, RGWErrorNotFound, fmt.Errorf("Bucket not found")
	}

	if err != nil {
		return nil, RGWErrorUnknown, err
	}

	return &model.ObjectBucket{Name: bucket, ObjectBucketMetadata: model.ObjectBucketMetadata{Owner: metadata.Owner, CreatedAt: metadata.CreatedAt}, ObjectBucketStats: *stat}, RGWErrorNone, nil
}

func DeleteBucket(context *clusterd.Context, bucketName string, purge bool, getClusterInfo func() (*mon.ClusterInfo, error)) (int, error) {
	options := []string{"--bucket", bucketName}
	if purge {
		options = append(options, "--purge-objects")
	}

	result, err := RunAdminCommand(context, getClusterInfo,
		"bucket",
		"rm",
		options...)
	if err != nil {
		return RGWErrorUnknown, fmt.Errorf("failed to delete bucket: %+v", err)
	}

	if result == "" {
		return RGWErrorNone, nil
	}

	if strings.Contains(result, "could not get bucket info for bucket=") {
		return RGWErrorNotFound, fmt.Errorf("Bucket not found")
	}

	return RGWErrorUnknown, fmt.Errorf("failed to delete bucket: %+v", err)
}
