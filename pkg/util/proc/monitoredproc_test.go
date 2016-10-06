package proc

import (
	"errors"
	"log"
	"os/exec"
	"testing"

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
	procMgr := &ProcManager{}
	cmd := &exec.Cmd{Args: []string{"/my/path", "1", "2", "3"}}
	proc := newMonitoredProc(procMgr, cmd)
	proc.retrySecondsExponentBase = 0.0

	traps := 0
	procMgr.Trap = func(action string, cmd *exec.Cmd) error {
		traps++
		switch {
		case traps == 1:
			return errors.New("test failure")
		case traps == 2:
			assert.Equal(t, 1, proc.retries)
			break
		}
		return nil
	}

	iter := 0
	proc.waitForExit = func() {
		log.Printf("waited for process")
		assert.True(t, proc.monitor)
		switch {
		case iter == 0:
		case iter == 1:
			assert.True(t, proc.monitor)
			log.Printf("stop monitoring")
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
