package proc

import (
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

		// initialize a new cmd object with the same arguments
		p.cmd = exec.Command(p.cmd.Args[0], p.cmd.Args[1:]...)

		// start the process
		err = p.parent.startChildProcessCmd(p.cmd)
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
	return p.parent.stopChildProcess(p.cmd)
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
