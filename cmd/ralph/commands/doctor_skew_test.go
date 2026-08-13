package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/gitutil"
	"github.com/mad01/ralph/internal/testutil"
)

// packageSkew is doctor's staleness verdict for a built package. These tests
// pin the three skew classes (poisoned record, moved subtree, outdated
// installed binary) and the fresh/no-verdict paths.

// commitChange writes a file and commits it, returning the new commit hash.
func commitChange(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.RunGitCmd(t, dir, "add", ".")
	testutil.RunGitCmd(t, dir, "commit", "-m", "change "+name)
	return gitutil.GetGitHash(dir)
}

func TestPackageSkew_EmptyRecordedHash(t *testing.T) {
	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)

	msg, skewed := packageSkew(dir, "", "", false)
	if !skewed || !strings.Contains(msg, "no recorded source hash") {
		t.Errorf("packageSkew with empty recorded hash = (%q, %v), want poisoned-record warning", msg, skewed)
	}
}

func TestPackageSkew_SourceChanged(t *testing.T) {
	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)
	staleHash := gitutil.GetTreeHash(dir)
	commitChange(t, dir, "new.txt", "v2")

	msg, skewed := packageSkew(dir, staleHash, "", false)
	if !skewed || !strings.Contains(msg, "source changed since last build") {
		t.Errorf("packageSkew with moved subtree = (%q, %v), want source-changed warning", msg, skewed)
	}
}

func TestPackageSkew_BinaryPredatesChanges(t *testing.T) {
	dir := t.TempDir()
	oldCommit := testutil.InitGitRepo(t, dir)
	commitChange(t, dir, "new.txt", "v2")

	// State is healthy (recorded hash matches HEAD) but the installed binary
	// reports a commit from before the subtree changed.
	msg, skewed := packageSkew(dir, gitutil.GetTreeHash(dir), oldCommit, true)
	if !skewed || !strings.Contains(msg, "predates source changes") {
		t.Errorf("packageSkew with outdated binary = (%q, %v), want binary-skew warning", msg, skewed)
	}
}

func TestPackageSkew_Fresh(t *testing.T) {
	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)
	head := gitutil.GetGitHash(dir)

	if msg, skewed := packageSkew(dir, gitutil.GetTreeHash(dir), head, true); skewed {
		t.Errorf("packageSkew on a fresh package = (%q, %v), want no skew", msg, skewed)
	}
}

func TestPackageSkew_NoVerdictOutsideGit(t *testing.T) {
	if msg, skewed := packageSkew(t.TempDir(), "somehash", "", false); skewed {
		t.Errorf("packageSkew outside a git repo = (%q, %v), want no verdict", msg, skewed)
	}
	if msg, skewed := packageSkew("", "somehash", "", false); skewed {
		t.Errorf("packageSkew with empty workDir = (%q, %v), want no verdict", msg, skewed)
	}
}
