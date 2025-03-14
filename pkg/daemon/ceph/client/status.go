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
	"regexp"

	"github.com/pkg/errors"
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
	defaultPgHealthyRegex    = `^(active\+clean|active\+clean\+scrubbing|active\+clean\+scrubbing\+deep)$`
)

var defaultPgHealthyRegexCompiled = regexp.MustCompile(defaultPgHealthyRegex)

type CephStatus struct {
	Health        HealthStatus `json:"health"`
	FSID          string       `json:"fsid"`
	ElectionEpoch int          `json:"election_epoch"`
	Quorum        []int        `json:"quorum"`
	QuorumNames   []string     `json:"quorum_names"`
	MonMap        MonMap       `json:"monmap"`
	OsdMap        OsdMap       `json:"osdmap"`
	PgMap         PgMap        `json:"pgmap"`
	MgrMap        MgrMap       `json:"mgrmap"`
	Fsmap         Fsmap        `json:"fsmap"`
}

type HealthStatus struct {
	Status string                  `json:"status"`
	Checks map[string]CheckMessage `json:"checks"`
}

type CheckMessage struct {
	Severity string  `json:"severity"`
	Summary  Summary `json:"summary"`
}

type Summary struct {
	Message string `json:"message"`
}

type MonMap struct {
	Epoch        int           `json:"epoch"`
	NumMons      int           `json:"num_mons"`
	FSID         string        `json:"fsid"`
	CreatedTime  string        `json:"created"`
	ModifiedTime string        `json:"modified"`
	Mons         []MonMapEntry `json:"mons"`
}

type MgrStat struct {
	Epoch      int    `json:"epoch"`
	Available  bool   `json:"available"`
	ActiveName string `json:"active_name"`
	NumStandby int    `json:"num_standby"`
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

// Fsmap is a struct representing the filesystem map
type Fsmap struct {
	Epoch  int `json:"epoch"`
	ID     int `json:"id"`
	Up     int `json:"up"`
	In     int `json:"in"`
	Max    int `json:"max"`
	ByRank []struct {
		FilesystemID int    `json:"filesystem_id"`
		Rank         int    `json:"rank"`
		Name         string `json:"name"`
		Status       string `json:"status"`
		Gid          int    `json:"gid"`
	} `json:"by_rank"`
	UpStandby int `json:"up:standby"`
}

func Status(context *clusterd.Context, clusterInfo *ClusterInfo) (CephStatus, error) {
	args := []string{"status"}
	cmd := NewCephCommand(context, clusterInfo, args)
	buf, err := cmd.Run()
	if err != nil {
		return CephStatus{}, errors.Wrapf(err, "failed to get status. %s", string(buf))
	}

	var status CephStatus
	if err := json.Unmarshal(buf, &status); err != nil {
		return CephStatus{}, errors.Wrap(err, "failed to unmarshal status response")
	}

	return status, nil
}

func StatusWithUser(context *clusterd.Context, clusterInfo *ClusterInfo) (CephStatus, error) {
	args := []string{"status", "--format", "json"}
	command, args := FinalizeCephCommandArgs("ceph", clusterInfo, args, context.ConfigDir)

	buf, err := context.Executor.ExecuteCommandWithOutput(command, args...)
	if err != nil {
		if buf != "" {
			return CephStatus{}, errors.Wrapf(err, "failed to get status. %s", string(buf))
		}
		return CephStatus{}, errors.Wrap(err, "failed to get ceph status")
	}

	var status CephStatus
	if err := json.Unmarshal([]byte(buf), &status); err != nil {
		return CephStatus{}, errors.Wrap(err, "failed to unmarshal status response")
	}

	return status, nil
}

// IsClusterClean returns msg (string), clean (bool), err (error)
// msg describes the state of the PGs
// clean is true if the cluster is clean
// err is not nil if getting the status failed.
func IsClusterClean(context *clusterd.Context, clusterInfo *ClusterInfo, pgHealthyRegex string) (string, bool, error) {
	status, err := Status(context, clusterInfo)
	if err != nil {
		return "unable to get PG health", false, err
	}

	pgHealthyRegexCompiled := defaultPgHealthyRegexCompiled
	if pgHealthyRegex != "" {
		pgHealthyRegexCompiled, err = regexp.Compile(pgHealthyRegex)
		if err != nil {
			return "unable to compile pgHealthyRegex", false, err
		}
	}

	msg, clean := isClusterClean(status, pgHealthyRegexCompiled)
	if !clean {
		return msg, false, nil
	}
	return msg, true, nil
}

// IsClusterCleanError returns an error indicating if the cluster is fully clean yet (i.e., all placement
// groups are in the active+clean state). It returns nil if the cluster is clean.
// Using IsClusterClean is recommended if you want to differentiate between a failure of the status query and
// an unclean cluster.
func IsClusterCleanError(context *clusterd.Context, clusterInfo *ClusterInfo, pgHealthyRegex string) error {
	msg, clean, err := IsClusterClean(context, clusterInfo, pgHealthyRegex)
	if err != nil {
		return err
	}
	if !clean {
		return errors.New(msg)
	}
	return nil
}

func isClusterClean(status CephStatus, pgHealthyRegex *regexp.Regexp) (string, bool) {
	if status.PgMap.NumPgs == 0 {
		// there are no PGs yet, that still counts as clean
		return "cluster has no PGs", true
	}

	cleanPGs := 0
	for _, pg := range status.PgMap.PgsByState {
		if pgHealthyRegex.MatchString(pg.StateName) {
			cleanPGs += pg.Count
		}
	}
	if cleanPGs == status.PgMap.NumPgs {
		// all PGs in the cluster are in a clean state
		logger.Debugf("all placement groups have reached a clean state: %+v", status.PgMap.PgsByState)
		return "all PGs in cluster are clean", true
	}

	return fmt.Sprintf("cluster is not fully clean. PGs: %+v", status.PgMap.PgsByState), false
}

// getMDSRank returns the rank of a given MDS
func getMDSRank(status CephStatus, fsName string) (int, error) {
	// dummy rank
	mdsRank := -1000
	for r := range status.Fsmap.ByRank {
		if status.Fsmap.ByRank[r].Name == fsName {
			mdsRank = r
		}
	}
	// if the mds is not shown in the map one reason might be because it's in standby
	// if not in standby there is something else going wrong
	if mdsRank < 0 && status.Fsmap.UpStandby < 1 {
		// it might seem strange to log an error since this could be a warning too
		// it is a warning until we reach the timeout, this should give enough time to the mds to transition its state
		// after the timeout we consider that the mds might be gone or the timeout was not long enough...
		return mdsRank, errors.Errorf("mds %s not found in fsmap, this likely means mdss are transitioning between active and standby states", fsName)
	}

	return mdsRank, nil
}

// MdsActiveOrStandbyReplay returns whether a given MDS is active or in standby
func MdsActiveOrStandbyReplay(context *clusterd.Context, clusterInfo *ClusterInfo, fsName string) error {
	status, err := Status(context, clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get ceph status")
	}

	mdsRank, err := getMDSRank(status, fsName)
	if err != nil {
		return errors.Cause(err)
	}

	// this MDS is in standby so let's return immediately
	if mdsRank < 0 {
		logger.Infof("mds %s is in standby, nothing to check", fsName)
		return nil
	}

	if status.Fsmap.ByRank[mdsRank].Status == "up:active" || status.Fsmap.ByRank[mdsRank].Status == "up:standby-replay" || status.Fsmap.ByRank[mdsRank].Status == "up:standby" {
		logger.Infof("mds %s is %s", fsName, status.Fsmap.ByRank[mdsRank].Status)
		return nil
	}

	return errors.Errorf("mds %s is %s, bad state", fsName, status.Fsmap.ByRank[mdsRank].Status)
}

// IsCephHealthy verifies Ceph is healthy, useful when performing an upgrade
// check if it's a minor or major upgrade... too!
func IsCephHealthy(context *clusterd.Context, clusterInfo *ClusterInfo) bool {
	cephStatus, err := Status(context, clusterInfo)
	if err != nil {
		logger.Errorf("failed to detect if Ceph is healthy. failed to get ceph status. %v", err)
		return false
	}

	return isCephHealthy(cephStatus)
}

func isCephHealthy(status CephStatus) bool {
	s := status.Health.Status
	if s == "HEALTH_WARN" || s == "HEALTH_OK" {
		return true
	}

	return false
}
