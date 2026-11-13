package execx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/testutil"
)

// maxStopTime is how long a stopped command may take to return. The commands
// under test would otherwise run for 30 s.
const maxStopTime = 3 * time.Second

// stalling returns a shell script that starts a background child holding the
// output pipe, writes the child's PID to pidFile, and blocks in the
// foreground. prefix runs first (e.g. a trap).
func stalling(prefix, pidFile string) string {
	return fmt.Sprintf("%s sleep 30 & echo $! > '%s'; sleep 30", prefix, pidFile)
}

// shorten sets *v to d for the duration of the test.
func shorten(t *testing.T, v *time.Duration, d time.Duration) {
	t.Helper()
	orig := *v
	*v = d
	t.Cleanup(func() { *v = orig })
}

// runTimed runs cmd and returns how long it took and its error.
func runTimed(cmd *Cmd) (time.Duration, error) {
	start := time.Now()
	err := cmd.Run()
	return time.Since(start), err
}

func TestCommand_TimeoutStopsProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	cmd := Command(ctx, "sh", "-c", stalling("", pidFile))
	cmd.Stdout = &bytes.Buffer{} // a pipe: a leftover child would block Wait
	elapsed, err := runTimed(cmd)

	if err == nil {
		t.Fatal("Run succeeded after its timeout, want an error")
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("ctx.Err() = %v, want DeadlineExceeded", ctx.Err())
	}
	if elapsed > maxStopTime {
		t.Errorf("Run returned after %s, want under %s", elapsed, maxStopTime)
	}
	testutil.RequireProcessGone(t, testutil.ReadPID(t, pidFile))
}

func TestCommand_CancelStopsProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_, _ = testutil.AwaitPID(pidFile)
		cancel()
	}()

	cmd := Command(ctx, "sh", "-c", stalling("", pidFile))
	cmd.Stdout = &bytes.Buffer{}
	elapsed, err := runTimed(cmd)

	if err == nil {
		t.Fatal("Run succeeded after its context was cancelled, want an error")
	}
	if elapsed > maxStopTime {
		t.Errorf("Run returned after %s, want under %s", elapsed, maxStopTime)
	}
	testutil.RequireProcessGone(t, testutil.ReadPID(t, pidFile))
}

func TestCommand_KillsGroupThatIgnoresSIGTERM(t *testing.T) {
	shorten(t, &killGrace, 200*time.Millisecond)
	pidFile := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	// An ignored signal stays ignored across fork and exec, so the shell and
	// both sleeps ignore SIGTERM and only SIGKILL stops them.
	cmd := Command(ctx, "sh", "-c", stalling("trap '' TERM;", pidFile))
	cmd.Stdout = &bytes.Buffer{}
	elapsed, err := runTimed(cmd)

	if err == nil {
		t.Fatal("Run succeeded after its timeout, want an error")
	}
	if elapsed > maxStopTime {
		t.Errorf("Run returned after %s, want under %s", elapsed, maxStopTime)
	}
	testutil.RequireProcessGone(t, testutil.ReadPID(t, pidFile))
}

func TestCommand_BackgroundChildHoldingOutputDoesNotBlock(t *testing.T) {
	shorten(t, &waitDelay, 200*time.Millisecond)
	pidFile := filepath.Join(t.TempDir(), "pid")

	// The shell exits 0 at once; the background sleep keeps stdout open.
	cmd := Command(
		context.Background(),
		"sh",
		"-c",
		fmt.Sprintf("sleep 30 & echo $! > '%s'", pidFile),
	)
	cmd.Stdout = &bytes.Buffer{}
	elapsed, err := runTimed(cmd)
	if err != nil {
		t.Errorf("Run error = %v, want nil: the command itself exited 0", err)
	}
	if elapsed > maxStopTime {
		t.Errorf("Run returned after %s, want under %s", elapsed, maxStopTime)
	}
	_ = testutil.ReadPID(t, pidFile) // kills the leftover sleep on cleanup
}

func TestTerminateAll_StopsRunningCommandsAndRefusesNewOnes(t *testing.T) {
	t.Cleanup(func() {
		running.Lock()
		running.stopping = false
		running.Unlock()
	})
	pidFile := filepath.Join(t.TempDir(), "pid")

	// Run blocks in its own goroutine, as it does on ralph's main goroutine
	// when the signal handler calls TerminateAll.
	cmd := Command(context.Background(), "sh", "-c", stalling("", pidFile))
	cmd.Stdout = &bytes.Buffer{}
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	pid := testutil.ReadPID(t, pidFile)

	start := time.Now()
	TerminateAll()
	err := <-done
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Wait succeeded after TerminateAll, want an error")
	}
	if elapsed > maxStopTime {
		t.Errorf("TerminateAll and Wait took %s, want under %s", elapsed, maxStopTime)
	}
	testutil.RequireProcessGone(t, pid)

	if err := Command(context.Background(), "true").Run(); !errors.Is(err, ErrStopped) {
		t.Errorf("Run after TerminateAll error = %v, want ErrStopped", err)
	}
}

func TestOutput_KeepsStderrOnFailure(t *testing.T) {
	out, err := Command(context.Background(), "sh", "-c", "echo out; echo err >&2; exit 3").Output()

	if got := string(out); got != "out\n" {
		t.Errorf("stdout = %q, want %q", got, "out\n")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("error = %v, want exit status 3", err)
	}
	if got := string(exitErr.Stderr); got != "err\n" {
		t.Errorf("ExitError.Stderr = %q, want %q", got, "err\n")
	}
}

func TestCombinedOutput_MergesStreams(t *testing.T) {
	out, err := Command(context.Background(), "sh", "-c", "echo out; echo err >&2").CombinedOutput()
	if err != nil {
		t.Fatalf("CombinedOutput error = %v", err)
	}
	if got := string(out); got != "out\nerr\n" {
		t.Errorf("output = %q, want %q", got, "out\nerr\n")
	}
}

func TestCommand_HasNoControllingTerminal(t *testing.T) {
	// A command in its own session cannot open /dev/tty, so a prompt fails
	// at once instead of stopping the process on SIGTTIN.
	err := Command(context.Background(), "sh", "-c", ": < /dev/tty").Run()
	if err == nil {
		t.Error("opening /dev/tty succeeded, want a failure without a controlling terminal")
	}
}
