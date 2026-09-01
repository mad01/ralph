package repo

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/testutil"
)

func TestCloneOrUpdateRepo_DryRunClone(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "new-repo")

	repo := config.Repo{
		URL:    "https://github.com/example/repo.git",
		Target: target,
	}

	var buf bytes.Buffer
	err := CloneOrUpdateRepo(&buf, "test", repo, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "DRY RUN") {
		t.Errorf("expected DRY RUN in output, got: %s", buf.String())
	}
}

func TestCloneOrUpdateRepo_ExistingSkips(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	testutil.InitGitRepo(t, repoDir)

	repo := config.Repo{
		URL:    "https://github.com/example/repo.git",
		Target: repoDir,
	}

	var buf bytes.Buffer
	err := CloneOrUpdateRepo(&buf, "test", repo, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "Skipping") {
		t.Errorf("expected 'Skipping' in output for existing repo, got: %s", buf.String())
	}
}

func TestCloneOrUpdateRepo_DryRunPull(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	testutil.InitGitRepo(t, repoDir)

	repo := config.Repo{
		URL:    "https://github.com/example/repo.git",
		Target: repoDir,
		Update: true,
	}

	var buf bytes.Buffer
	err := CloneOrUpdateRepo(&buf, "test", repo, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "DRY RUN") {
		t.Errorf("expected DRY RUN in output, got: %s", buf.String())
	}
}

func TestProcessRepos_EmptyRepos(t *testing.T) {
	var buf bytes.Buffer
	err := ProcessRepos(&buf, nil, "host", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty repos, got: %s", buf.String())
	}
}

func TestProcessRepos_DisabledSkipped(t *testing.T) {
	enabled := false
	repos := map[string]config.Repo{
		"disabled": {
			URL:    "https://example.com/repo.git",
			Target: "/tmp/test",
			Enable: &enabled,
		},
	}

	var buf bytes.Buffer
	err := ProcessRepos(&buf, repos, "host", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "disabled") {
		t.Errorf("expected 'disabled' in output, got: %s", buf.String())
	}
}

func TestProcessRepos_HostFilterSkipped(t *testing.T) {
	repos := map[string]config.Repo{
		"filtered": {
			URL:    "https://example.com/repo.git",
			Target: "/tmp/test",
			Hosts:  []string{"otherhost"},
		},
	}

	var buf bytes.Buffer
	err := ProcessRepos(&buf, repos, "myhost", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "host filter") {
		t.Errorf("expected 'host filter' in output, got: %s", buf.String())
	}
}

func TestProcessRepos_ProfileFilterSkipped(t *testing.T) {
	repos := map[string]config.Repo{
		"filtered": {
			URL:      "https://example.com/repo.git",
			Target:   "/tmp/test",
			Profiles: []string{"work"},
		},
	}

	var buf bytes.Buffer
	err := ProcessRepos(&buf, repos, "myhost", []string{"personal"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "profile filter") {
		t.Errorf("expected 'profile filter' in output, got: %s", buf.String())
	}
}
