package hooks

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/testutil"
)

// A stored hash shorter than the eight characters the change message shows
// (a hand-edited or truncated .builds_state) must not panic the run=once
// freshness check; the message prints the stored hash as it is.
func TestRunBuild_OnceWithShortStoredHash(t *testing.T) {
	tmpDir := testutil.WithHome(t)
	gitDir := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}
	testutil.InitGitRepo(t, gitDir)
	if GetTreeHash(gitDir) == "" {
		t.Skip("git not available")
	}

	state := &BuildState{Builds: map[string]BuildRecord{
		"git_build": {CompletedAt: time.Now(), GitHash: "abc"},
	}}
	if err := SaveBuildState(state); err != nil {
		t.Fatalf("save build state: %v", err)
	}

	build := config.Build{Commands: []string{"true"}, Run: "once", WorkingDir: gitDir}
	var out bytes.Buffer
	err := RunBuild(
		context.Background(),
		&out,
		"git_build",
		build,
		"testhost",
		nil,
		BuildOptions{DryRun: true},
	)
	if err != nil {
		t.Fatalf("RunBuild: %v", err)
	}
	if !strings.Contains(out.String(), "was: abc,") {
		t.Errorf("change message should show the short stored hash, got:\n%s", out.String())
	}
}
