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

package config

import (
	"fmt"
	"net"
	"strings"

	"github.com/coreos/pkg/capnslog"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
)

// ClusterInfo is a collection of information about a particular Ceph cluster. Rook uses information
// about the cluster to configure daemons to connect to the desired cluster.
type ClusterInfo struct {
	FSID          string
	MonitorSecret string
	AdminSecret   string
	Name          string
	Monitors      map[string]*MonInfo
	CephVersion   cephver.CephVersion
}

// MonInfo is a collection of information about a Ceph mon.
type MonInfo struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
}

// IsInitialized returns true if the critical information in the ClusterInfo struct has been filled
// in. This method exists less out of necessity than the desire to be explicit about the lifecycle
// of the ClusterInfo struct during startup, specifically that it is expected to exist after the
// Rook operator has started up or connected to the first components of the Ceph cluster.
func (c *ClusterInfo) IsInitialized() bool {
	if c == nil || c.FSID == "" || c.MonitorSecret == "" || c.AdminSecret == "" {
		logger.Errorf("clusterInfo: %+v", c)
		return false
	}
	return true
}

// NewMonInfo returns a new Ceph mon info struct from the given inputs.
func NewMonInfo(name, ip string, port int32) *MonInfo {
	return &MonInfo{Name: name, Endpoint: net.JoinHostPort(ip, fmt.Sprintf("%d", port))}
}

// Log writes the cluster info struct to the logger
func (c *ClusterInfo) Log(logger *capnslog.PackageLogger) {
	mons := []string{}
	for _, m := range c.Monitors {
		mons = append(mons, fmt.Sprintf("{Name: %s, Endpoint: %s}", m.Name, m.Endpoint))
	}
	monsec := ""
	if c.MonitorSecret != "" {
		monsec = "<hidden>"
	}
	admsec := ""
	if c.AdminSecret != "" {
		admsec = "<hidden>"
	}
	s := fmt.Sprintf(
		"ClusterInfo: {FSID: %s, MonitorSecret: %s, AdminSecret: %s, Name: %s, Monitors: %s}",
		c.FSID, monsec, admsec, c.Name, strings.Join(mons, " "))
	logger.Info(s)
}
