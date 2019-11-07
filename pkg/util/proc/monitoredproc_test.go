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
	"time"

	"github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestRestartDelay(t *testing.T) {
	var zeroTime time.Time

	// test cases for when last retry check time is not available
	assert.Equal(t, float64(0), calcRetryDelay(0, 0, zeroTime, zeroTime, zeroTime))
	assert.Equal(t, float64(0), calcRetryDelay(0, 2, zeroTime, zeroTime, zeroTime))
	assert.Equal(t, float64(1), calcRetryDelay(2, 0, zeroTime, zeroTime, zeroTime))
	assert.Equal(t, float64(2), calcRetryDelay(2, 1, zeroTime, zeroTime, zeroTime))
	assert.Equal(t, float64(4), calcRetryDelay(2, 2, zeroTime, zeroTime, zeroTime))
	assert.Equal(t, float64(30), calcRetryDelay(2, 5, zeroTime, zeroTime, zeroTime))
	assert.Equal(t, float64(30), calcRetryDelay(2, 100, zeroTime, zeroTime, zeroTime))

	// test cases for when last retry check is available and retry count is 0 and the process hasn't been
	// running long at all. the time delay should be 2^(number of secs since last retry), with a max limit of 30.
	now := time.Now()
	lastStartTime := now
	lastRetryCheck := now
	assert.Equal(t, float64(1), calcRetryDelay(2, 0, now, lastStartTime, lastRetryCheck))

	lastRetryCheck = now.Add(-1 * time.Second)
	assert.Equal(t, float64(2), calcRetryDelay(2, 0, now, lastStartTime, lastRetryCheck))

	lastRetryCheck = now.Add(-2 * time.Second)
	assert.Equal(t, float64(4), calcRetryDelay(2, 0, now, lastStartTime, lastRetryCheck))

	lastRetryCheck = now.Add(-4 * time.Second)
	assert.Equal(t, float64(16), calcRetryDelay(2, 0, now, lastStartTime, lastRetryCheck))

	lastRetryCheck = now.Add(-5 * time.Second)
	assert.Equal(t, float64(30), calcRetryDelay(2, 0, now, lastStartTime, lastRetryCheck))

	lastRetryCheck = now.Add(-999999999 * time.Second)
	assert.Equal(t, float64(30), calcRetryDelay(2, 0, now, lastStartTime, lastRetryCheck))

	// test case for when the process has been running for quite awhile.  we should not delay for very long since we
	// are clearly not in a rapid retry loop.
	lastStartTime = now.Add(-100 * time.Second)
	assert.Equal(t, float64(1), calcRetryDelay(2, 0, now, lastStartTime, lastRetryCheck))

	// test cases for when both retry count and last retry check are available, retry count should be favored
	lastRetryCheck = now.Add(-4 * time.Second)
	assert.Equal(t, float64(2), calcRetryDelay(2, 1, now, lastStartTime, lastRetryCheck))
}

func TestMonitoredRestart(t *testing.T) {
	executor := &test.MockExecutor{}
	procMgr := New(executor)
	cmd := &exec.Cmd{Args: []string{"/my/path", "1", "2", "3"}}
	proc := newMonitoredProc(procMgr, cmd)
	proc.retrySecondsExponentBase = 0.0

	commands := 0
	executor.MockStartExecuteCommand = func(debug bool, name string, command string, args ...string) (*exec.Cmd, error) {
		commands++
		cmd := &exec.Cmd{Args: append([]string{command}, args...)}
		switch {
		case commands == 1:
			return cmd, errors.New("test failure")
		case commands == 2:
			assert.Equal(t, 1, proc.retries)
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
		}
		iter++
	}

	proc.Monitor("testproc")
	assert.False(t, proc.monitor)
	assert.Equal(t, proc.retries, 0)
	assert.Equal(t, proc.totalRetries, 2)
}
