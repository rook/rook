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
	"log"
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
	m := &MonitoredProc{parent: p, cmd: cmd, retrySecondsExponentBase: 2}
	m.waitForExit = m.waitForProcessExit
	return m
}

func (p *MonitoredProc) Monitor() {
	p.monitor = true
	var err error

	for {
		// wait for the given process to complete, unless the last retry had failed immediately
		if err == nil {
			p.waitForExit()
		}

		if !p.monitor {
			log.Printf("done monitoring process %v", p.cmd.Args)
			break
		}

		// calculate the delay
		delaySeconds := calcRetryDelay(p.retrySecondsExponentBase, p.retries)

		log.Printf("starting process %v again after %.1f seconds", p.cmd.Args, delaySeconds)
		<-time.After(time.Second * time.Duration(delaySeconds))

		// start the process
		p.cmd, err = p.parent.executor.StartExecuteCommand("restart", p.cmd.Args[0], p.cmd.Args[1:]...)
		if err != nil {
			log.Printf("retry %d (total %d): process %v failed to restart. %v", p.retries, p.totalRetries, p.cmd.Args, err)
			p.retries++
		} else {
			log.Printf("retry (total %d). started process %v", p.totalRetries, p.cmd.Args)
			p.retries = 0
		}

		p.totalRetries++
	}
}

func (p *MonitoredProc) waitForProcessExit() {
	state, err := p.cmd.Process.Wait()
	if err != nil {
		log.Printf("waiting for process %d had an error: %+v", p.cmd.Process.Pid, err)
		return
	}

	// check the wait status of the process which has all the exit information
	waitStatus, ok := state.Sys().(syscall.WaitStatus)
	if !ok {
		log.Printf("unknown waitStatus for process %d: %+v", p.cmd.Process.Pid, state.Sys())
		return
	}

	log.Printf("process %d completed.  Exited: %t, ExitStatus: %d, Signaled: %t, Signal: %d",
		p.cmd.Process.Pid, waitStatus.Exited(), waitStatus.ExitStatus(), waitStatus.Signaled(), waitStatus.Signal())
}

func (p *MonitoredProc) Stop() error {
	p.monitor = false
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}

	pid := p.cmd.Process.Pid
	fmt.Printf("stopping child process %d\n", pid)
	if err := p.cmd.Process.Kill(); err != nil {
		fmt.Printf("failed to stop child process %d: %+v\n", pid, err)
		return err
	}

	fmt.Printf("child process %d stopped successfully\n", pid)
	return nil
}

func calcRetryDelay(base float64, power int) float64 {
	if base == 0 {
		return 0
	}

	delay := math.Pow(base, float64(power))
	if delay > maxDelaySeconds {
		return maxDelaySeconds
	}

	return delay
}
