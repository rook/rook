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

// Package test for the operator tests
package test

import (
	"fmt"

	"github.com/rook/rook/pkg/daemon/ceph/mon"
)

// CreateConfigDir creates a test cluster
func CreateConfigDir(mons int) *mon.ClusterInfo {
	c := &mon.ClusterInfo{
		FSID:          "12345",
		Name:          "default",
		MonitorSecret: "monsecret",
		AdminSecret:   "adminsecret",
		Monitors:      map[string]*mon.CephMonitorConfig{},
	}
	for i := 1; i <= mons; i++ {
		id := fmt.Sprintf("rook-ceph-mon%d", i)
		c.Monitors[id] = &mon.CephMonitorConfig{
			Name:     id,
			Endpoint: fmt.Sprintf("1.2.3.%d:6790", i),
		}
	}
	return c
}
