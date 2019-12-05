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
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
)

type OSDUsage struct {
	OSDNodes []OSDNodeUsage `json:"nodes"`
	Summary  struct {
		TotalKB      json.Number `json:"total_kb"`
		TotalUsedKB  json.Number `json:"total_kb_used"`
		TotalAvailKB json.Number `json:"total_kb_avail"`
		AverageUtil  json.Number `json:"average_utilization"`
	} `json:"summary"`
}

type OSDNodeUsage struct {
	ID          int         `json:"id"`
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
	Flags          string              `json:"flags"`
	CrushNodeFlags map[string][]string `json:"crush_node_flags"`
}

// IsFlagSet checks if an OSD flag is set
func (dump *OSDDump) IsFlagSet(checkFlag string) bool {
	flags := strings.Split(dump.Flags, ",")
	for _, flag := range flags {
		if flag == checkFlag {
			return true
		}
	}
	return false
}

// IsFlagSetOnCrushUnit checks if an OSD flag is set on specified Crush unit
func (dump *OSDDump) IsFlagSetOnCrushUnit(checkFlag, crushUnit string) bool {
	for unit, list := range dump.CrushNodeFlags {
		if crushUnit == unit {
			for _, flag := range list {
				if flag == checkFlag {
					return true
				}
			}
		}
	}
	return false
}

// UpdateFlagOnCrushUnit checks if the flag is in the desired state and sets/unsets if it isn't. Mitigates redundant calls
// it returns true if the value was changed
func (dump *OSDDump) UpdateFlagOnCrushUnit(context *clusterd.Context, set bool, clusterName, crushUnit, flag string) (bool, error) {
	flagSet := dump.IsFlagSetOnCrushUnit(flag, crushUnit)
	if flagSet && !set {
		err := UnsetFlagOnCrushUnit(context, clusterName, crushUnit, flag)
		if err != nil {
			return true, err
		}
		return true, nil
	}
	if !flagSet && set {
		err := SetFlagOnCrushUnit(context, clusterName, crushUnit, flag)
		if err != nil {
			return true, err
		}
		return true, nil
	}
	return false, nil
}

// SetFlagOnCrushUnit sets the specified flag on the crush unit
func SetFlagOnCrushUnit(context *clusterd.Context, clusterName, crushUnit, flag string) error {
	args := []string{"osd", "set-group", flag, crushUnit}
	cmd := NewCephCommand(context, clusterName, args)
	_, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to set flag %s on %s", crushUnit, flag)
	}
	return nil
}

// UnsetFlagOnCrushUnit unsets the specified flag on the crush unit
func UnsetFlagOnCrushUnit(context *clusterd.Context, clusterName, crushUnit, flag string) error {
	args := []string{"osd", "unset-group", flag, crushUnit}
	cmd := NewCephCommand(context, clusterName, args)
	_, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to unset flag %s on %s", crushUnit, flag)
	}
	return nil
}

type SafeToDestroyStatus struct {
	SafeToDestroy []int `json:"safe_to_destroy"`
}

// OsdTree represents the CRUSH hierarchy
type OsdTree struct {
	Nodes []struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Type        string `json:"type"`
		TypeID      int    `json:"type_id"`
		Children    []int  `json:"children,omitempty"`
		PoolWeights struct {
		} `json:"pool_weights,omitempty"`
		CrushWeight     float64 `json:"crush_weight,omitempty"`
		Depth           int     `json:"depth,omitempty"`
		Exists          int     `json:"exists,omitempty"`
		Status          string  `json:"status,omitempty"`
		Reweight        float64 `json:"reweight,omitempty"`
		PrimaryAffinity float64 `json:"primary_affinity,omitempty"`
	} `json:"nodes"`
	Stray []struct {
		ID              int     `json:"id"`
		Name            string  `json:"name"`
		Type            string  `json:"type"`
		TypeID          int     `json:"type_id"`
		CrushWeight     float64 `json:"crush_weight"`
		Depth           int     `json:"depth"`
		Exists          int     `json:"exists"`
		Status          string  `json:"status"`
		Reweight        float64 `json:"reweight"`
		PrimaryAffinity float64 `json:"primary_affinity"`
	} `json:"stray"`
}

// OsdList returns the list of OSD by their IDs
type OsdList []int

// StatusByID returns status and inCluster states for given OSD id
func (dump *OSDDump) StatusByID(id int64) (int64, int64, error) {
	for _, d := range dump.OSDs {
		i, err := d.OSD.Int64()
		if err != nil {
			return 0, 0, err
		}

		if id == i {
			in, err := d.In.Int64()
			if err != nil {
				return 0, 0, err
			}

			up, err := d.Up.Int64()
			if err != nil {
				return 0, 0, err
			}

			return up, in, nil
		}
	}

	return 0, 0, errors.Errorf("not found osd.%d in OSDDump", id)
}

func GetOSDUsage(context *clusterd.Context, clusterName string) (*OSDUsage, error) {
	args := []string{"osd", "df"}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get osd df")
	}

	var osdUsage OSDUsage
	if err := json.Unmarshal(buf, &osdUsage); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal osd df response")
	}

	return &osdUsage, nil
}

func GetOSDPerfStats(context *clusterd.Context, clusterName string) (*OSDPerfStats, error) {
	args := []string{"osd", "perf"}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get osd perf")
	}

	var osdPerfStats OSDPerfStats
	if err := json.Unmarshal(buf, &osdPerfStats); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal osd perf response")
	}

	return &osdPerfStats, nil
}

func GetOSDDump(context *clusterd.Context, clusterName string) (*OSDDump, error) {
	args := []string{"osd", "dump"}
	cmd := NewCephCommand(context, clusterName, args)
	cmd.Debug = true
	buf, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get osd dump")
	}

	var osdDump OSDDump
	if err := json.Unmarshal(buf, &osdDump); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal osd dump response")
	}

	return &osdDump, nil
}

func OSDOut(context *clusterd.Context, clusterName string, osdID int) (string, error) {
	args := []string{"osd", "out", strconv.Itoa(osdID)}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	return string(buf), err
}

func OSDRemove(context *clusterd.Context, clusterName string, osdID int) (string, error) {
	args := []string{"osd", "rm", strconv.Itoa(osdID)}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	return string(buf), err
}

func OsdSafeToDestroy(context *clusterd.Context, clusterName string, osdID int, cephVersion cephver.CephVersion) (bool, error) {
	if !cephVersion.IsAtLeastNautilus() {
		logger.Debugf("failed to get safe-to-destroy status: ceph version in lower than Nautilus")
		return false, nil
	}
	args := []string{"osd", "safe-to-destroy", strconv.Itoa(osdID)}
	cmd := NewCephCommand(context, clusterName, args)
	buf, err := cmd.Run()
	if err != nil {
		return false, errors.Wrapf(err, "failed to get safe-to-destroy status")
	}

	var output SafeToDestroyStatus
	if err := json.Unmarshal(buf, &output); err != nil {
		return false, errors.Wrapf(err, "failed to unmarshal safe-to-destroy response")
	}
	if len(output.SafeToDestroy) != 0 && output.SafeToDestroy[0] == osdID {
		return true, nil
	}
	return false, nil
}

func (usage *OSDUsage) ByID(osdID int) *OSDNodeUsage {
	for i := range usage.OSDNodes {
		if usage.OSDNodes[i].ID == osdID {
			return &usage.OSDNodes[i]
		}
	}

	return nil
}

// HostTree returns the osd tree
func HostTree(context *clusterd.Context, clusterName string) (OsdTree, error) {
	var output OsdTree

	args := []string{"osd", "tree"}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return output, errors.Wrapf(err, "failed to get osd tree")
	}

	err = json.Unmarshal(buf, &output)
	if err != nil {
		return output, errors.Wrapf(err, "failed to unmarshal 'osd tree' response")
	}

	return output, nil
}

// OsdListNum returns the list of OSDs
func OsdListNum(context *clusterd.Context, clusterName string) (OsdList, error) {
	var output OsdList

	args := []string{"osd", "ls"}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return output, errors.Wrapf(err, "failed to get osd list")
	}

	err = json.Unmarshal(buf, &output)
	if err != nil {
		return output, errors.Wrapf(err, "failed to unmarshal 'osd ls' response")
	}

	return output, nil
}
