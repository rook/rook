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
	"strconv"

	"github.com/rook/rook/pkg/clusterd"
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

	return 0, 0, fmt.Errorf("not found osd.%d in OSDDump", id)
}

func GetOSDUsage(context *clusterd.Context, clusterName string) (*OSDUsage, error) {
	args := []string{"osd", "df"}
	buf, err := NewCephCommand(context, clusterName, args).Run()
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
	buf, err := NewCephCommand(context, clusterName, args).Run()
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
	cmd := NewCephCommand(context, clusterName, args)
	cmd.Debug = true
	buf, err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to get osd dump: %+v", err)
	}

	var osdDump OSDDump
	if err := json.Unmarshal(buf, &osdDump); err != nil {
		return nil, fmt.Errorf("failed to unmarshal osd dump response: %+v", err)
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

func OsdSafeToDestroy(context *clusterd.Context, clusterName string, osdID int) (bool, error) {
	args := []string{"osd", "safe-to-destroy", strconv.Itoa(osdID)}
	cmd := NewCephCommand(context, clusterName, args)
	buf, err := cmd.Run()
	if err != nil {
		return false, fmt.Errorf("failed to get safe-to-destroy status: %+v", err)
	}

	var output SafeToDestroyStatus
	if err := json.Unmarshal(buf, &output); err != nil {
		return false, fmt.Errorf("failed to unmarshal safe-to-destroy response: %+v", err)
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
		return output, fmt.Errorf("failed to get osd tree: %+v", err)
	}

	err = json.Unmarshal(buf, &output)
	if err != nil {
		return output, fmt.Errorf("failed to unmarshal 'osd tree' response: %+v", err)
	}

	return output, nil
}

// OsdListNum returns the list of OSDs
func OsdListNum(context *clusterd.Context, clusterName string) (OsdList, error) {
	var output OsdList

	args := []string{"osd", "ls"}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return output, fmt.Errorf("failed to get osd list: %+v", err)
	}

	err = json.Unmarshal(buf, &output)
	if err != nil {
		return output, fmt.Errorf("failed to unmarshal 'osd ls' response: %+v", err)
	}

	return output, nil
}
