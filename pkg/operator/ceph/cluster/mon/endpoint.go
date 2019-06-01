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

package mon

import (
	"encoding/json"
	"fmt"
	"strings"

	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
)

// FlattenMonEndpoints returns a comma-delimited string of all mons and endpoints in the form
// <mon-name>=<mon-endpoint>
func FlattenMonEndpoints(mons map[string]*cephconfig.MonInfo) string {
	endpoints := []string{}
	for _, m := range mons {
		endpoints = append(endpoints, fmt.Sprintf("%s=%s", m.Name, m.Endpoint))
	}
	return strings.Join(endpoints, ",")
}

// ParseMonEndpoints parses a flattened representation of mons and endpoints in the form
// <mon-name>=<mon-endpoint> and returns a list of Ceph mon configs.
func ParseMonEndpoints(input string) map[string]*cephconfig.MonInfo {
	logger.Infof("parsing mon endpoints: %s", input)
	mons := map[string]*cephconfig.MonInfo{}
	rawMons := strings.Split(input, ",")
	for _, rawMon := range rawMons {
		parts := strings.Split(rawMon, "=")
		if len(parts) != 2 {
			logger.Warningf("ignoring invalid monitor %s", rawMon)
			continue
		}
		mons[parts[0]] = &cephconfig.MonInfo{Name: parts[0], Endpoint: parts[1]}
	}
	return mons
}

// FormatCsiClusterConfig returns a json-formatted string containing
// the cluster-to-mon mapping required to configure ceph csi.
func FormatCsiClusterConfig(
	clusterKey string, mons map[string]*cephconfig.MonInfo) (string, error) {

	type csiClusterConfig struct {
		ClusterID string   `json:"clusterID"`
		Monitors  []string `json:"monitors"`
	}
	cc := make([]csiClusterConfig, 1)
	cc[0].ClusterID = clusterKey
	cc[0].Monitors = []string{}
	for _, m := range mons {
		cc[0].Monitors = append(cc[0].Monitors, m.Endpoint)
	}

	ccJson, err := json.Marshal(cc)
	if err != nil {
		return "", fmt.Errorf("failed to marshal csi cluster config. %+v", err)
	}
	return string(ccJson), nil
}
