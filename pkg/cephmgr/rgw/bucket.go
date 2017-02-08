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
	"time"

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

func GetBucketStats(context *clusterd.Context) (map[string]model.ObjectBucketStats, error) {
	result, err := RunAdminCommand(context,
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
		stat := model.ObjectBucketStats{Size: 0, NumberOfObjects: 0}
		for _, usage := range rgwStat.Usage {
			stat.Size = stat.Size + usage.Size
			stat.NumberOfObjects = stat.NumberOfObjects + usage.NumberOfObjects
		}
		stats[rgwStat.Bucket] = stat
	}

	return stats, nil
}

func ListBuckets(context *clusterd.Context) ([]model.ObjectBucket, error) {
	logger.Infof("Listing buckets")

	stats, err := GetBucketStats(context)
	if err != nil {
		return nil, fmt.Errorf("Failed to get bucket stats: %+v", err)
	}

	buckets := []model.ObjectBucket{}

	for bucket, stat := range stats {
		result, err := RunAdminCommand(context,
			"metadata",
			"get",
			"bucket:"+bucket)
		if err != nil {
			return nil, fmt.Errorf("failed to list buckets: %+v", err)
		}

		var s struct {
			Data struct {
				Owner        string `json:"owner"`
				CreationTime string `json:"creation_time"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(result), &s); err != nil {
			return nil, fmt.Errorf("failed to read buckets list. %+v, result=%s", err, result)
		}

		createdAt, err := time.Parse("2006-01-02 15:04:05.999999999Z", s.Data.CreationTime)
		if err != nil {
			return nil, fmt.Errorf("Error parsing date (%s): %+v", s.Data.CreationTime, err)
		}

		buckets = append(buckets, model.ObjectBucket{Name: bucket, Owner: s.Data.Owner, CreatedAt: createdAt, ObjectBucketStats: stat})
	}

	return buckets, nil
}
