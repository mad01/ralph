package hooks

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/testutil"
)

// maxStopTime is how long a timed-out or cancelled command may take to
// return, measured from its start. The commands under test would otherwise
// run for 30 s.
const maxStopTime = 5 * time.Second

// stallingCommand starts a background child that holds the output pipe, then
// blocks in the foreground. The child's PID goes to pidFile.
func stallingCommand(pidFile string) string {
	return fmt.Sprintf("sleep 30 & echo $! > '%s'; sleep 30", pidFile)
}

func TestRunBuild_TimeoutStopsChildProcesses(t *testing.T) {
	_ = testutil.WithHome(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	build := config.Build{
		Commands: []string{stallingCommand(pidFile)},
		Run:      "always",
		Timeout:  1,
	}

	var out bytes.Buffer
	start := time.Now()
	err := RunBuild(context.Background(), &out, "stall", build, "testhost", nil, BuildOptions{})
	elapsed := time.Since(start)

	if err == nil || !strings.Contains(err.Error(), "timed out after 1s") {
		t.Fatalf("RunBuild error = %v, want a 'timed out after 1s' error", err)
	}
	if elapsed > maxStopTime {
		t.Errorf("RunBuild returned after %s, want under %s", elapsed, maxStopTime)
	}
	testutil.RequireProcessGone(t, testutil.ReadPID(t, pidFile))
}

func TestRunBuild_ScriptTimeoutStopsChildProcesses(t *testing.T) {
	_ = testutil.WithHome(t)
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	script := filepath.Join(dir, "build.sh")
	if err := os.WriteFile(script, []byte(stallingCommand(pidFile)+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	build := config.Build{Script: script, Run: "always", Timeout: 1}

	var out bytes.Buffer
	start := time.Now()
	err := RunBuild(context.Background(), &out, "stall", build, "testhost", nil, BuildOptions{})
	elapsed := time.Since(start)

	if err == nil || !strings.Contains(err.Error(), "timed out after 1s") {
		t.Fatalf("RunBuild error = %v, want a 'timed out after 1s' error", err)
	}
	if elapsed > maxStopTime {
		t.Errorf("RunBuild returned after %s, want under %s", elapsed, maxStopTime)
	}
	testutil.RequireProcessGone(t, testutil.ReadPID(t, pidFile))
}

func TestRunBuild_ContextCancelStopsChildProcesses(t *testing.T) {
	_ = testutil.WithHome(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	build := config.Build{Commands: []string{stallingCommand(pidFile)}, Run: "always"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_, _ = testutil.AwaitPID(pidFile)
		cancel()
	}()

	var out bytes.Buffer
	start := time.Now()
	err := RunBuild(ctx, &out, "stall", build, "testhost", nil, BuildOptions{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("RunBuild succeeded after its context was cancelled, want an error")
	}
	if elapsed > maxStopTime {
		t.Errorf("RunBuild returned after %s, want under %s", elapsed, maxStopTime)
	}
	testutil.RequireProcessGone(t, testutil.ReadPID(t, pidFile))
}

func TestRun_HookTimeoutStopsChildProcesses(t *testing.T) {
	orig := hookTimeout
	hookTimeout = 500 * time.Millisecond
	t.Cleanup(func() { hookTimeout = orig })
	pidFile := filepath.Join(t.TempDir(), "pid")

	var out bytes.Buffer
	start := time.Now()
	err := RunHooks(&out, []string{stallingCommand(pidFile)}, PostApply, &HookContext{}, false)
	elapsed := time.Since(start)

	if err == nil || !strings.Contains(err.Error(), "hook timed out after 500ms") {
		t.Fatalf("RunHooks error = %v, want a 'hook timed out after 500ms' error", err)
	}
	if elapsed > maxStopTime {
		t.Errorf("RunHooks returned after %s, want under %s", elapsed, maxStopTime)
	}
	testutil.RequireProcessGone(t, testutil.ReadPID(t, pidFile))
}
