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
	"strings"
	"syscall"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
)

const (
	defaultStretchCrushRuleName = "default_stretch_cluster_rule"
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

// MonDump represents the response from a mon dump
type MonDump struct {
	StretchMode      bool           `json:"stretch_mode"`
	ElectionStrategy int            `json:"election_strategy"`
	FSID             string         `json:"fsid"`
	Mons             []MonDumpEntry `json:"mons"`
	Quorum           []int          `json:"quorum"`
}

type MonDumpEntry struct {
	Name          string `json:"name"`
	Rank          int    `json:"rank"`
	CrushLocation string `json:"crush_location"`
}

// GetMonQuorumStatus calls quorum_status mon_command
func GetMonQuorumStatus(context *clusterd.Context, clusterInfo *ClusterInfo) (MonStatusResponse, error) {
	args := []string{"quorum_status"}
	cmd := NewCephCommand(context, clusterInfo, args)
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

// GetMonDump calls mon dump command
func GetMonDump(context *clusterd.Context, clusterInfo *ClusterInfo) (MonDump, error) {
	args := []string{"mon", "dump"}
	cmd := NewCephCommand(context, clusterInfo, args)
	buf, err := cmd.Run()
	if err != nil {
		return MonDump{}, errors.Wrap(err, "mon dump failed")
	}

	var response MonDump
	err = json.Unmarshal(buf, &response)
	if err != nil {
		return MonDump{}, errors.Wrapf(err, "unmarshal failed. raw buffer response: %s", buf)
	}

	return response, nil
}

// EnableStretchElectionStrategy enables the mon connectivity algorithm for stretch clusters
func EnableStretchElectionStrategy(context *clusterd.Context, clusterInfo *ClusterInfo) error {
	args := []string{"mon", "set", "election_strategy", "connectivity"}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrap(err, "failed to enable stretch cluster election strategy")
	}
	logger.Infof("successfully enabled stretch cluster election strategy. %s", string(buf))
	return nil
}

// CreateDefaultStretchCrushRule creates the default CRUSH rule for the stretch cluster
func CreateDefaultStretchCrushRule(context *clusterd.Context, clusterInfo *ClusterInfo, clusterSpec *cephv1.ClusterSpec, failureDomain string) error {
	pool := cephv1.PoolSpec{
		FailureDomain: failureDomain,
		Replicated:    cephv1.ReplicatedSpec{SubFailureDomain: clusterSpec.Mon.StretchCluster.SubFailureDomain},
	}
	if err := createStretchCrushRule(context, clusterInfo, clusterSpec, defaultStretchCrushRuleName, pool); err != nil {
		return errors.Wrap(err, "failed to create default stretch crush rule")
	}
	logger.Info("successfully created the default stretch crush rule")
	return nil
}

// SetMonStretchTiebreaker sets the tiebreaker mon in the stretch cluster
func SetMonStretchTiebreaker(context *clusterd.Context, clusterInfo *ClusterInfo, monName, bucketType string) error {
	logger.Infof("enabling stretch mode with mon arbiter %q with crush rule %q in failure domain %q", monName, defaultStretchCrushRuleName, bucketType)
	args := []string{"mon", "enable_stretch_mode", monName, defaultStretchCrushRuleName, bucketType}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.EINVAL) {
			// TODO: Get a more distinctive error from ceph so we don't have to compare the error message
			if strings.Contains(string(buf), "stretch mode is already engaged") {
				logger.Infof("stretch mode is already enabled")
				return nil
			}
			return errors.Wrapf(err, "stretch mode failed to be enabled. %s", string(buf))
		}
		return errors.Wrap(err, "failed to set mon stretch zone")
	}
	logger.Debug(string(buf))
	logger.Infof("successfully set mon tiebreaker %q in failure domain %q", monName, bucketType)
	return nil
}
