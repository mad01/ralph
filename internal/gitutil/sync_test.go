package gitutil

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// makeFixtureRepo creates a local git repo with two commits, a tag "v1.0.0"
// on the first commit, and a branch "feature" on the second. Returns the repo
// path and the two commit hashes.
func makeFixtureRepo(t *testing.T) (repoPath, first, second string) {
	t.Helper()
	repoPath = t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoPath
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repoPath, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "first")
	first = GetGitHash(repoPath)
	run("tag", "v1.0.0")

	if err := os.WriteFile(filepath.Join(repoPath, "b.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "second")
	second = GetGitHash(repoPath)
	run("branch", "feature")

	return repoPath, first, second
}

func TestCloneDefaultBranch(t *testing.T) {
	src, _, second := makeFixtureRepo(t)
	target := filepath.Join(t.TempDir(), "clone")

	if err := Clone(io.Discard, src, target, ""); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	if got := GetGitHash(target); got != second {
		t.Errorf("HEAD = %s, want %s", got, second)
	}
	if got := CurrentBranch(target); got != "main" {
		t.Errorf("branch = %q, want main", got)
	}
}

func TestCloneTag(t *testing.T) {
	src, first, _ := makeFixtureRepo(t)
	target := filepath.Join(t.TempDir(), "clone")

	if err := Clone(io.Discard, src, target, "v1.0.0"); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	if got := GetGitHash(target); got != first {
		t.Errorf("HEAD = %s, want tag commit %s", got, first)
	}
	if got := CurrentBranch(target); got != "" {
		t.Errorf("branch = %q, want detached HEAD", got)
	}
}

func TestCheckoutCommit(t *testing.T) {
	src, first, second := makeFixtureRepo(t)
	target := filepath.Join(t.TempDir(), "clone")

	if err := Clone(io.Discard, src, target, ""); err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	if err := Checkout(io.Discard, target, first); err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
	if got := GetGitHash(target); got != first {
		t.Errorf("HEAD = %s, want %s", got, first)
	}
	if got := CurrentBranch(target); got != "" {
		t.Errorf("branch = %q, want detached HEAD", got)
	}
	if got := GetGitHash(target); got == second {
		t.Error("HEAD still at second commit after checkout")
	}
}

func TestFetchAndPullAdvances(t *testing.T) {
	src, _, _ := makeFixtureRepo(t)
	target := filepath.Join(t.TempDir(), "clone")

	if err := Clone(io.Discard, src, target, ""); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	// Advance the source repo.
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = src
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(src, "c.txt"), []byte("three"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "third")
	third := GetGitHash(src)

	if err := Fetch(io.Discard, target); err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if err := Pull(io.Discard, target); err != nil {
		t.Fatalf("pull failed: %v", err)
	}
	if got := GetGitHash(target); got != third {
		t.Errorf("HEAD = %s, want %s after pull", got, third)
	}
}

func TestResolveRef(t *testing.T) {
	src, first, second := makeFixtureRepo(t)

	if got := ResolveRef(src, "v1.0.0"); got != first {
		t.Errorf("ResolveRef(v1.0.0) = %s, want %s", got, first)
	}
	if got := ResolveRef(src, "main"); got != second {
		t.Errorf("ResolveRef(main) = %s, want %s", got, second)
	}
	if got := ResolveRef(src, "nope"); got != "" {
		t.Errorf("ResolveRef(nope) = %q, want empty", got)
	}
}
