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
	"path"
	"time"

	"github.com/rook/rook/pkg/clusterd"
)

const (
	AdminUsername     = "client.admin"
	CephTool          = "ceph"
	RBDTool           = "rbd"
	CrushTool         = "crushtool"
	cmdExecuteTimeout = 1 * time.Minute
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

func AppendAdminConnectionArgs(args []string, configDir, clusterName string) []string {
	confFile := fmt.Sprintf("%s.config", clusterName)
	keyringFile := fmt.Sprintf("%s.keyring", AdminUsername)
	configArgs := []string{
		fmt.Sprintf("--cluster=%s", clusterName),
		fmt.Sprintf("--conf=%s", path.Join(configDir, clusterName, confFile)),
		fmt.Sprintf("--keyring=%s", path.Join(configDir, clusterName, keyringFile)),
	}
	return append(args, configArgs...)
}

func ExecuteCephCommand(context *clusterd.Context, clusterName string, args []string) ([]byte, error) {
	return executeCephCommandWithOutputFile(context, clusterName, false, args)
}

func ExecuteCephCommandPlain(context *clusterd.Context, clusterName string, args []string) ([]byte, error) {
	args = AppendAdminConnectionArgs(args, context.ConfigDir, clusterName)
	args = append(args, "--format", "plain")
	return executeCommandWithOutputFile(context, false, CephTool, args)
}

func ExecuteCephCommandPlainNoOutputFile(context *clusterd.Context, clusterName string, args []string) ([]byte, error) {
	args = AppendAdminConnectionArgs(args, context.ConfigDir, clusterName)
	args = append(args, "--format", "plain")
	return executeCommand(context, CephTool, args)
}

func executeCephCommandWithOutputFile(context *clusterd.Context, clusterName string, debug bool, args []string) ([]byte, error) {
	args = AppendAdminConnectionArgs(args, context.ConfigDir, clusterName)
	args = append(args, "--format", "json")
	return executeCommandWithOutputFile(context, debug, CephTool, args)
}

func ExecuteRBDCommand(context *clusterd.Context, clusterName string, args []string) ([]byte, error) {
	args = AppendAdminConnectionArgs(args, context.ConfigDir, clusterName)
	args = append(args, "--format", "json")
	return executeCommand(context, RBDTool, args)
}

func ExecuteRBDCommandNoFormat(context *clusterd.Context, clusterName string, args []string) ([]byte, error) {
	args = AppendAdminConnectionArgs(args, context.ConfigDir, clusterName)
	return executeCommand(context, RBDTool, args)
}

func ExecuteRBDCommandWithTimeout(context *clusterd.Context, clusterName string, args []string) (string, error) {
	output, err := context.Executor.ExecuteCommandWithTimeout(false, cmdExecuteTimeout, "", RBDTool, args...)
	return output, err
}

func executeCommand(context *clusterd.Context, tool string, args []string) ([]byte, error) {
	output, err := context.Executor.ExecuteCommandWithOutput(false, "", tool, args...)
	return []byte(output), err
}

func executeCommandWithOutputFile(context *clusterd.Context, debug bool, tool string, args []string) ([]byte, error) {
	output, err := context.Executor.ExecuteCommandWithOutputFile(debug, "", tool, "--out-file", args...)
	return []byte(output), err
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

// MonStats is a subset of fields on the response from the mon command "status".  These fields
// are focused on monitor stats.
type MonStats struct {
	Health struct {
		Status string                  `json:"status"`
		Checks map[string]CheckMessage `json:"checks"`
	} `json:"health"`
	Quorum []int `json:"quorum"`
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

func GetMonStats(context *clusterd.Context, clusterName string) (*MonStats, error) {
	// note this is another call to the mon command "status", but we'll be marshalling it into
	// a type with a different subset of fields, scoped to monitor stats
	args := []string{"status"}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %+v", err)
	}

	var monStats MonStats
	if err := json.Unmarshal(buf, &monStats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal status response: %+v", err)
	}

	return &monStats, nil
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
