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
	"regexp"
	"strings"
	"sync"
	"syscall"

	ps "github.com/jbw976/go-ps"
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
func (p *ProcManager) RunWithOutput(logName, command string, args ...string) (string, error) {

	logger.Infof("Running process %s with args: %v", command, args)
	output, err := p.executor.ExecuteCommandWithOutput(false, logName, command, args...)
	if err != nil {
		return "", fmt.Errorf("failed to run %s: %+v", command, err)
	}

	return output, nil
}

// Start a child process and wait for its completion
func (p *ProcManager) RunWithCombinedOutput(logName, command string, args ...string) (string, error) {

	logger.Infof("Running process %s with args: %v", command, args)
	output, err := p.executor.ExecuteCommandWithCombinedOutput(false, logName, command, args...)
	if err != nil {
		return "", fmt.Errorf("failed to run %s: %+v", command, err)
	}

	return output, nil
}

// Start a child process and wait for its completion
func (p *ProcManager) Run(logName, command string, args ...string) error {

	logger.Infof("Running process %s with args: %v", command, args)
	err := p.executor.ExecuteCommand(false, logName, command, args...)
	if err != nil {
		return fmt.Errorf("failed to run %s: %+v", command, err)
	}

	return nil
}

// Start the given process with the provided arguments.  Handling of any matching existing process will be in accordance
// with the given ProcStartPolicy.  The search pattern will be used to search through the cmdline args of existing
// processes to find any matching existing process.  Therefore, it should be a regex pattern that can uniquely
// identify the process (e.g., --id=1)
func (p *ProcManager) Start(name, command, procSearchPattern string, policy ProcStartPolicy, args ...string) (*MonitoredProc, error) {
	// look for an existing process first
	shouldStart, err := p.checkProcessExists(command, procSearchPattern, policy)
	if err != nil {
		return nil, err
	}

	if !shouldStart {
		// based on the presence of an existing process and the given policy, we should not start a process, bail out.
		return nil, nil
	}

	logger.Infof("Starting process %s with args: %v", name, args)
	cmd, err := p.executor.StartExecuteCommand(false, name, command, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to start process %s: %+v", name, err)
	}

	proc := newMonitoredProc(p, cmd)

	// monitor the process if it was not mocked
	if cmd != nil && cmd.Process != nil {
		go proc.Monitor(name)
	}

	p.Lock()
	p.procs = append(p.procs, proc)
	p.Unlock()
	return proc, nil
}

func (p *ProcManager) Shutdown() {
	p.RLock()
	for _, proc := range p.procs {
		proc.Stop(false)
	}
	p.procs = nil
	p.RUnlock()
}

// Checks if a process exists. If the restart policy indicates the process should be restarted,
// the process will be stopped here.
func (p *ProcManager) checkProcessExists(binary, procSearchPattern string, policy ProcStartPolicy) (shouldStart bool, err error) {
	// check if this process is already being monitored
	p.RLock()
	defer p.RUnlock()
	if index, proc := p.findMonitoredProc(binary, procSearchPattern); proc != nil {
		if policy == ReuseExisting {
			// no need to start the process since the process manager is already watching to ensure it is running
			logger.Infof("Process is already being managed and will not be started again. %s", procSearchPattern)
			return false, nil
		} else if policy == RestartExisting {
			// stop the process and the process manager so a new process manager can be started.
			// the caller is responsible to start a new process
			logger.Infof("Policy is 'restart' for %s. Stopping its monitor.", procSearchPattern)
			p.purgeManagedProc(index, proc)
			return true, nil
		}
	}

	// check if this process is currently running even though not being managed
	existingProc, err := ps.FindProcessByCmdline(binary, procSearchPattern)
	if err != nil {
		return false, fmt.Errorf("failed to search for process %s with pattern '%s': %+v", binary, procSearchPattern, err)
	}

	if existingProc == nil {
		logger.Infof("no existing process found for binary %s and pattern '%s'", binary, procSearchPattern)
		return true, nil
	}

	logger.Infof("existing process found for binary %s with pid %d, cmdline '%s'.", binary, existingProc.Pid(), existingProc.Cmdline())
	if policy == ReuseExisting {
		logger.Infof("Policy is 'reuse', reusing existing process.")
		return false, nil
	}

	logger.Infof("Policy is 'restart', restarting existing process.")

	// double check if the process is managed and stop it
	index, proc := p.findMonitoredProcByPID(existingProc.Pid())
	if proc != nil {
		p.purgeManagedProc(index, proc)
		return true, nil
	}

	// we couldn't stop the existing process through our own managed process set, try to stop the process
	// via a direct signal to its PID
	if err := syscall.Kill(existingProc.Pid(), syscall.SIGKILL); err != nil {
		return false, fmt.Errorf("failed to stop child process %d: %v", existingProc.Pid(), err)
	}

	return true, nil
}

// Stop and remove a process from the list of managed.
// Assumes we are inside the process lock.
func (p *ProcManager) purgeManagedProc(index int, proc *MonitoredProc) {
	err := proc.Stop(false)
	if err != nil {
		logger.Warningf("did not stop process %+v. %+v", proc.cmd, err)
	}

	// remove the process from our managed list (if it's in there)
	// If the process failed to stop, it is still not being managed so we will go ahead and remove it
	if index >= 0 {
		p.procs[index] = p.procs[len(p.procs)-1]
		p.procs[len(p.procs)-1] = nil
		p.procs = p.procs[:len(p.procs)-1]
	}
}

// Find a monitored process.
// Assumes we are inside the process lock.
func (p *ProcManager) findMonitoredProc(binary, searchPattern string) (int, *MonitoredProc) {
	// find if the process is being monitored
	for index, proc := range p.procs {
		if os.Args[0] != binary {
			// skip processes that are not owned by rook
			continue
		}

		cmdline := strings.Join(proc.cmd.Args, " ")
		if matched, _ := regexp.MatchString(searchPattern, cmdline); matched {
			logger.Infof("found monitored process for %s", searchPattern)
			return index, proc
		}

	}

	return -1, nil
}

// Find a monitored process by its pid.
// Assumes we are inside the process lock.
func (p *ProcManager) findMonitoredProcByPID(pid int) (int, *MonitoredProc) {
	// find if the process is being monitored
	for index, proc := range p.procs {
		if proc.cmd.Process != nil && proc.cmd.Process.Pid == pid {
			return index, proc
		}
	}

	return -1, nil
}
