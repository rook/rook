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

// Package test provides common resources useful for testing many operators. This includes functions
// for creating fake/mock resources and functions for testing that resources match what is expected.
package test

import (
	"fmt"

	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
)

// CreateConfigDir creates a test cluster
func CreateConfigDir(monCount int) *cephconfig.ClusterInfo {
	c := &cephconfig.ClusterInfo{
		FSID:          "12345",
		Name:          "default",
		MonitorSecret: "monsecret",
		AdminSecret:   "adminsecret",
		Monitors:      map[string]*cephconfig.MonInfo{},
	}
	mons := []string{"a", "b", "c", "d", "e"}
	for i := 0; i < monCount; i++ {
		id := mons[i]
		c.Monitors[id] = &cephconfig.MonInfo{
			Name:     id,
			Endpoint: fmt.Sprintf("1.2.3.%d:6789", i+1),
		}
	}
	return c
}
