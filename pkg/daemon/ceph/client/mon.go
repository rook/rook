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

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
)

// MonStatusResponse represents the response from a quorum_status mon_command (subset of all available fields, only
// marshal ones we care about)
type MonStatusResponse struct {
	Quorum []int `json:"quorum"`
	MonMap struct {
		Mons []MonMapEntry `json:"mons"`
	} `json:"monmap"`
}

// MonMapEntry represents an entry in the monitor map
type MonMapEntry struct {
	Name        string `json:"name"`
	Rank        int    `json:"rank"`
	Address     string `json:"addr"`
	PublicAddr  string `json:"public_addr"`
	PublicAddrs struct {
		Addrvec []AddrvecEntry `json:"addrvec"`
	} `json:"public_addrs"`
}

// AddrvecEntry represents an entry type for a given messenger version
type AddrvecEntry struct {
	Type  string `json:"type"`
	Addr  string `json:"addr"`
	Nonce int    `json:"nonce"`
}

// GetMonQuorumStatus calls quorum_status mon_command
func GetMonQuorumStatus(context *clusterd.Context, clusterName string) (MonStatusResponse, error) {
	args := []string{"quorum_status"}
	cmd := NewCephCommand(context, clusterName, args)
	buf, err := cmd.Run()
	if err != nil {
		return MonStatusResponse{}, errors.Wrap(err, "mon quorum status failed")
	}

	var resp MonStatusResponse
	err = json.Unmarshal(buf, &resp)
	if err != nil {
		return MonStatusResponse{}, errors.Wrapf(err, "unmarshal failed. raw buffer response: %s", buf)
	}

	return resp, nil
}

// GetMonQuorumStatusHealth calls quorum_status mon_command with a special client key
func GetMonQuorumStatusHealth(context *clusterd.Context, clusterName, userName string) (MonStatusResponse, error) {
	args := []string{"quorum_status", "--format", "json"}
	command, args := FinalizeCephCommandArgs("ceph", args, context.ConfigDir, clusterName, userName)

	buf, err := context.Executor.ExecuteCommandWithOutput(command, args...)
	if err != nil {
		return MonStatusResponse{}, errors.Wrap(err, "mon quorum status failed")
	}

	var resp MonStatusResponse
	err = json.Unmarshal([]byte(buf), &resp)
	if err != nil {
		return MonStatusResponse{}, errors.Wrapf(err, "unmarshal failed. raw buffer response: %s", buf)
	}

	return resp, nil
}
