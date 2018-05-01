/*
Copyright 2017 The Rook Authors. All rights reserved.

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
package osd

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/stretchr/testify/assert"
)

func TestOSDStatus(t *testing.T) {
	// Overwriting default grace period to speed up testing
	osdGracePeriod = 1 * time.Microsecond

	configDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temp config dir: %+v", err)
	}
	defer os.RemoveAll(configDir)

	storeConfig := &config.StoreConfig{StoreType: "bluestore"}
	// Mocking agent and executor, re-using a function from agent tests
	agent, executor, _ := createTestAgent(t, "sdx", configDir, "node1271", storeConfig)

	var execCount = 0
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
		logger.Infof("ExecuteCommandWithOutputFile: %s %v", command, args)
		execCount++
		if args[1] == "dump" {
			// Mock executor for OSD Dump command, returning an osd in Down state
			return `{"OSDs": [{"OSD": 1, "Up": 0, "In": 1}]}`, nil
		} else if args[1] == "create" {
			// Mock executor for osd creation
			return `{"osdid": 1.0}`, nil
		}
		return "", nil
	}

	executor.MockExecuteCommandWithOutput = func(debug bool, actionName string, command string, args ...string) (string, error) {
		logger.Infof("ExecuteCommandWithOutput: %s %v", command, args)
		// Mocking execution of lsbk of existing devices
		if strings.HasPrefix(actionName, "lsblk /dev/disk/by-partuuid") {
			return `SIZE="111" TYPE="part"`, nil
		}
		return "", nil
	}

	// Setting up objects needed to create OSD
	context := &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
	}
	devices := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{
		"sdx": {Data: -1},
	}}
	// Creating an OSD for which Down status will be returned
	_, err = agent.configureDevices(context, devices)
	assert.Nil(t, err)

	// Initializing an OSD monitoring
	osdMon := NewMonitor(context, agent)
	// Run OSD monitoring routine
	err = osdMon.osdStatus()
	assert.Nil(t, err)
	// After creating an OSD, the dump has to be the 4th mocked cmd
	assert.Equal(t, 4, execCount)
	// Only one OSD proc was mocked
	assert.Equal(t, 1, len(agent.osdProc))
	// OSD monitoring should start tracking an osd with Down status
	assert.Equal(t, 1, len(osdMon.lastStatus))

	// Run OSD monitoring routine again to trigger an action on tracked proc
	err = osdMon.osdStatus()
	assert.Nil(t, err)
	assert.Equal(t, 5, execCount)
	// OSD monitor should stop tracking that process once the action is triggered
	assert.Equal(t, 0, len(osdMon.lastStatus))
}
