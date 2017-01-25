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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"os"

	"github.com/coreos/pkg/capnslog"
	"github.com/go-ini/ini"
	"github.com/rook/rook/pkg/clusterd"
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

func TestGenerateConfigFile(t *testing.T) {
	// set up a temporary config directory that will be cleaned up after test
	configDir, err := ioutil.TempDir("", "TestGenerateConfigFile")
	if err != nil {
		t.Fatalf("failed to create temp config dir: %+v", err)
	}
	defer os.RemoveAll(configDir)

	// set up a config file override (in the config dir so it also gets cleaned up)
	configFileOverride := filepath.Join(configDir, "override.conf")
	overrideContents := `[global]
debug bluestore = 1234`
	err = ioutil.WriteFile(configFileOverride, []byte(overrideContents), 0644)
	if err != nil {
		t.Fatalf("failed to create config file override at '%s': %+v", configFileOverride, err)
	}

	// create mocked cluster context and info
	context := &clusterd.Context{
		ConfigDir:          configDir,
		ConfigFileOverride: configFileOverride,
	}
	clusterInfo := &ClusterInfo{
		FSID:          "myfsid",
		MonitorSecret: "monsecret",
		AdminSecret:   "adminsecret",
		Name:          "foo-cluster",
		Monitors: map[string]*CephMonitorConfig{
			"node0": &CephMonitorConfig{Name: "mon0", Endpoint: "10.0.0.1:6790"},
		},
	}

	// generate the config file to disk now
	configFilePath, err := GenerateConfigFile(context, clusterInfo, configDir, "myuser", filepath.Join(configDir, "mykeyring"), false, nil, nil)
	assert.Nil(t, err)
	assert.Equal(t, filepath.Join(configDir, "foo-cluster.config"), configFilePath)

	// verify some of the contents of written config file by loading it from disk
	actualConf, err := ini.Load(configFilePath)
	assert.Nil(t, err)
	verifyConfigValue(t, actualConf, "global", "fsid", clusterInfo.FSID)
	verifyConfigValue(t, actualConf, "mon.mon0", "mon addr", "10.0.0.1:6790")

	// verify the content of the config file override successfully overwrote the default generated config
	verifyConfigValue(t, actualConf, "global", "debug bluestore", "1234")
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
}

func TestLogLevel(t *testing.T) {
	defaultVal := 1
	level := getEnvLogLevel("NON_EXISTENT_VAR", defaultVal)
	assert.Equal(t, defaultVal, level)

	os.Setenv("MY_LOG_LEVEL", "10")
	level = getEnvLogLevel("MY_LOG_LEVEL", defaultVal)
	assert.Equal(t, 10, level)

	os.Setenv("MY_LOG_LEVEL", "abc")
	level = getEnvLogLevel("MY_LOG_LEVEL", defaultVal)
	assert.Equal(t, defaultVal, level)

	os.Setenv("MY_LOG_LEVEL", "-2")
	level = getEnvLogLevel("MY_LOG_LEVEL", defaultVal)
	assert.Equal(t, defaultVal, level)

	os.Setenv("MY_LOG_LEVEL", "-1")
	level = getEnvLogLevel("MY_LOG_LEVEL", defaultVal)
	assert.Equal(t, -1, level)

	os.Setenv("MY_LOG_LEVEL", "100")
	level = getEnvLogLevel("MY_LOG_LEVEL", defaultVal)
	assert.Equal(t, 100, level)

	os.Setenv("MY_LOG_LEVEL", "101")
	level = getEnvLogLevel("MY_LOG_LEVEL", defaultVal)
	assert.Equal(t, defaultVal, level)
}

func verifyConfigValue(t *testing.T, actualConf *ini.File, section, key, expectedVal string) {
	s, err := actualConf.GetSection(section)
	if !assert.Nil(t, err) {
		return
	}

	k := s.Key(key)
	if !assert.NotNil(t, k) {
		return
	}

	actualVal := k.Value()
	assert.Equal(t, expectedVal, actualVal)
}
