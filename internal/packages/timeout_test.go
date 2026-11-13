package packages

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

// maxStopTime is how long a timed-out command may take to return, measured
// from its start. The commands under test would otherwise run for 30 s.
const maxStopTime = 5 * time.Second

// stallingCommand starts a background child that holds the output pipe, then
// blocks in the foreground. The child's PID goes to pidFile.
func stallingCommand(pidFile string) string {
	return fmt.Sprintf("sleep 30 & echo $! > '%s'; sleep 30", pidFile)
}

func TestBuildPackage_TimeoutStopsChildProcesses(t *testing.T) {
	tmpDir := testutil.WithHome(t)
	workDir := filepath.Join(tmpDir, "stall_pkg")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(t.TempDir(), "pid")
	pkg := config.Package{
		Source:     "local",
		WorkingDir: workDir,
		Build:      []string{stallingCommand(pidFile)},
		Timeout:    1,
	}

	var buf bytes.Buffer
	start := time.Now()
	result := BuildPackage(context.Background(), &buf, "stall_pkg", pkg, BuildOptions{Force: true})
	elapsed := time.Since(start)

	if result.Action != "error" || result.Err == nil ||
		!strings.Contains(result.Err.Error(), "timed out after 1s") {
		t.Fatalf("result = %+v, want an error that says 'timed out after 1s'", result)
	}
	if elapsed > maxStopTime {
		t.Errorf("BuildPackage returned after %s, want under %s", elapsed, maxStopTime)
	}
	testutil.RequireProcessGone(t, testutil.ReadPID(t, pidFile))
}

func TestMaybeRestartService_TimeoutStopsChildProcesses(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	pkg := config.Package{
		Timeout: 1,
		Service: &config.Service{Restart: stallingCommand(pidFile)},
	}

	var buf bytes.Buffer
	start := time.Now()
	restarted := maybeRestartService(
		context.Background(),
		&buf,
		"p",
		pkg,
		"old",
		"new",
		BuildOptions{},
	)
	elapsed := time.Since(start)

	if restarted {
		t.Error("maybeRestartService reported success for a restart that timed out")
	}
	if elapsed > maxStopTime {
		t.Errorf("maybeRestartService returned after %s, want under %s", elapsed, maxStopTime)
	}
	testutil.RequireProcessGone(t, testutil.ReadPID(t, pidFile))
}

func TestGitPull_StalledRemoteTimesOut(t *testing.T) {
	orig := gitTimeout
	gitTimeout = time.Second
	t.Cleanup(func() { gitTimeout = orig })

	// A clone whose branch tracks an ssh:// origin served by a shell command
	// that records its PID and never answers: a hung remote, no network.
	repo := t.TempDir()
	testutil.InitGitRepo(t, repo)
	testutil.RunGitCmd(t, repo, "remote", "add", "origin", "ssh://stall.invalid/repo.git")
	branch := getCurrentBranch(context.Background(), repo)
	testutil.RunGitCmd(t, repo, "config", "branch."+branch+".remote", "origin")
	testutil.RunGitCmd(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	pidFile := filepath.Join(t.TempDir(), "pid")
	t.Setenv("GIT_SSH_COMMAND", `sh -c 'echo $$ > "$STALL_PID_FILE"; exec sleep 30'`)
	t.Setenv("GIT_SSH_VARIANT", "simple")
	t.Setenv("STALL_PID_FILE", pidFile)

	var buf bytes.Buffer
	start := time.Now()
	err := GitPull(context.Background(), &buf, repo, false, false)
	elapsed := time.Since(start)

	if err == nil || !strings.Contains(err.Error(), "git pull timed out after 1s") {
		t.Fatalf("GitPull error = %v, want a 'git pull timed out after 1s' error", err)
	}
	if elapsed > maxStopTime {
		t.Errorf("GitPull returned after %s, want under %s", elapsed, maxStopTime)
	}
	// Git keeps the terminal, so only git itself is signalled; the ssh helper
	// is not, and WaitDelay is what bounds the wait. ReadPID kills the
	// helper when the test ends.
	_ = testutil.ReadPID(t, pidFile)
}
