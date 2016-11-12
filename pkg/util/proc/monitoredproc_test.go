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
package proc

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestRestartDelay(t *testing.T) {
	assert.Equal(t, 0, calcRetryDelay(0, 0))
	assert.Equal(t, 0, calcRetryDelay(0, 2))
	assert.Equal(t, 1, calcRetryDelay(2, 0))
	assert.Equal(t, 2, calcRetryDelay(2, 1))
	assert.Equal(t, 4, calcRetryDelay(2, 2))
	assert.Equal(t, 30, calcRetryDelay(2, 5))
	assert.Equal(t, 30, calcRetryDelay(2, 100))
}

func TestMonitoredRestart(t *testing.T) {
	executor := &test.MockExecutor{}
	procMgr := New(executor)
	cmd := &exec.Cmd{Args: []string{"/my/path", "1", "2", "3"}}
	proc := newMonitoredProc(procMgr, cmd)
	proc.retrySecondsExponentBase = 0.0

	commands := 0
	executor.MockStartExecuteCommand = func(name string, command string, args ...string) (*exec.Cmd, error) {
		commands++
		cmd := &exec.Cmd{Args: append([]string{command}, args...)}
		switch {
		case commands == 1:
			return cmd, errors.New("test failure")
		case commands == 2:
			assert.Equal(t, 1, proc.retries)
			break
		}
		return cmd, nil
	}

	iter := 0
	proc.waitForExit = func() {
		logger.Debugf("waited for process")
		assert.True(t, proc.monitor)
		switch {
		case iter == 0:
		case iter == 1:
			assert.True(t, proc.monitor)
			logger.Debugf("stop monitoring")
			proc.monitor = false
			break
		}
		iter++
	}

	proc.Monitor()
	assert.False(t, proc.monitor)
	assert.Equal(t, proc.retries, 0)
	assert.Equal(t, proc.totalRetries, 2)
}
