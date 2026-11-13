package testutil

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// processWait bounds how long the process helpers wait for a PID file to
// appear or for a process to exit (and be reaped by init).
const processWait = 5 * time.Second

// processPoll is how often the process helpers check again.
const processPoll = 20 * time.Millisecond

// ReadPID waits for path to hold a process ID, as written by a test command
// such as `sleep 30 & echo $! > path`, and returns it. The process is killed
// when the test ends if it is still running then.
func ReadPID(t *testing.T, path string) int {
	t.Helper()
	pid, ok := AwaitPID(path)
	if !ok {
		t.Fatalf("no PID in %s after %s", path, processWait)
	}
	t.Cleanup(func() {
		if ProcessExists(pid) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	return pid
}

// AwaitPID waits up to a few seconds for path to hold a process ID and
// returns it. Unlike ReadPID it does not touch the test, so a goroutine may
// call it.
func AwaitPID(path string) (int, bool) {
	deadline := time.Now().Add(processWait)
	for {
		if data, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
				return pid, true
			}
		}
		if time.Now().After(deadline) {
			return 0, false
		}
		time.Sleep(processPoll)
	}
}

// ProcessExists reports whether a process with this PID exists. A zombie
// counts as existing until its parent reaps it.
func ProcessExists(pid int) bool {
	return !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

// RequireProcessGone fails the test unless process pid is gone within a few
// seconds. A process still running then is killed, so a failing test leaves
// nothing behind.
func RequireProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(processWait)
	for ProcessExists(pid) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("process %d still running %s after the command returned", pid, processWait)
		}
		time.Sleep(processPoll)
	}
}
