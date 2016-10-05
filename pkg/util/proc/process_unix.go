// TODO: this source is from https://github.com/mitchellh/go-ps/blob/master/process_unix.go
// and it has some additional functionality.  Propery license this code, or even better fork
// the repo and get our additions into upstream, then just consume upstream via vendoring.

package proc

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// UnixProcess is an implementation of Process that contains Unix-specific
// fields and information.
type UnixProcess struct {
	pid   int
	ppid  int
	state rune
	pgrp  int
	sid   int

	binary  string
	cmdline string
}

func (p *UnixProcess) Pid() int {
	return p.pid
}

func (p *UnixProcess) PPid() int {
	return p.ppid
}

func (p *UnixProcess) Executable() string {
	return p.binary
}

// Refresh reloads all the data associated with this process.
func (p *UnixProcess) Refresh() error {
	statPath := fmt.Sprintf("/proc/%d/stat", p.pid)
	dataBytes, err := ioutil.ReadFile(statPath)
	if err != nil {
		return err
	}

	// First, parse out the image name
	data := string(dataBytes)
	binStart := strings.IndexRune(data, '(') + 1
	binEnd := strings.IndexRune(data[binStart:], ')')
	p.binary = data[binStart : binStart+binEnd]

	// Move past the image name and start parsing the rest
	data = data[binStart+binEnd+2:]
	_, err = fmt.Sscanf(data,
		"%c %d %d %d",
		&p.state,
		&p.ppid,
		&p.pgrp,
		&p.sid)

	// retrieve the cmdline for the process as well
	cmdlineFilePath := fmt.Sprintf("/proc/%d/cmdline", p.pid)
	dataBytes, err = ioutil.ReadFile(cmdlineFilePath)
	if err != nil {
		return err
	}

	p.cmdline = strings.TrimSpace(string(dataBytes))

	return err
}

func findProcess(pid int) (*UnixProcess, error) {
	dir := fmt.Sprintf("/proc/%d", pid)
	_, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, err
	}

	return newUnixProcess(pid)
}

func findProcessSearch(binary, cmdlinePattern string) (*UnixProcess, error) {
	if cmdlinePattern == "" {
		return nil, fmt.Errorf("no cmdline search pattern provided")
	}

	procs, err := processes()
	if err != nil {
		return nil, err
	}

	b := filepath.Base(binary)

	for i := range procs {
		if procs[i].binary == b {
			if matched, _ := regexp.MatchString(cmdlinePattern, procs[i].cmdline); matched {
				return procs[i], nil
			}
		}
	}

	return nil, nil
}

func processes() ([]*UnixProcess, error) {
	d, err := os.Open("/proc")
	if err != nil {
		return nil, err
	}
	defer d.Close()

	results := make([]*UnixProcess, 0, 50)
	for {
		fis, err := d.Readdir(10)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		for _, fi := range fis {
			// We only care about directories, since all pids are dirs
			if !fi.IsDir() {
				continue
			}

			// We only care if the name starts with a numeric
			name := fi.Name()
			if name[0] < '0' || name[0] > '9' {
				continue
			}

			// From this point forward, any errors we just ignore, because
			// it might simply be that the process doesn't exist anymore.
			pid, err := strconv.ParseInt(name, 10, 0)
			if err != nil {
				continue
			}

			p, err := newUnixProcess(int(pid))
			if err != nil {
				continue
			}

			results = append(results, p)
		}
	}

	return results, nil
}

func newUnixProcess(pid int) (*UnixProcess, error) {
	p := &UnixProcess{pid: pid}
	return p, p.Refresh()
}
