package gitutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/ralph/internal/testutil"
)

func TestGetGitHash_ReturnsHash(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	hash := testutil.InitGitRepo(t, repoDir)

	got := GetGitHash(repoDir)
	if got != hash {
		t.Errorf("GetGitHash() = %q, want %q", got, hash)
	}
	if len(got) != 40 {
		t.Errorf("expected 40-char hash, got %d chars", len(got))
	}
}

func TestGetGitHash_NonGitDir_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	got := GetGitHash(dir)
	if got != "" {
		t.Errorf("GetGitHash(non-git) = %q, want empty", got)
	}
}

func TestHasGitChanges_CleanRepo(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	testutil.InitGitRepo(t, repoDir)

	if HasGitChanges(repoDir) {
		t.Error("expected no changes in clean repo")
	}
}

func TestHasGitChanges_DirtyRepo(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	testutil.InitGitRepo(t, repoDir)

	os.WriteFile(filepath.Join(repoDir, "test.txt"), []byte("modified"), 0644)

	if !HasGitChanges(repoDir) {
		t.Error("expected changes after modifying tracked file")
	}
}

func TestHasGitChanges_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	if HasGitChanges(dir) {
		t.Error("expected false for non-git directory")
	}
}

// writeAndCommit writes content to relPath under repoDir and commits it.
func writeAndCommit(t *testing.T, repoDir, relPath, content, msg string) {
	t.Helper()
	full := filepath.Join(repoDir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	testutil.RunGitCmd(t, repoDir, "add", "-A")
	testutil.RunGitCmd(t, repoDir, "commit", "-m", msg)
}

func TestGetTreeHash_SubdirUnchangedAcrossUnrelatedCommit(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	testutil.InitGitRepo(t, repoDir)

	toolDir := filepath.Join(repoDir, "tools", "mytool")
	writeAndCommit(t, repoDir, "tools/mytool/main.go", "package main", "add tool")

	rootBefore := GetTreeHash(repoDir)
	toolBefore := GetTreeHash(toolDir)
	if rootBefore == "" || toolBefore == "" {
		t.Fatalf("expected non-empty tree hashes, got root=%q tool=%q", rootBefore, toolBefore)
	}
	if rootBefore == toolBefore {
		t.Errorf("root and subdir tree hashes should differ, both = %q", rootBefore)
	}

	// Commit a change OUTSIDE the tool subdir.
	writeAndCommit(t, repoDir, "README.md", "unrelated change", "unrelated")

	rootAfter := GetTreeHash(repoDir)
	toolAfter := GetTreeHash(toolDir)

	if rootAfter == rootBefore {
		t.Error("root tree hash should change after an unrelated commit")
	}
	if toolAfter != toolBefore {
		t.Errorf("subdir tree hash should be unchanged by an unrelated commit: before=%q after=%q", toolBefore, toolAfter)
	}
}

func TestGetTreeHash_SubdirChangesWhenSubdirChanges(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	testutil.InitGitRepo(t, repoDir)

	toolDir := filepath.Join(repoDir, "tools", "mytool")
	writeAndCommit(t, repoDir, "tools/mytool/main.go", "package main", "add tool")
	before := GetTreeHash(toolDir)

	writeAndCommit(t, repoDir, "tools/mytool/main.go", "package main // v2", "edit tool")
	after := GetTreeHash(toolDir)

	if after == before {
		t.Errorf("subdir tree hash should change when the subdir changes: before=%q after=%q", before, after)
	}
}

func TestGetTreeHash_NonGitDir_ReturnsEmpty(t *testing.T) {
	if got := GetTreeHash(t.TempDir()); got != "" {
		t.Errorf("GetTreeHash(non-git) = %q, want empty", got)
	}
}

func TestHasGitChangesInPath_ScopedToSubdir(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	testutil.InitGitRepo(t, repoDir)
	toolDir := filepath.Join(repoDir, "tools", "mytool")
	writeAndCommit(t, repoDir, "tools/mytool/main.go", "package main", "add tool")

	if HasGitChangesInPath(toolDir) {
		t.Error("expected clean subdir to report no changes")
	}

	// Modify a tracked file OUTSIDE the subdir.
	if err := os.WriteFile(filepath.Join(repoDir, "test.txt"), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if HasGitChangesInPath(toolDir) {
		t.Error("changes outside the subdir must not count as subdir changes")
	}

	// Modify a tracked file INSIDE the subdir.
	if err := os.WriteFile(filepath.Join(toolDir, "main.go"), []byte("package main // edit"), 0644); err != nil {
		t.Fatal(err)
	}
	if !HasGitChangesInPath(toolDir) {
		t.Error("expected subdir change to be detected")
	}
}

func TestHasGitChangesInPath_IgnoresGitignoredFiles(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	testutil.InitGitRepo(t, repoDir)
	toolDir := filepath.Join(repoDir, "tools", "mytool")
	writeAndCommit(t, repoDir, "tools/mytool/main.go", "package main", "add tool")
	writeAndCommit(t, repoDir, "tools/mytool/.gitignore", "bin/\n", "ignore build output")

	// A gitignored build artifact must NOT register as a change.
	if err := os.MkdirAll(filepath.Join(toolDir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "bin", "mytool"), []byte("ELF..."), 0755); err != nil {
		t.Fatal(err)
	}
	if HasGitChangesInPath(toolDir) {
		t.Error("gitignored build output must not count as a subdir change")
	}
}

func TestIsSafeRemoteURL(t *testing.T) {
	cases := map[string]bool{
		"https://github.com/x/y.git": true,
		"git@github.com:x/y.git":     true,
		"file:///home/user/repo":     true,
		"":                           false,
		"-oProxyCommand=evil":        false,
		"ext::sh -c 'rm -rf ~'":      false,
		"EXT::sh -c whoami":          false,
		"fd::17/foo":                 false,
	}
	for url, want := range cases {
		if got := IsSafeRemoteURL(url); got != want {
			t.Errorf("IsSafeRemoteURL(%q) = %v, want %v", url, got, want)
		}
	}
}

func TestIsSafeGitRef(t *testing.T) {
	if !IsSafeGitRef("main") || !IsSafeGitRef("v1.2.3") || !IsSafeGitRef("") {
		t.Error("expected normal refs to be safe")
	}
	if IsSafeGitRef("--upload-pack=evil") || IsSafeGitRef("-x") {
		t.Error("expected option-like refs to be rejected")
	}
}
