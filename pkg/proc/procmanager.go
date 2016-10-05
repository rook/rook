package proc

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
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
	procs []*MonitoredProc

	// test override method to handle starting processes
	Trap func(action string, cmd *exec.Cmd) error
}

func (p *ProcManager) Run(daemon string, args ...string) error {
	log.Printf("Running process %s with args: %v", daemon, args)
	err := p.runChildProcess(daemon, args...)
	if err != nil {
		return fmt.Errorf("failed to run %s: %+v", daemon, err)
	}

	return nil
}

// Start the given daemon and provided arguments.  Handling of any matching existing process will be in accordance
// with the given ProcStartPolicy.  The search pattern will be used to search through the cmdline args of existing
// processes to find any matching existing process.  Therefore, it should be a regex pattern that can uniquely
// identify the process (e.g., --id=1)
func (p *ProcManager) Start(daemon, procSearchPattern string, policy ProcStartPolicy, args ...string) (*MonitoredProc, error) {
	// look for an existing process first
	shouldStart, err := p.checkProcessExists(os.Args[0], procSearchPattern, policy)
	if err != nil {
		return nil, err
	}

	if !shouldStart {
		// based on the presence of an existing process and the given policy, we should not start a process, bail out.
		return nil, nil
	}

	log.Printf("Starting process %s with args: %v", daemon, args)
	process, err := p.startChildProcess(daemon, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to start daemon %s: %+v", daemon, err)
	}

	proc := newMonitoredProc(p, process)

	// monitor the process if it was not mocked
	if process != nil && process.Process != nil {
		go proc.Monitor()
	}

	p.Lock()
	p.procs = append(p.procs, proc)
	p.Unlock()
	return proc, nil
}

func (p *ProcManager) stopChildProcess(cmd *exec.Cmd) error {

	if p.Trap != nil {
		// mock stopping the process
		return p.Trap(StopAction, cmd)
	}

	pid := cmd.Process.Pid
	fmt.Printf("stopping child process %d\n", pid)
	if err := cmd.Process.Kill(); err != nil {
		fmt.Printf("failed to stop child process %d: %+v\n", pid, err)
		return err
	}

	fmt.Printf("child process %d stopped successfully\n", pid)
	return nil
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
		log.Printf("no existing process found for binary %s and pattern '%s'", binary, procSearchPattern)
		return true, nil
	}

	log.Printf("existing process found for binary %s with pid %d, cmdline '%s'.", binary, existingProc.pid, existingProc.cmdline)
	if policy == ReuseExisting {
		log.Printf("Policy is 'reuse', reusing existing process.")
		return false, nil
	}

	log.Printf("Policy is 'restart', restarting existing process.")
	// find our managed instance and try to stop the process through that
	stopped := false
	p.RLock()
	for i := range p.procs {
		proc := p.procs[i].cmd
		if proc != nil && proc.Process != nil && proc.Process.Pid == existingProc.pid {
			if err := p.procs[i].Stop(); err != nil {
				log.Printf("failed to stop child process %d: %v", existingProc.pid, err)
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

func (p *ProcManager) runChildProcess(daemon string, args ...string) error {
	cmd := createCmd(daemon, args...)
	if p.Trap != nil {
		// mock running the process
		return p.Trap(RunAction, cmd)
	}

	return cmd.Run()
}

func (p *ProcManager) startChildProcess(daemon string, args ...string) (*exec.Cmd, error) {
	cmd := createCmd(daemon, args...)
	err := p.startChildProcessCmd(cmd)
	return cmd, err
}

func (p *ProcManager) startChildProcessCmd(cmd *exec.Cmd) (err error) {
	if p.Trap != nil {
		// mock starting the process
		return p.Trap(StartAction, cmd)
	}

	return cmd.Start()
}

func createCmd(daemon string, args ...string) (cmd *exec.Cmd) {
	cmd = exec.Command(os.Args[0], append([]string{"daemon", fmt.Sprintf("--type=%s", daemon), "--"}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd
}
