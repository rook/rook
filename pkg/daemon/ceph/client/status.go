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
*/
package client

import (
	"encoding/json"
	"fmt"

	"github.com/rook/rook/pkg/clusterd"
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

const (
	activeClean              = "active+clean"
	activeCleanScrubbing     = "active+clean+scrubbing"
	activeCleanScrubbingDeep = "active+clean+scrubbing+deep"
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
	PgMap  PgMap  `json:"pgmap"`
	MgrMap MgrMap `json:"mgrmap"`
}

type HealthStatus struct {
	Status string                  `json:"status"`
	Checks map[string]CheckMessage `json:"checks"`
}

type CheckMessage struct {
	Severity string `json:"severity"`
	Summary  struct {
		Message string `json:"message"`
	} `json:"summary"`
}

type MonMap struct {
	Epoch        int           `json:"epoch"`
	FSID         string        `json:"fsid"`
	CreatedTime  string        `json:"created"`
	ModifiedTime string        `json:"modified"`
	Mons         []MonMapEntry `json:"mons"`
}

type MgrMap struct {
	Epoch      int          `json:"epoch"`
	ActiveGID  int          `json:"active_gid"`
	ActiveName string       `json:"active_name"`
	ActiveAddr string       `json:"active_addr"`
	Available  bool         `json:"available"`
	Standbys   []MgrStandby `json:"standbys"`
}

type MgrStandby struct {
	GID  int    `json:"gid"`
	Name string `json:"name"`
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
	PgsByState            []PgStateEntry `json:"pgs_by_state"`
	Version               int            `json:"version"`
	NumPgs                int            `json:"num_pgs"`
	DataBytes             uint64         `json:"data_bytes"`
	UsedBytes             uint64         `json:"bytes_used"`
	AvailableBytes        uint64         `json:"bytes_avail"`
	TotalBytes            uint64         `json:"bytes_total"`
	ReadBps               uint64         `json:"read_bytes_sec"`
	WriteBps              uint64         `json:"write_bytes_sec"`
	ReadOps               uint64         `json:"read_op_per_sec"`
	WriteOps              uint64         `json:"write_op_per_sec"`
	RecoveryBps           uint64         `json:"recovering_bytes_per_sec"`
	RecoveryObjectsPerSec uint64         `json:"recovering_objects_per_sec"`
	RecoveryKeysPerSec    uint64         `json:"recovering_keys_per_sec"`
	CacheFlushBps         uint64         `json:"flush_bytes_sec"`
	CacheEvictBps         uint64         `json:"evict_bytes_sec"`
	CachePromoteBps       uint64         `json:"promote_op_per_sec"`
}

type PgStateEntry struct {
	StateName string `json:"state_name"`
	Count     int    `json:"count"`
}

func Status(context *clusterd.Context, clusterName string, debug bool) (CephStatus, error) {
	args := []string{"status"}
	buf, err := ExecuteCephCommandDebug(context, clusterName, debug, args)
	if err != nil {
		return CephStatus{}, fmt.Errorf("failed to get status: %+v", err)
	}

	var status CephStatus
	if err := json.Unmarshal(buf, &status); err != nil {
		return CephStatus{}, fmt.Errorf("failed to unmarshal status response: %+v", err)
	}

	return status, nil
}

// IsClusterClean returns a value indicating if the cluster is fully clean yet (i.e., all placement
// groups are in the active+clean state).
func IsClusterClean(context *clusterd.Context, clusterName string) error {
	status, err := Status(context, clusterName, false)
	if err != nil {
		return err
	}

	return isClusterClean(status)
}

func isClusterClean(status CephStatus) error {
	if status.PgMap.NumPgs == 0 {
		// there are no PGs yet, that still counts as clean
		return nil
	}

	cleanPGs := 0
	for _, pg := range status.PgMap.PgsByState {
		if pg.StateName == activeClean || pg.StateName == activeCleanScrubbing || pg.StateName == activeCleanScrubbingDeep {
			cleanPGs += pg.Count
		}
	}
	if cleanPGs == status.PgMap.NumPgs {
		// all PGs in the cluster are in a clean state
		logger.Infof("all placement groups have reached a clean state: %+v", status.PgMap.PgsByState)
		return nil
	}

	return fmt.Errorf("cluster is not fully clean. PGs: %+v", status.PgMap.PgsByState)
}
