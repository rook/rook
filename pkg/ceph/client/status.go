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
	"github.com/rook/rook/pkg/model"
)

const (
	// CephHealthOK denotes the status of ceph cluster when healthy.
	CephHealthOK = "HEALTH_OK"

	// CephHealthWarn denotes the status of ceph cluster when unhealthy but recovering.
	CephHealthWarn = "HEALTH_WARN"

	// CephHealthErr denotes the status of ceph cluster when unhealthy but usually needs
	// manual intervention.
	CephHealthErr = "HEALTH_ERR"
)

type CephStatus struct {
	Health        HealthStatus `json:"health"`
	FSID          string       `json:"fsid"`
	ElectionEpoch int          `json:"election_epoch"`
	Quorum        []int        `json:"quorum"`
	QuorumNames   []string     `json:"quorum_names"`
	MonMap        MonMap       `json:"monmap"`
	OsdMap        struct {
		OsdMap OsdMap `json:"osdmap"`
	} `json:"osdmap"`
	PgMap PgMap `json:"pgmap"`
}

type HealthStatus struct {
	Details struct {
		Services []map[string][]HealthService `json:"health_services"`
	} `json:"health"`
	Timechecks struct {
		Epoch       int    `json:"epoch"`
		Round       int    `json:"round"`
		RoundStatus string `json:"round_status"`
	} `json:"timechecks"`
	Summary       []HealthSummary `json:"summary"`
	OverallStatus string          `json:"overall_status"`
	Detail        []interface{}   `json:"detail"`
}

type HealthService struct {
	Name             string `json:"name"`
	Health           string `json:"health"`
	KbTotal          uint64 `json:"kb_total"`
	KbUsed           uint64 `json:"kb_used"`
	KbAvailable      uint64 `json:"kb_avail"`
	AvailablePercent int    `json:"avail_percent"`
	LastUpdated      string `json:"last_updated"`
	StoreStats       struct {
		BytesTotal  uint64 `json:"bytes_total"`
		BytesSst    uint64 `json:"bytes_sst"`
		BytesLog    uint64 `json:"bytes_log"`
		BytesMisc   uint64 `json:"bytes_misc"`
		LastUpdated string `json:"last_updated"`
	} `json:"store_stats"`
}

type HealthSummary struct {
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
}

type MonMap struct {
	Epoch        int           `json:"epoch"`
	FSID         string        `json:"fsid"`
	CreatedTime  string        `json:"created"`
	ModifiedTime string        `json:"modified"`
	Mons         []MonMapEntry `json:"mons"`
}

type OsdMap struct {
	Epoch          int  `json:"epoch"`
	NumOsd         int  `json:"num_osds"`
	NumUpOsd       int  `json:"num_up_osds"`
	NumInOsd       int  `json:"num_in_osds"`
	Full           bool `json:"full"`
	NearFull       bool `json:"nearfull"`
	NumRemappedPgs int  `json:"num_remapped_pgs"`
}

type PgMap struct {
	PgsByState     []PgStateEntry `json:"pgs_by_state"`
	Version        int            `json:"version"`
	NumPgs         int            `json:"num_pgs"`
	DataBytes      uint64         `json:"data_bytes"`
	UsedBytes      uint64         `json:"bytes_used"`
	AvailableBytes uint64         `json:"bytes_avail"`
	TotalBytes     uint64         `json:"bytes_total"`
}

type PgStateEntry struct {
	StateName string `json:"state_name"`
	Count     int    `json:"count"`
}

func Status(context *clusterd.Context, clusterName string) (CephStatus, error) {
	args := []string{"status", "--format", "json"}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return CephStatus{}, fmt.Errorf("failed to get status: %+v", err)
	}

	var status CephStatus
	if err := json.Unmarshal(buf, &status); err != nil {
		return CephStatus{}, fmt.Errorf("failed to unmarshal status response: %+v", err)
	}

	return status, nil
}

func StatusPlain(context *clusterd.Context, clusterName string) ([]byte, error) {
	args := []string{"status"}
	buf, err := ExecuteCephCommandPlain(context, clusterName, args)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %+v", err)
	}

	return buf, nil
}

func HealthToModelHealthStatus(cephHealth string) model.HealthStatus {
	switch cephHealth {
	case CephHealthOK:
		return model.HealthOK
	case CephHealthWarn:
		return model.HealthWarning
	case CephHealthErr:
		return model.HealthError
	default:
		return model.HealthUnknown
	}
}

func GetMonitorHealthSummaries(cephStatus CephStatus) []HealthService {
	// of all the available health services, we are looking for the one called "mons",
	// which will then be a collection of monitor healths
	for _, hs := range cephStatus.Health.Details.Services {
		monsHealth, ok := hs["mons"]
		if ok {
			return monsHealth
		}
	}

	return nil
}
