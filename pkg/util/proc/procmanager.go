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
	"fmt"
	"os"
	"sync"
	"syscall"

	"github.com/rook/rook/pkg/util/exec"
)

type ProcStartPolicy int

const (
	StartAction = "start"
	RunAction   = "run"
	StopAction  = "stop"

	RestartExisting ProcStartPolicy = iota
	ReuseExisting
)

type ProcManager struct {
	sync.RWMutex
	procs    []*MonitoredProc
	executor exec.Executor
}

// Create a new proc manager
func New(executor exec.Executor) *ProcManager {
	return &ProcManager{executor: executor}
}

// Start a child process and wait for its completion
func (p *ProcManager) RunWithOutput(logName, tool string, args ...string) (string, error) {

	logger.Infof("Running process %s with args: %v", tool, args)
	output, err := p.executor.ExecuteCommandWithOutput(logName, os.Args[0], createToolArgs(tool, args...)...)
	if err != nil {
		return "", fmt.Errorf("failed to run %s: %+v", tool, err)
	}

	return output, nil
}

// Start a child process and wait for its completion
func (p *ProcManager) Run(logName, tool string, args ...string) error {

	logger.Infof("Running process %s with args: %v", tool, args)
	err := p.executor.ExecuteCommand(logName, os.Args[0], createToolArgs(tool, args...)...)
	if err != nil {
		return fmt.Errorf("failed to run %s: %+v", tool, err)
	}

	return nil
}

// Start the given daemon and provided arguments.  Handling of any matching existing process will be in accordance
// with the given ProcStartPolicy.  The search pattern will be used to search through the cmdline args of existing
// processes to find any matching existing process.  Therefore, it should be a regex pattern that can uniquely
// identify the process (e.g., --id=1)
func (p *ProcManager) Start(logName, daemon, procSearchPattern string, policy ProcStartPolicy, args ...string) (*MonitoredProc, error) {
	// look for an existing process first
	shouldStart, err := p.checkProcessExists(os.Args[0], procSearchPattern, policy)
	if err != nil {
		return nil, err
	}

	if !shouldStart {
		// based on the presence of an existing process and the given policy, we should not start a process, bail out.
		return nil, nil
	}

	logger.Infof("Starting process %s with args: %v", daemon, args)
	cmd, err := p.executor.StartExecuteCommand(logName, os.Args[0], createDaemonArgs(daemon, args...)...)
	if err != nil {
		return nil, fmt.Errorf("failed to start daemon %s: %+v", daemon, err)
	}

	proc := newMonitoredProc(p, cmd)

	// monitor the process if it was not mocked
	if cmd != nil && cmd.Process != nil {
		go proc.Monitor(logName)
	}

	p.Lock()
	p.procs = append(p.procs, proc)
	p.Unlock()
	return proc, nil
}

func (p *ProcManager) Shutdown() {
	p.RLock()
	for _, proc := range p.procs {
		proc.Stop()
	}
	p.procs = nil
	p.RUnlock()
}

func (p *ProcManager) checkProcessExists(binary, procSearchPattern string, policy ProcStartPolicy) (bool, error) {
	existingProc, err := findProcessSearch(binary, procSearchPattern)
	if err != nil {
		return false, fmt.Errorf("failed to search for process %s with pattern '%s': %+v", binary, procSearchPattern, err)
	}

	if existingProc == nil {
		logger.Infof("no existing process found for binary %s and pattern '%s'", binary, procSearchPattern)
		return true, nil
	}

	logger.Infof("existing process found for binary %s with pid %d, cmdline '%s'.", binary, existingProc.pid, existingProc.cmdline)
	if policy == ReuseExisting {
		logger.Infof("Policy is 'reuse', reusing existing process.")
		return false, nil
	}

	logger.Infof("Policy is 'restart', restarting existing process.")
	// find our managed instance and try to stop the process through that
	stopped := false
	p.RLock()
	for i := range p.procs {
		proc := p.procs[i].cmd
		if proc != nil && proc.Process != nil && proc.Process.Pid == existingProc.pid {
			if err := p.procs[i].Stop(); err != nil {
				logger.Warningf("failed to stop child process %d: %v", existingProc.pid, err)
				break
			}

			stopped = true
			break
		}
	}
	p.RUnlock()

	if !stopped {
		// we could't stop the existing process through our own managed proces set, try to stop the process
		// via a direct signal to its PID
		if err := syscall.Kill(existingProc.pid, syscall.SIGKILL); err != nil {
			return false, fmt.Errorf("failed to stop child process %d: %v", existingProc.pid, err)
		}
		stopped = true
	}

	if stopped {
		// the child process was stopped, remove it from our managed list (if it's in there)
		p.removeManagedProc(existingProc.pid)
	}

	return true, nil
}

func (p *ProcManager) removeManagedProc(pid int) {
	procNum := -1

	p.Lock()
	for i := range p.procs {
		proc := p.procs[i].cmd
		if proc != nil && proc.Process != nil && proc.Process.Pid == pid {
			procNum = i
			break
		}
	}

	if procNum >= 0 {
		p.procs[procNum] = p.procs[len(p.procs)-1]
		p.procs[len(p.procs)-1] = nil
		p.procs = p.procs[:len(p.procs)-1]
	}
	p.Unlock()
}

func createDaemonArgs(daemon string, args ...string) []string {
	return append(
		[]string{"daemon", fmt.Sprintf("--type=%s", daemon), "--"},
		args...)
}

func createToolArgs(tool string, args ...string) []string {
	return append(
		[]string{"tool", fmt.Sprintf("--type=%s", tool), "--"},
		args...)
}
