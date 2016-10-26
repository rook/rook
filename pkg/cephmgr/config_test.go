package cephmgr

import (
	"strings"
	"testing"

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

	cephConfig := createDefaultCephConfig(clusterInfo, "/var/lib/rook1", false, false)
	verifyConfig(t, cephConfig, monMembers, "", "filestore", 0)

	cephConfig = createDefaultCephConfig(clusterInfo, "/var/lib/rook1", false, true)
	verifyConfig(t, cephConfig, monMembers, "bluestore rocksdb", "bluestore", 0)

	cephConfig = createDefaultCephConfig(clusterInfo, "/var/lib/rook1", true, false)
	verifyConfig(t, cephConfig, monMembers, "", "filestore", 20)

	cephConfig = createDefaultCephConfig(clusterInfo, "/var/lib/rook1", true, true)
	verifyConfig(t, cephConfig, monMembers, "bluestore rocksdb", "bluestore", 20)
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
