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
	"math"
	"os/exec"
	"syscall"
	"time"
)

const (
	maxDelaySeconds = 30
)

type MonitoredProc struct {
	parent                   *ProcManager
	cmd                      *exec.Cmd
	monitor                  bool
	retries                  int
	totalRetries             int
	retrySecondsExponentBase float64
	waitForExit              func()
}

func newMonitoredProc(p *ProcManager, cmd *exec.Cmd) *MonitoredProc {
	m := &MonitoredProc{
		parent:                   p,
		cmd:                      cmd,
		retrySecondsExponentBase: 2,
	}
	m.waitForExit = m.waitForProcessExit
	return m
}

func (p *MonitoredProc) Monitor(logName string) {
	p.monitor = true
	var err error
	var lastRetryCheck time.Time
	var lastStartTime time.Time

	for {
		// wait for the given process to complete, unless the last retry had failed immediately
		if err == nil {
			p.waitForExit()
		}

		if !p.monitor {
			logger.Infof("done monitoring process %v", p.cmd.Args)
			break
		}

		// calculate the delay
		delay := calcRetryDelay(p.retrySecondsExponentBase, p.retries, time.Now(), lastStartTime, lastRetryCheck)
		lastRetryCheck = time.Now()

		logger.Infof("starting process %v again after %.1f seconds", p.cmd.Args, delay)
		<-time.After(time.Second * time.Duration(delay))

		if !p.monitor {
			logger.Infof("done monitoring process %v", p.cmd.Args)
			break
		}

		// start the process
		p.cmd, err = p.parent.executor.StartExecuteCommand(false, logName, p.cmd.Args[0], p.cmd.Args[1:]...)
		if err != nil {
			logger.Warningf("retry %d (total %d): process %v failed to restart. %v", p.retries, p.totalRetries, p.cmd.Args, err)
			p.retries++
		} else {
			logger.Infof("retry (total %d). started process %v", p.totalRetries, p.cmd.Args)
			lastStartTime = time.Now()
			p.retries = 0
		}

		p.totalRetries++
	}
}

func (p *MonitoredProc) waitForProcessExit() {
	state, err := p.cmd.Process.Wait()
	if err != nil {
		logger.Errorf("waiting for process %d had an error: %+v", p.cmd.Process.Pid, err)
		return
	}

	// check the wait status of the process which has all the exit information
	waitStatus, ok := state.Sys().(syscall.WaitStatus)
	if !ok {
		logger.Errorf("unknown waitStatus for process %d: %+v", p.cmd.Process.Pid, state.Sys())
		return
	}

	logger.Infof("process %d completed.  Exited: %t, ExitStatus: %d, Signaled: %t, Signal: %d, %+v",
		p.cmd.Process.Pid, waitStatus.Exited(), waitStatus.ExitStatus(), waitStatus.Signaled(), waitStatus.Signal(), p.cmd)
}

func (p *MonitoredProc) Stop(mon bool) error {
	p.monitor = mon
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}

	pid := p.cmd.Process.Pid
	logger.Infof("stopping child process %d\n", pid)
	if err := p.cmd.Process.Kill(); err != nil {
		logger.Errorf("failed to stop child process %d: %+v\n", pid, err)
		return err
	}

	logger.Infof("child process %d stopped successfully\n", pid)
	return nil
}

// determines the time to delay in seconds before retrying again
func calcRetryDelay(base float64, retries int, now, lastStartTime, lastRetryCheck time.Time) float64 {
	if base == 0 {
		return 0
	}

	power := 0.0
	if retries > 0 {
		power = float64(retries)
	} else if !lastRetryCheck.IsZero() && !lastStartTime.IsZero() && now.Sub(lastStartTime).Seconds() < maxDelaySeconds {
		// the process was running for shorter than the max retry delay, so it wasn't running for too long before
		// it exited again. retry count is 0, so the process did start "successfully" last time, but in order to prevent
		// rapid retry loops where the process starts successfully but crashes right after, let's use the number of
		// seconds since the last retry check as a proxy for retry count.  this will give us exponential back off
		// for a process that crashes immediately after starting.
		power = now.Sub(lastRetryCheck).Seconds()
	}

	delay := math.Pow(base, power)
	if delay > float64(maxDelaySeconds) {
		return float64(maxDelaySeconds)
	}

	return delay
}
