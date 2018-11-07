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
	"testing"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"

	"github.com/stretchr/testify/assert"
)

func TestOSDStatus(t *testing.T) {
	// Overwriting default grace period to speed up testing
	osdGracePeriod = 1 * time.Microsecond

	cluster := "fake"

	var execCount = 0
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return "{\"key\":\"mysecurekey\", \"osdid\":3.0}", nil
		},
	}
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
		logger.Infof("ExecuteCommandWithOutputFile: %s %v", command, args)
		execCount++
		if args[1] == "dump" {
			// Mock executor for OSD Dump command, returning an osd in Down state
			return `{"OSDs": [{"OSD": 0, "Up": 0, "In": 1}]}`, nil
		}
		return "", nil
	}

	executor.MockExecuteCommandWithOutput = func(debug bool, actionName string, command string, args ...string) (string, error) {
		logger.Infof("ExecuteCommandWithOutput: %s %v", command, args)
		return "", nil
	}

	// Setting up objects needed to create OSD
	context := &clusterd.Context{
		Executor: executor,
	}
	// Initializing an OSD monitoring
	osdMon := NewMonitor(context, cluster)
	// Run OSD monitoring routine
	err := osdMon.osdStatus()
	assert.Nil(t, err)
	// After creating an OSD, the dump has to be the 1 mocked cmd
	assert.Equal(t, 1, execCount)
	// Only one OSD proc was mocked
	//assert.Equal(t, 1, len(agent.osdProc))
	// FIX: OSD monitoring should start tracking an osd with Down status
	//assert.Equal(t, 1, len(osdMon.lastStatus))

	// Run OSD monitoring routine again to trigger an action on tracked proc
	err = osdMon.osdStatus()
	assert.Nil(t, err)
	assert.Equal(t, 2, execCount)
	// OSD monitor should stop tracking that process once the action is triggered
	assert.Equal(t, 1, len(osdMon.lastStatus))
}

func TestMonitorStart(t *testing.T) {
	stopCh := make(chan struct{})
	osdMon := NewMonitor(&clusterd.Context{}, "cluster")
	logger.Infof("starting osd monitor")
	go osdMon.Start(stopCh)
	close(stopCh)
}
