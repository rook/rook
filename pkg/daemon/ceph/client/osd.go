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
	"math"
	"strconv"
	"strings"

	"github.com/pkg/errors"
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
	DeviceClass string      `json:"device_class"`
	CrushWeight json.Number `json:"crush_weight"`
	Depth       json.Number `json:"depth"`
	Reweight    json.Number `json:"reweight"`
	KB          json.Number `json:"kb"` // KB is in KiB units
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
	Flags             string              `json:"flags"`
	CrushNodeFlags    map[string][]string `json:"crush_node_flags"`
	FullRatio         float64             `json:"full_ratio"`
	BackfillFullRatio float64             `json:"backfillfull_ratio"`
	NearFullRatio     float64             `json:"nearfull_ratio"`
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
func (dump *OSDDump) UpdateFlagOnCrushUnit(context *clusterd.Context, clusterInfo *ClusterInfo, set bool, crushUnit, flag string) (bool, error) {
	flagSet := dump.IsFlagSetOnCrushUnit(flag, crushUnit)
	if flagSet && !set {
		err := UnsetFlagOnCrushUnit(context, clusterInfo, crushUnit, flag)
		if err != nil {
			return true, err
		}
		return true, nil
	}
	if !flagSet && set {
		err := SetFlagOnCrushUnit(context, clusterInfo, crushUnit, flag)
		if err != nil {
			return true, err
		}
		return true, nil
	}
	return false, nil
}

// SetFlagOnCrushUnit sets the specified flag on the crush unit
func SetFlagOnCrushUnit(context *clusterd.Context, clusterInfo *ClusterInfo, crushUnit, flag string) error {
	args := []string{"osd", "set-group", flag, crushUnit}
	cmd := NewCephCommand(context, clusterInfo, args)
	_, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to set flag %s on %s", crushUnit, flag)
	}
	return nil
}

// UnsetFlagOnCrushUnit unsets the specified flag on the crush unit
func UnsetFlagOnCrushUnit(context *clusterd.Context, clusterInfo *ClusterInfo, crushUnit, flag string) error {
	args := []string{"osd", "unset-group", flag, crushUnit}
	cmd := NewCephCommand(context, clusterInfo, args)
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
		ID              int      `json:"id"`
		Name            string   `json:"name"`
		Type            string   `json:"type"`
		TypeID          int      `json:"type_id"`
		Children        []int    `json:"children,omitempty"`
		PoolWeights     struct{} `json:"pool_weights,omitempty"`
		CrushWeight     float64  `json:"crush_weight,omitempty"`
		Depth           int      `json:"depth,omitempty"`
		Exists          int      `json:"exists,omitempty"`
		Status          string   `json:"status,omitempty"`
		Reweight        float64  `json:"reweight,omitempty"`
		PrimaryAffinity float64  `json:"primary_affinity,omitempty"`
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

func GetOSDUsage(context *clusterd.Context, clusterInfo *ClusterInfo) (*OSDUsage, error) {
	args := []string{"osd", "df"}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get osd df")
	}

	var osdUsage OSDUsage
	if err := json.Unmarshal(buf, &osdUsage); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal osd df response")
	}

	return &osdUsage, nil
}

func convertKibibytesToTebibytes(kib string) (float64, error) {
	kibFloat, err := strconv.ParseFloat(kib, 64)
	if err != nil {
		return float64(0), errors.Wrap(err, "failed to convert string to float")
	}
	return kibFloat / float64(1024*1024*1024), nil
}

func ResizeOsdCrushWeight(actualOSD OSDNodeUsage, ctx *clusterd.Context, clusterInfo *ClusterInfo) (bool, error) {
	currentCrushWeight, err := strconv.ParseFloat(actualOSD.CrushWeight.String(), 64)
	if err != nil {
		return false, errors.Wrapf(err, "failed converting string to float for osd.%d crush weight %q", actualOSD.ID, actualOSD.CrushWeight.String())
	}
	// actualOSD.KB is in KiB units
	calculatedCrushWeight, err := convertKibibytesToTebibytes(actualOSD.KB.String())
	if err != nil {
		return false, errors.Wrapf(err, "failed to convert KiB to TiB for osd.%d crush weight %q", actualOSD.ID, actualOSD.KB.String())
	}

	// do not reweight if the calculated crush weight is 0 or less than equal to actualCrushWeight or there percentage resize is less than 1 percent
	if calculatedCrushWeight == float64(0) {
		logger.Debugf("osd size is 0 for osd.%d, not resizing the crush weights", actualOSD.ID)
		return false, nil
	} else if calculatedCrushWeight <= currentCrushWeight {
		logger.Debugf("calculatedCrushWeight %f is less then current currentCrushWeight %f for osd.%d, not resizing the crush weights", calculatedCrushWeight, currentCrushWeight, actualOSD.ID)
		return false, nil
	} else if math.Abs(((calculatedCrushWeight - currentCrushWeight) / currentCrushWeight)) <= 0.01 {
		logger.Debugf("calculatedCrushWeight %f is less then 1 percent increased from currentCrushWeight %f for osd.%d, not resizing the crush weights", calculatedCrushWeight, currentCrushWeight, actualOSD.ID)
		return false, nil
	}

	calculatedCrushWeightString := fmt.Sprintf("%f", calculatedCrushWeight)
	logger.Infof("updating osd.%d crush weight to %q for cluster in namespace %q", actualOSD.ID, calculatedCrushWeightString, clusterInfo.Namespace)
	args := []string{"osd", "crush", "reweight", fmt.Sprintf("osd.%d", actualOSD.ID), calculatedCrushWeightString}
	buf, err := NewCephCommand(ctx, clusterInfo, args).Run()
	if err != nil {
		return false, errors.Wrapf(err, "failed to reweight osd.%d for cluster in namespace %q from actual crush weight %f to calculated crush weight %f: %s", actualOSD.ID, clusterInfo.Namespace, currentCrushWeight, calculatedCrushWeight, string(buf))
	}

	return true, nil
}

func SetDeviceClass(context *clusterd.Context, clusterInfo *ClusterInfo, osdID int, deviceClass string) error {
	// First remove the existing device class
	args := []string{"osd", "crush", "rm-device-class", fmt.Sprintf("osd.%d", osdID)}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrap(err, "failed to remove device class. "+string(buf))
	}

	// Second, apply the desired device class
	args = []string{"osd", "crush", "set-device-class", deviceClass, fmt.Sprintf("osd.%d", osdID)}
	buf, err = NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrap(err, "failed to set the device class. "+string(buf))
	}

	return nil
}

func GetOSDPerfStats(context *clusterd.Context, clusterInfo *ClusterInfo) (*OSDPerfStats, error) {
	args := []string{"osd", "perf"}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get osd perf")
	}

	var osdPerfStats OSDPerfStats
	if err := json.Unmarshal(buf, &osdPerfStats); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal osd perf response")
	}

	return &osdPerfStats, nil
}

func GetOSDDump(context *clusterd.Context, clusterInfo *ClusterInfo) (*OSDDump, error) {
	args := []string{"osd", "dump"}
	cmd := NewCephCommand(context, clusterInfo, args)
	buf, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get osd dump")
	}

	var osdDump OSDDump
	if err := json.Unmarshal(buf, &osdDump); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal osd dump response")
	}

	return &osdDump, nil
}

func OSDOut(context *clusterd.Context, clusterInfo *ClusterInfo, osdID int) (string, error) {
	args := []string{"osd", "out", strconv.Itoa(osdID)}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	return string(buf), err
}

func OsdSafeToDestroy(context *clusterd.Context, clusterInfo *ClusterInfo, osdID int) (bool, error) {
	args := []string{"osd", "safe-to-destroy", strconv.Itoa(osdID)}
	cmd := NewCephCommand(context, clusterInfo, args)
	buf, err := cmd.Run()
	if err != nil {
		return false, errors.Wrap(err, "failed to get safe-to-destroy status")
	}

	var output SafeToDestroyStatus
	if err := json.Unmarshal(buf, &output); err != nil {
		return false, errors.Wrapf(err, "failed to unmarshal safe-to-destroy response. %s", string(buf))
	}
	if len(output.SafeToDestroy) != 0 && output.SafeToDestroy[0] == osdID {
		return true, nil
	}
	return false, nil
}

// HostTree returns the osd tree
func HostTree(context *clusterd.Context, clusterInfo *ClusterInfo) (OsdTree, error) {
	var output OsdTree

	args := []string{"osd", "tree"}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return output, errors.Wrap(err, "failed to get osd tree")
	}

	err = json.Unmarshal(buf, &output)
	if err != nil {
		return output, errors.Wrap(err, "failed to unmarshal 'osd tree' response")
	}

	return output, nil
}

// OsdListNum returns the list of OSDs
func OsdListNum(context *clusterd.Context, clusterInfo *ClusterInfo) (OsdList, error) {
	var output OsdList

	args := []string{"osd", "ls"}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return output, errors.Wrap(err, "failed to get osd list")
	}

	err = json.Unmarshal(buf, &output)
	if err != nil {
		return output, errors.Wrap(err, "failed to unmarshal 'osd ls' response")
	}

	return output, nil
}

// OSDDeviceClass report device class for osd
type OSDDeviceClass struct {
	ID          int    `json:"osd"`
	DeviceClass string `json:"device_class"`
}

// OSDDeviceClasses returns the device classes for particular OsdIDs
func OSDDeviceClasses(context *clusterd.Context, clusterInfo *ClusterInfo, osdIds []string) ([]OSDDeviceClass, error) {
	var deviceClasses []OSDDeviceClass

	args := []string{"osd", "crush", "get-device-class"}
	args = append(args, osdIds...)
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return deviceClasses, errors.Wrap(err, "failed to get device-class info")
	}

	err = json.Unmarshal(buf, &deviceClasses)
	if err != nil {
		return deviceClasses, errors.Wrap(err, "failed to unmarshal 'osd crush get-device-class' response")
	}

	return deviceClasses, nil
}

// OSDOkToStopStats report detailed information about which OSDs are okay to stop
type OSDOkToStopStats struct {
	OkToStop          bool     `json:"ok_to_stop"`
	OSDs              []int    `json:"osds"`
	NumOkPGs          int      `json:"num_ok_pgs"`
	NumNotOkPGs       int      `json:"num_not_ok_pgs"`
	BadBecomeInactive []string `json:"bad_become_inactive"`
	OkBecomeDegraded  []string `json:"ok_become_degraded"`
}

// OSDOkToStop returns a list of OSDs that can be stopped that includes the OSD ID given.
// This is relevant, for example, when checking which OSDs can be updated.
// The number of OSDs returned is limited by the value set in maxReturned.
// maxReturned=0 is the same as maxReturned=1.
func OSDOkToStop(context *clusterd.Context, clusterInfo *ClusterInfo, osdID, maxReturned int) ([]int, error) {
	args := []string{"osd", "ok-to-stop", strconv.Itoa(osdID)}
	// NOTE: if the number of OSD IDs given in the CLI arg query is Q and --max=N is given, if
	// N < Q, Ceph treats the query as though max=Q instead, always returning at least Q OSDs.
	args = append(args, fmt.Sprintf("--max=%d", maxReturned))

	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		// is not ok to stop (or command error)
		return []int{}, errors.Wrapf(err, "OSD %d is not ok to stop", osdID)
	}

	var stats OSDOkToStopStats
	err = json.Unmarshal(buf, &stats)
	if err != nil {
		// Since the command succeeded we still know that at least the given OSD ID is ok to
		// stop, so we do not *have* to return an error. However, it is good to do it anyway so
		// that we can catch breaking changes to JSON output in CI testing. As a middle ground
		// here, return error but also return the given OSD ID in the output in case the calling
		// function wants to recover from this case.
		return []int{osdID}, errors.Wrapf(err, "failed to unmarshal 'osd ok-to-stop %d' response", osdID)
	}

	return stats.OSDs, nil
}

// SetPrimaryAffinity assigns primary-affinity (within range [0.0, 1.0]) to a specific OSD.
func SetPrimaryAffinity(context *clusterd.Context, clusterInfo *ClusterInfo, osdID int, affinity string) error {
	logger.Infof("setting osd.%d with primary-affinity %q", osdID, affinity)
	args := []string{"osd", "primary-affinity", fmt.Sprintf("osd.%d", osdID), affinity}
	_, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to set osd.%d with primary-affinity %q", osdID, affinity)
	}
	logger.Infof("successfully applied osd.%d primary-affinity %q", osdID, affinity)
	return nil
}

type OSDMetadata struct {
	Id       int    `json:"id"`
	HostName string `json:"hostname"`
}

// GetOSDMetadata returns the output of `ceph osd metadata`
func GetOSDMetadata(context *clusterd.Context, clusterInfo *ClusterInfo) (*[]OSDMetadata, error) {
	args := []string{"osd", "metadata"}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get osd metadata")
	}
	var osdMetadata []OSDMetadata
	if err := json.Unmarshal(buf, &osdMetadata); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal osd metadata response")
	}
	return &osdMetadata, nil
}
