package tool

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/testutil"
)

func TestCheckStatus(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{"true succeeds", "true", true},
		{"false fails", "false", false},
		{"empty string", "", false},
		{"whitespace only", "   ", false},
		{"echo succeeds", "echo hello", true},
		{"nonexistent command", "nonexistent_command_abc123", false},
		{"command -v existing", "command -v sh", true},
		{"command -v missing", "command -v nonexistent_cmd_xyz", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CheckStatus(tt.cmd)
			if err != nil {
				t.Fatalf("CheckStatus(%q) error = %v, want nil", tt.cmd, err)
			}
			if got != tt.want {
				t.Errorf("CheckStatus(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestCheckStatus_TimeoutStopsChildProcesses(t *testing.T) {
	orig := checkTimeout
	checkTimeout = 500 * time.Millisecond
	t.Cleanup(func() { checkTimeout = orig })

	pidFile := filepath.Join(t.TempDir(), "pid")
	check := fmt.Sprintf("sleep 30 & echo $! > '%s'; sleep 30", pidFile)

	start := time.Now()
	installed, err := CheckStatus(check)
	elapsed := time.Since(start)

	if installed {
		t.Error("CheckStatus reported installed for a check that timed out")
	}
	if err == nil || !strings.Contains(err.Error(), "timed out after 500ms") {
		t.Errorf("CheckStatus error = %v, want a 'timed out after 500ms' error", err)
	}
	if limit := 5 * time.Second; elapsed > limit {
		t.Errorf("CheckStatus returned after %s, want under %s", elapsed, limit)
	}
	testutil.RequireProcessGone(t, testutil.ReadPID(t, pidFile))
}
