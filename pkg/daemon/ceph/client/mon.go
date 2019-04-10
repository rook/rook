/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// represents the response from a mon_status mon_command (subset of all available fields, only
// marshal ones we care about)
type MonStatusResponse struct {
	Quorum []int `json:"quorum"`
	MonMap struct {
		Mons []MonMapEntry `json:"mons"`
	} `json:"monmap"`
}

// request to simplify deserialization of a test request
type MonStatusRequest struct {
	Prefix string   `json:"prefix"`
	Format string   `json:"format"`
	ID     int      `json:"id"`
	Weight float32  `json:"weight"`
	Pool   string   `json:"pool"`
	Var    string   `json:"var"`
	Args   []string `json:"args"`
}

// represents an entry in the monitor map
type MonMapEntry struct {
	Name    string `json:"name"`
	Rank    int    `json:"rank"`
	Address string `json:"addr"`
}

// GetMonStatus calls mon_status mon_command
func GetMonStatus(context *clusterd.Context, clusterName string, debug bool) (MonStatusResponse, error) {
	args := []string{"mon_status"}
	buf, err := executeCephCommandWithOutputFile(context, clusterName, debug, args)
	if err != nil {
		return MonStatusResponse{}, fmt.Errorf("mon status failed. %+v", err)
	}

	var resp MonStatusResponse
	err = json.Unmarshal(buf, &resp)
	if err != nil {
		return MonStatusResponse{}, fmt.Errorf("unmarshal failed: %+v.  raw buffer response: %s", err, buf)
	}

	logger.Debugf("MON STATUS: %+v", resp)
	return resp, nil
}

type MonTimeStatus struct {
	Skew   map[string]MonTimeSkewStatus `json:"time_skew_status"`
	Checks struct {
		Epoch       int    `json:"epoch"`
		Round       int    `json:"round"`
		RoundStatus string `json:"round_status"`
	} `json:"timechecks"`
}

type MonTimeSkewStatus struct {
	Skew    json.Number `json:"skew"`
	Latency json.Number `json:"latency"`
	Health  string      `json:"health"`
}

func GetMonTimeStatus(context *clusterd.Context, clusterName string) (*MonTimeStatus, error) {
	args := []string{"time-sync-status"}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return nil, fmt.Errorf("failed to get time sync status: %+v", err)
	}

	var timeStatus MonTimeStatus
	if err := json.Unmarshal(buf, &timeStatus); err != nil {
		return nil, fmt.Errorf("failed to unmarshal time sync status response: %+v", err)
	}

	return &timeStatus, nil
}
