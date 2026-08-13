package packages

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/buildstate"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/testutil"
)

// A build record can be saved with an empty git_hash when GetTreeHash fails at
// save time (transient git breakage). The change check used to require BOTH
// hashes non-empty before comparing, so such a record could never trigger a
// git-change rebuild again — the package froze on its stale binary through
// every subsequent `ralph up`. These tests pin the repaired behavior: an empty
// recorded hash rebuilds once (re-recording a real hash), while matching
// hashes and non-git working dirs keep their existing semantics.

// writeFile creates path (and its parent dirs) with the given content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

// treeHash returns HEAD^{tree} of the repo at dir, matching what
// gitutil.GetTreeHash computes for a repo-root working dir.
func treeHash(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD^{tree}")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestBuildPackage_RebuildsWhenRecordedHashEmpty(t *testing.T) {
	tmpDir := testutil.WithHome(t)
	workDir := filepath.Join(tmpDir, "frozen_pkg")
	testutil.InitGitRepo(t, workDir)

	// Poisoned record: exists, but carries no source hash. The installed
	// binary is present so the install_path self-heal cannot mask the check.
	testutil.SaveBuildStateJSON(t, tmpDir, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{
			"pkg:frozen_pkg": {CompletedAt: time.Now(), GitHash: ""},
		},
	})
	binPath := filepath.Join(tmpDir, "code", "bin", "frozen_tool")
	writeFile(t, binPath, "stale")

	pkg := config.Package{
		Source:       "local",
		WorkingDir:   workDir,
		Build:        []string{"true"},
		Install:      []string{"true"},
		InstallPaths: []string{binPath},
	}

	var buf bytes.Buffer
	result := BuildPackage(context.Background(), &buf, "frozen_pkg", pkg, BuildOptions{})
	if result.Action != "built" {
		t.Fatalf(
			"expected rebuild for record with empty git_hash, got action=%s (msg=%s err=%v)\n%s",
			result.Action,
			result.Message,
			result.Err,
			buf.String(),
		)
	}

	// The rebuild must repair the record so the freeze cannot recur.
	state, err := buildstate.LoadBuildState()
	if err != nil {
		t.Fatalf("load build state: %v", err)
	}
	if got := state.Builds["pkg:frozen_pkg"].GitHash; got == "" {
		t.Error("expected rebuild to record a non-empty git_hash")
	}
}

func TestBuildPackage_UpToDateWhenRecordedHashMatches(t *testing.T) {
	tmpDir := testutil.WithHome(t)
	workDir := filepath.Join(tmpDir, "current_pkg")
	testutil.InitGitRepo(t, workDir)

	testutil.SaveBuildStateJSON(t, tmpDir, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{
			"pkg:current_pkg": {CompletedAt: time.Now(), GitHash: treeHash(t, workDir)},
		},
	})
	binPath := filepath.Join(tmpDir, "code", "bin", "current_tool")
	writeFile(t, binPath, "bin")

	pkg := config.Package{
		Source:       "local",
		WorkingDir:   workDir,
		Build:        []string{"echo should-not-run"},
		Install:      []string{"echo should-not-run"},
		InstallPaths: []string{binPath},
	}

	var buf bytes.Buffer
	result := BuildPackage(context.Background(), &buf, "current_pkg", pkg, BuildOptions{})
	if result.Action != "up-to-date" {
		t.Errorf("expected up-to-date when recorded hash matches, got %s\n%s",
			result.Action, buf.String())
	}
}

func TestSavePackageState_WarnsWhenTreeHashUnavailable(t *testing.T) {
	tmpDir := testutil.WithHome(t)
	workDir := filepath.Join(tmpDir, "plain_dir") // not a git repo
	writeFile(t, filepath.Join(workDir, "x"), "x")

	var buf bytes.Buffer
	savePackageState(&buf, "pkg:plain", workDir, "")
	if !strings.Contains(buf.String(), "no source hash") {
		t.Errorf("expected a warning about the missing source hash, got: %q", buf.String())
	}
}
