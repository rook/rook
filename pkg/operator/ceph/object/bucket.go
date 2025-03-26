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

package object

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	timeParser = "2006-01-02T15:04:05.999999999Z"
)

type ObjectBucketMetadata struct {
	Owner     string    `json:"owner"`
	CreatedAt time.Time `json:"createdAt"`
}

type ObjectBucketStats struct {
	Size            uint64 `json:"size"`
	NumberOfObjects uint64 `json:"numberOfObjects"`
}

type ObjectBucket struct {
	Name string `json:"name"`
	ObjectBucketMetadata
	ObjectBucketStats
}

type rgwBucketStats struct {
	Bucket string `json:"bucket"`
	Usage  map[string]struct {
		Size            uint64 `json:"size"`
		NumberOfObjects uint64 `json:"num_objects"`
	}
}

type ObjectBuckets []ObjectBucket

func (slice ObjectBuckets) Len() int {
	return len(slice)
}

func (slice ObjectBuckets) Less(i, j int) bool {
	return slice[i].Name < slice[j].Name
}

func (slice ObjectBuckets) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

func bucketStatsFromRGW(stats rgwBucketStats) ObjectBucketStats {
	s := ObjectBucketStats{Size: 0, NumberOfObjects: 0}
	for _, usage := range stats.Usage {
		s.Size = s.Size + usage.Size
		s.NumberOfObjects = s.NumberOfObjects + usage.NumberOfObjects
	}
	return s
}

func GetBucketStats(c *Context, bucketName string) (*ObjectBucketStats, bool, error) {
	result, err := runAdminCommand(c,
		true,
		"bucket",
		"stats",
		"--bucket", bucketName)
	if err != nil {
		if strings.Contains(err.Error(), "exit status 2") {
			return nil, true, errors.New("not found")
		} else {
			return nil, false, errors.Wrap(err, "failed to get bucket stats")
		}
	}

	var rgwStats rgwBucketStats
	if err := json.Unmarshal([]byte(result), &rgwStats); err != nil {
		return nil, false, errors.Wrapf(err, "failed to read buckets stats result=%s", result)
	}

	stat := bucketStatsFromRGW(rgwStats)

	return &stat, false, nil
}

func GetBucketsStats(c *Context) (map[string]ObjectBucketStats, error) {
	result, err := runAdminCommand(c,
		true,
		"bucket",
		"stats")
	if err != nil {
		return nil, errors.Wrap(err, "failed to list buckets")
	}

	var rgwStats []rgwBucketStats
	if err := json.Unmarshal([]byte(result), &rgwStats); err != nil {
		return nil, errors.Wrapf(err, "failed to read buckets stats result=%s", result)
	}

	stats := map[string]ObjectBucketStats{}

	for _, rgwStat := range rgwStats {
		stats[rgwStat.Bucket] = bucketStatsFromRGW(rgwStat)
	}

	return stats, nil
}

func getBucketMetadata(c *Context, bucket string) (*ObjectBucketMetadata, bool, error) {
	result, err := runAdminCommand(c,
		false,
		"metadata",
		"get",
		"bucket:"+bucket)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to list buckets")
	}

	if strings.Contains(result, "can't get key") {
		return nil, true, errors.New("not found")
	}
	match, err := extractJSON(result)
	if err != nil {
		return nil, false, errors.Wrapf(err, "failed to read buckets list result=%s", result)
	}

	var s struct {
		Data struct {
			Owner        string `json:"owner"`
			CreationTime string `json:"creation_time"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(match), &s); err != nil {
		return nil, false, errors.Wrapf(err, "failed to read buckets list result=%s", match)
	}

	createdAt, err := time.Parse(timeParser, s.Data.CreationTime)
	if err != nil {
		return nil, false, errors.Wrapf(err, "Error parsing date (%s)", s.Data.CreationTime)
	}

	return &ObjectBucketMetadata{Owner: s.Data.Owner, CreatedAt: createdAt}, false, nil
}

func GetBucket(c *Context, bucket string) (*ObjectBucket, int, error) {
	stat, notFound, err := GetBucketStats(c, bucket)
	if notFound {
		return nil, RGWErrorNotFound, errors.New("Bucket not found")
	}

	if err != nil {
		return nil, RGWErrorUnknown, errors.Wrap(err, "Failed to get bucket stats")
	}

	metadata, notFound, err := getBucketMetadata(c, bucket)
	if notFound {
		return nil, RGWErrorNotFound, errors.New("Bucket not found")
	}

	if err != nil {
		return nil, RGWErrorUnknown, err
	}

	return &ObjectBucket{Name: bucket, ObjectBucketMetadata: ObjectBucketMetadata{Owner: metadata.Owner, CreatedAt: metadata.CreatedAt}, ObjectBucketStats: *stat}, RGWErrorNone, nil
}
