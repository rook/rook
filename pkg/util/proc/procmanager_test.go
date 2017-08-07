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
	"os"
	"os/exec"
	"testing"

	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestCheckProcessExists(t *testing.T) {
	executor := &exectest.MockExecutor{}
	p := &ProcManager{executor: executor}

	// no managed process to find
	shouldStart, err := p.checkProcessExists(os.Args[0], "bar=2", ReuseExisting)
	assert.Nil(t, err)
	assert.True(t, shouldStart)

	// create a couple managed processes
	cmd1 := exec.Command("mycmd1", []string{os.Args[0], "foo=1", "bar=2"}...)
	cmd2 := exec.Command("mycmd2", []string{os.Args[0], "foo=3", "bar=4"}...)
	monitored1 := &MonitoredProc{parent: p, cmd: cmd1, waitForExit: func() {}}
	monitored2 := &MonitoredProc{parent: p, cmd: cmd2, waitForExit: func() {}}
	p.procs = []*MonitoredProc{monitored1, monitored2}

	// find the managed process
	shouldStart, err = p.checkProcessExists(os.Args[0], "bar=2", ReuseExisting)
	assert.Nil(t, err)
	assert.False(t, shouldStart)

	// can't find another process
	index, proc := p.findMonitoredProcByPID(0)
	assert.Equal(t, -1, index)
	assert.Nil(t, proc)

	// purge the managed process
	p.purgeManagedProc(0, monitored1)
	assert.Equal(t, 1, len(p.procs))
	assert.Equal(t, "mycmd2", p.procs[0].cmd.Args[0])

	p.Shutdown()
	assert.Equal(t, 0, len(p.procs))
}

func TestRunProcesses(t *testing.T) {
	executor := &exectest.MockExecutor{}
	p := &ProcManager{executor: executor}

	proc, err := p.Start("mylog", "mydaemon", "mysearch", ReuseExisting, "arg1", "arg2")
	assert.Nil(t, err)
	assert.Equal(t, "mydaemon", proc.cmd.Args[0])
	assert.Equal(t, "arg1", proc.cmd.Args[1])
	assert.Equal(t, "arg2", proc.cmd.Args[2])

	run := false
	executor.MockExecuteCommand = func(debug bool, name string, command string, args ...string) error {
		assert.Equal(t, "arg1", args[0])
		assert.Equal(t, "arg2", args[1])
		run = true
		return nil
	}

	err = p.Run("mylog", "mytool", "arg1", "arg2")
	assert.Nil(t, err)
	assert.True(t, run)
}
