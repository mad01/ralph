package gitutil

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/testutil"
)

// stallSSH points git's ssh transport at a shell command that records its PID
// and never answers, so an ssh:// remote hangs without touching the network.
// It returns the PID file path. The command is inline rather than a fresh
// script file: macOS scans a newly written executable on first run, which can
// take longer than the test's timeout.
func stallSSH(t *testing.T) string {
	t.Helper()
	pidFile := filepath.Join(t.TempDir(), "pid")
	t.Setenv("GIT_SSH_COMMAND", `sh -c 'echo $$ > "$STALL_PID_FILE"; exec sleep 30'`)
	t.Setenv("GIT_SSH_VARIANT", "simple") // no `-G` probe of the fake ssh
	t.Setenv("STALL_PID_FILE", pidFile)
	return pidFile
}

func TestClone_StalledRemoteTimesOut(t *testing.T) {
	orig := networkTimeout
	networkTimeout = time.Second
	t.Cleanup(func() { networkTimeout = orig })
	pidFile := stallSSH(t)

	target := filepath.Join(t.TempDir(), "clone")
	start := time.Now()
	err := Clone(io.Discard, "ssh://stall.invalid/repo.git", target, "")
	elapsed := time.Since(start)

	if err == nil || !strings.Contains(err.Error(), "git clone timed out after 1s") {
		t.Fatalf("Clone error = %v, want a 'git clone timed out after 1s' error", err)
	}
	if limit := 5 * time.Second; elapsed > limit {
		t.Errorf("Clone returned after %s, want under %s", elapsed, limit)
	}
	testutil.RequireProcessGone(t, testutil.ReadPID(t, pidFile))
}

func TestRunGit_QueryTimeoutNamesCommand(t *testing.T) {
	err := timeoutError([]string{"status", "--porcelain"}, 2*time.Minute)
	if got, want := err.Error(), "git status timed out after 2m0s"; got != want {
		t.Errorf("timeoutError = %q, want %q", got, want)
	}
}
