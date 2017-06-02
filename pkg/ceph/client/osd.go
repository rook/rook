/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Some of the code below came from https://github.com/digitalocean/ceph_exporter
which has the same license.
*/
package client

import (
	"encoding/json"
	"fmt"

	"github.com/rook/rook/pkg/clusterd"
)

type OSDUsage struct {
	OSDNodes []struct {
		Name        string      `json:"name"`
		CrushWeight json.Number `json:"crush_weight"`
		Depth       json.Number `json:"depth"`
		Reweight    json.Number `json:"reweight"`
		KB          json.Number `json:"kb"`
		UsedKB      json.Number `json:"kb_used"`
		AvailKB     json.Number `json:"kb_avail"`
		Utilization json.Number `json:"utilization"`
		Variance    json.Number `json:"var"`
		Pgs         json.Number `json:"pgs"`
	} `json:"nodes"`

	Summary struct {
		TotalKB      json.Number `json:"total_kb"`
		TotalUsedKB  json.Number `json:"total_kb_used"`
		TotalAvailKB json.Number `json:"total_kb_avail"`
		AverageUtil  json.Number `json:"average_utilization"`
	} `json:"summary"`
}

type OSDPerfStats struct {
	PerfInfo []struct {
		ID    json.Number `json:"id"`
		Stats struct {
			CommitLatency json.Number `json:"commit_latency_ms"`
			ApplyLatency  json.Number `json:"apply_latency_ms"`
		} `json:"perf_stats"`
	} `json:"osd_perf_infos"`
}

type OSDDump struct {
	OSDs []struct {
		OSD json.Number `json:"osd"`
		Up  json.Number `json:"up"`
		In  json.Number `json:"in"`
	} `json:"osds"`
}

func GetOSDUsage(context *clusterd.Context, clusterName string) (*OSDUsage, error) {
	args := []string{"osd", "df"}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return nil, fmt.Errorf("failed to get osd df: %+v", err)
	}

	var osdUsage OSDUsage
	if err := json.Unmarshal(buf, &osdUsage); err != nil {
		return nil, fmt.Errorf("failed to unmarshal osd df response: %+v", err)
	}

	return &osdUsage, nil
}

func GetOSDPerfStats(context *clusterd.Context, clusterName string) (*OSDPerfStats, error) {
	args := []string{"osd", "perf"}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return nil, fmt.Errorf("failed to get osd perf: %+v", err)
	}

	var osdPerfStats OSDPerfStats
	if err := json.Unmarshal(buf, &osdPerfStats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal osd perf response: %+v", err)
	}

	return &osdPerfStats, nil
}

func GetOSDDump(context *clusterd.Context, clusterName string) (*OSDDump, error) {
	args := []string{"osd", "dump"}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return nil, fmt.Errorf("failed to get osd dump: %+v", err)
	}

	var osdDump OSDDump
	if err := json.Unmarshal(buf, &osdDump); err != nil {
		return nil, fmt.Errorf("failed to unmarshal osd dump response: %+v", err)
	}

	return &osdDump, nil
}
