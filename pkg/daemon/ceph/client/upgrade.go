/*
Copyright 2019 The Rook Authors. All rights reserved.

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
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
)

// CephDaemonsVersions is a structure that can be used to parsed the output of the 'ceph versions' command
type CephDaemonsVersions struct {
	Mon     map[string]int `json:"mon,omitempty"`
	Osd     map[string]int `json:"osd,omitempty"`
	Mgr     map[string]int `json:"mgr,omitempty"`
	Mds     map[string]int `json:"mds,omitempty"`
	Overall map[string]int `json:"overall,omitempty"`
}

func getCephMonVersionString(context *clusterd.Context) (string, error) {
	output, err := context.Executor.ExecuteCommandWithOutput(false, "", "ceph", "version")
	if err != nil {
		return "", fmt.Errorf("failed to run ceph version: %+v", err)
	}
	logger.Debug(output)

	return output, nil
}

func getCephVersionsString(context *clusterd.Context) (string, error) {
	output, err := context.Executor.ExecuteCommandWithOutput(false, "", "ceph", "versions")
	if err != nil {
		return "", fmt.Errorf("failed to run ceph versions: %+v", err)
	}
	logger.Debug(output)

	return output, nil
}

// GetCephMonVersion reports the Ceph version of all the monitors, or at least a majority with quorum
func GetCephMonVersion(context *clusterd.Context) (*cephver.CephVersion, error) {
	output, err := getCephMonVersionString(context)
	if err != nil {
		return nil, fmt.Errorf("failed to run ceph version: %+v", err)
	}
	logger.Debug(output)

	v, err := cephver.ExtractCephVersion(output)
	if err != nil {
		return nil, fmt.Errorf("failed to extract ceph version. %+v", err)
	}

	return v, nil
}

// GetCephVersions reports the Ceph version of each daemon in the cluster
func GetCephVersions(context *clusterd.Context) (*CephDaemonsVersions, error) {
	output, err := getCephVersionsString(context)
	if err != nil {
		return nil, fmt.Errorf("failed to run ceph versions: %+v", err)
	}
	logger.Debug(output)

	var cephVersionsResult CephDaemonsVersions
	err = json.Unmarshal([]byte(output), &cephVersionsResult)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve ceph versions results. %+v", err)
	}

	return &cephVersionsResult, nil
}

// EnableMessenger2 enable the messenger 2 protocol on Nautilus clusters
func EnableMessenger2(context *clusterd.Context) error {
	_, err := context.Executor.ExecuteCommandWithOutput(false, "", "ceph", "mon", "enable-msgr2")
	if err != nil {
		return fmt.Errorf("failed to enable msgr2 protocol: %+v", err)
	}
	logger.Infof("successfully enabled msgr2 protocol")

	return nil
}

// EnableNautilusOSD disallows pre-Nautilus OSDs and enables all new Nautilus-only functionality
func EnableNautilusOSD(context *clusterd.Context) error {
	_, err := context.Executor.ExecuteCommandWithOutput(false, "", "ceph", "osd", "require-osd-release", "nautilus")
	if err != nil {
		return fmt.Errorf("failed to disallow pre-nautilus osds and enable all new nautilus-only functionality: %+v", err)
	}
	logger.Infof("successfully disallowed pre-nautilus osds and enabled all new nautilus-only functionality")

	return nil
}
