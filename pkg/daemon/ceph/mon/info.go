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
	"fmt"
	"strings"
	"sync"
)

type ClusterInfo struct {
	FSID             string
	MonitorSecret    string
	AdminSecret      string
	Name             string
	Monitors         map[string]*CephMonitorConfig
	MonitorAddresses map[string]*CephMonitorConfig
	MonMutex         sync.Mutex
}

// MonEndpoints returns the Monitors as a comma separated string
func (c *ClusterInfo) MonEndpoints() string {
	var endpoints []string
	for _, mon := range c.Monitors {
		endpoints = append(endpoints, fmt.Sprintf("%s-%s", mon.Name, mon.Endpoint))
	}
	return strings.Join(endpoints, ",")
}

// MonAddresses returns the MonitorAddresses as a comma separated string
func (c *ClusterInfo) MonAddresses() string {
	var endpoints []string
	for _, mon := range c.MonitorAddresses {
		endpoints = append(endpoints, fmt.Sprintf("%s-%s", mon.Name, mon.Endpoint))
	}
	return strings.Join(endpoints, ",")
}

// RemovePortFromEndpoint removes the port from a given endpoint
func (c *ClusterInfo) RemovePortFromEndpoint(endpoint string) string {
	split := strings.Split(endpoint, ":")
	if len(split) <= 1 {
		return endpoint
	}
	return split[len(split)-1]
}
