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
	"strings"
	"testing"

	"github.com/coreos/pkg/capnslog"
	"github.com/stretchr/testify/assert"
)

func TestCreateDefaultCephConfig(t *testing.T) {
	clusterInfo := &ClusterInfo{
		FSID:          "id",
		MonitorSecret: "monsecret",
		AdminSecret:   "adminsecret",
		Name:          "foo-cluster",
		Monitors: map[string]*CephMonitorConfig{
			"node0": &CephMonitorConfig{Name: "mon0", Endpoint: "10.0.0.1:6790"},
			"node1": &CephMonitorConfig{Name: "mon1", Endpoint: "10.0.0.2:6790"},
		},
	}

	monMembers := "mon0 mon1"

	cephConfig := CreateDefaultCephConfig(clusterInfo, "/var/lib/rook1", capnslog.INFO, false)
	verifyConfig(t, cephConfig, monMembers, "", "filestore", 0)

	cephConfig = CreateDefaultCephConfig(clusterInfo, "/var/lib/rook1", capnslog.INFO, true)
	verifyConfig(t, cephConfig, monMembers, "bluestore rocksdb", "bluestore", 0)

	cephConfig = CreateDefaultCephConfig(clusterInfo, "/var/lib/rook1", capnslog.DEBUG, false)
	verifyConfig(t, cephConfig, monMembers, "", "filestore", 10)

	cephConfig = CreateDefaultCephConfig(clusterInfo, "/var/lib/rook1", capnslog.DEBUG, true)
	verifyConfig(t, cephConfig, monMembers, "bluestore rocksdb", "bluestore", 10)
}

func verifyConfig(t *testing.T, cephConfig *cephConfig, expectedMonMembers, experimental, objectStore string, loggingLevel int) {

	for _, expectedMon := range strings.Split(expectedMonMembers, " ") {
		contained := false
		for _, actualMon := range strings.Split(cephConfig.MonMembers, " ") {
			if expectedMon == actualMon {
				contained = true
				break
			}
		}

		assert.True(t, contained, "expectedMons: %+v, actualMons: %+v", expectedMonMembers, cephConfig.MonMembers)
	}

	assert.Equal(t, experimental, cephConfig.EnableExperimental)
	assert.Equal(t, objectStore, cephConfig.OsdObjectStore)
	assert.Equal(t, loggingLevel, cephConfig.DebugLogDefaultLevel)
	assert.Equal(t, loggingLevel, cephConfig.DebugLogMonLevel)
	assert.Equal(t, loggingLevel, cephConfig.DebugLogRadosLevel)
	assert.Equal(t, loggingLevel, cephConfig.DebugLogBluestoreLevel)
}
