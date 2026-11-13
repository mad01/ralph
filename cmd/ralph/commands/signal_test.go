package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/execx"
	"github.com/mad01/ralph/internal/testutil"
)

// signalHelperEnv carries the PID file path to the helper process and marks
// the test binary as running in helper mode.
const signalHelperEnv = "RALPH_TEST_SIGNAL_HELPER_PID_FILE"

// TestSignalHelperProcess is not a test on its own: TestInterruptStopsRunning
// Commands re-runs the test binary with signalHelperEnv set, and this function
// then acts as ralph in the middle of a long build.
func TestSignalHelperProcess(t *testing.T) {
	pidFile := os.Getenv(signalHelperEnv)
	if pidFile == "" {
		t.Skip("helper process for TestInterruptStopsRunningCommands")
	}
	stopOnSignal()
	script := fmt.Sprintf("sleep 30 & echo $! > '%s'; sleep 30", pidFile)
	_ = execx.Command(context.Background(), "sh", "-c", script).Run()
	// The signal handler exits the process; getting here means it did not.
	time.Sleep(10 * time.Second)
	os.Exit(3)
}

func TestInterruptStopsRunningCommands(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	helper := exec.Command(os.Args[0], "-test.run=^TestSignalHelperProcess$")
	helper.Env = append(os.Environ(), signalHelperEnv+"="+pidFile)
	if err := helper.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- helper.Wait() }()
	t.Cleanup(func() { _ = helper.Process.Kill() })

	child := testutil.ReadPID(t, pidFile)
	if err := helper.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}

	var err error
	select {
	case err = <-exited:
	case <-time.After(10 * time.Second):
		t.Fatal("ralph did not exit within 10s of SIGINT")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("helper exit = %v, want a non-zero exit", err)
	}
	if got, want := exitErr.ExitCode(), 128+int(syscall.SIGINT); got != want {
		t.Errorf("exit code = %d, want %d", got, want)
	}
	testutil.RequireProcessGone(t, child)
}
