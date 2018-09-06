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
)

// ClusterInfo is a collection of information about a particular Ceph cluster. Rook uses information
// about the cluster to configure daemons to connect to the desired cluster.
type ClusterInfo struct {
	FSID          string
	MonitorSecret string
	AdminSecret   string
	Name          string
	Monitors      map[string]*MonInfo
}

// MonInfo is a collection of information about a Ceph mon.
type MonInfo struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
}

// NewMonInfo returns a new Ceph mon info struct from the given inputs.
func NewMonInfo(name, ip string, port int32) *MonInfo {
	return &MonInfo{Name: name, Endpoint: net.JoinHostPort(ip, fmt.Sprintf("%d", port))}
}
