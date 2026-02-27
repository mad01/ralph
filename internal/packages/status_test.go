package packages

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/hooks"
)

// testStateDir creates a temp directory and sets HOME to it for isolated testing.
func testStateDir(t *testing.T) (string, func()) {
	t.Helper()
	origHome := os.Getenv("HOME")
	tmpDir, err := os.MkdirTemp("", "ralph-pkg-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	os.Setenv("HOME", tmpDir)
	return tmpDir, func() {
		os.Setenv("HOME", origHome)
		os.RemoveAll(tmpDir)
	}
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2024-01-01T00:00:00", "GIT_COMMITTER_DATE=2024-01-01T00:00:00")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Logf("git command failed: %s, output: %s", err, output)
	}
}

func saveBuildState(t *testing.T, tmpDir string, state *hooks.BuildState) {
	t.Helper()
	stateDir := filepath.Join(tmpDir, ".config", "ralph")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, ".builds_state"), data, 0644); err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}
}

func initGitRepo(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test")

	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("test content"), 0644)
	runGitCmd(t, dir, "add", ".")
	runGitCmd(t, dir, "commit", "-m", "initial")

	hash := hooks.GetGitHash(dir)
	if hash == "" {
		t.Skip("git not available or repo setup failed")
	}
	return hash
}

func TestCheckPackageStatuses_EmptyPackages(t *testing.T) {
	result := CheckPackageStatuses(nil, "", "testhost")
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}

	result = CheckPackageStatuses(map[string]config.Package{}, "", "testhost")
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestCheckPackageStatuses_DisabledPackage(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	enabled := false
	pkgs := map[string]config.Package{
		"disabled_pkg": {
			Source:     "local",
			WorkingDir: "/some/path",
			Enable:     &enabled,
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost")
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if s.Enabled {
		t.Error("expected Enabled=false")
	}
	if s.NeedsBuild {
		t.Error("expected NeedsBuild=false for disabled package")
	}
}

func TestCheckPackageStatuses_HostFiltered(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	pkgs := map[string]config.Package{
		"filtered_pkg": {
			Source:     "local",
			WorkingDir: "/some/path",
			Hosts:      []string{"otherhost"},
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "myhost")
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if !s.Enabled {
		t.Error("expected Enabled=true")
	}
	if s.HostMatch {
		t.Error("expected HostMatch=false")
	}
	if s.NeedsBuild {
		t.Error("expected NeedsBuild=false for host-filtered package")
	}
}

func TestCheckPackageStatuses_LocalNeverBuilt(t *testing.T) {
	tmpDir, cleanup := testStateDir(t)
	defer cleanup()

	workDir := filepath.Join(tmpDir, "local_pkg")
	initGitRepo(t, workDir)

	pkgs := map[string]config.Package{
		"local_pkg": {
			Source:     "local",
			WorkingDir: workDir,
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost")
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if !s.NeedsBuild {
		t.Error("expected NeedsBuild=true for never-built package")
	}
	if s.NeedReason != "never built" {
		t.Errorf("expected reason 'never built', got '%s'", s.NeedReason)
	}
	if !s.Cloned {
		t.Error("expected Cloned=true")
	}
}

func TestCheckPackageStatuses_LocalUpToDate(t *testing.T) {
	tmpDir, cleanup := testStateDir(t)
	defer cleanup()

	workDir := filepath.Join(tmpDir, "local_pkg")
	hash := initGitRepo(t, workDir)

	saveBuildState(t, tmpDir, &hooks.BuildState{
		Builds: map[string]hooks.BuildRecord{
			"pkg:local_pkg": {
				CompletedAt: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
				GitHash:     hash,
			},
		},
	})

	pkgs := map[string]config.Package{
		"local_pkg": {
			Source:     "local",
			WorkingDir: workDir,
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost")
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if s.NeedsBuild {
		t.Errorf("expected NeedsBuild=false for up-to-date package, got reason: %s", s.NeedReason)
	}
	if s.LastBuiltAt == nil {
		t.Error("expected LastBuiltAt to be set")
	}
	if s.CurrentHash != hash {
		t.Errorf("expected CurrentHash=%s, got %s", hash, s.CurrentHash)
	}
}

func TestCheckPackageStatuses_LocalHashChanged(t *testing.T) {
	tmpDir, cleanup := testStateDir(t)
	defer cleanup()

	workDir := filepath.Join(tmpDir, "local_pkg")
	initGitRepo(t, workDir)

	saveBuildState(t, tmpDir, &hooks.BuildState{
		Builds: map[string]hooks.BuildRecord{
			"pkg:local_pkg": {
				CompletedAt: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
				GitHash:     "oldhash123456789",
			},
		},
	})

	pkgs := map[string]config.Package{
		"local_pkg": {
			Source:     "local",
			WorkingDir: workDir,
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost")
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if !s.NeedsBuild {
		t.Error("expected NeedsBuild=true for hash-changed package")
	}
	if s.NeedReason != "git hash changed" {
		t.Errorf("expected reason 'git hash changed', got '%s'", s.NeedReason)
	}
}

func TestCheckPackageStatuses_LocalUncommittedChanges(t *testing.T) {
	tmpDir, cleanup := testStateDir(t)
	defer cleanup()

	workDir := filepath.Join(tmpDir, "local_pkg")
	hash := initGitRepo(t, workDir)

	// Add uncommitted changes
	os.WriteFile(filepath.Join(workDir, "test.txt"), []byte("modified"), 0644)

	saveBuildState(t, tmpDir, &hooks.BuildState{
		Builds: map[string]hooks.BuildRecord{
			"pkg:local_pkg": {
				CompletedAt: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
				GitHash:     hash,
			},
		},
	})

	pkgs := map[string]config.Package{
		"local_pkg": {
			Source:     "local",
			WorkingDir: workDir,
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost")
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if !s.NeedsBuild {
		t.Error("expected NeedsBuild=true for package with uncommitted changes")
	}
	if s.NeedReason != "uncommitted changes" {
		t.Errorf("expected reason 'uncommitted changes', got '%s'", s.NeedReason)
	}
}

func TestCheckPackageStatuses_LocalMissingWorkingDir(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	pkgs := map[string]config.Package{
		"missing_pkg": {
			Source:     "local",
			WorkingDir: "/nonexistent/path",
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost")
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if s.Cloned {
		t.Error("expected Cloned=false for missing working dir")
	}
	if s.NeedReason != "working_dir missing" {
		t.Errorf("expected reason 'working_dir missing', got '%s'", s.NeedReason)
	}
}

func TestCheckPackageStatuses_RemoteNotCloned(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	pkgs := map[string]config.Package{
		"remote_pkg": {
			Source: "remote",
			Repo:   "https://github.com/example/repo.git",
			Target: "/nonexistent/target",
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost")
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if s.Cloned {
		t.Error("expected Cloned=false for not-cloned remote package")
	}
	if !s.NeedsBuild {
		t.Error("expected NeedsBuild=true for not-cloned remote package")
	}
	if s.NeedReason != "not cloned" {
		t.Errorf("expected reason 'not cloned', got '%s'", s.NeedReason)
	}
}

func TestCheckPackageStatuses_RemoteClonedNeverBuilt(t *testing.T) {
	tmpDir, cleanup := testStateDir(t)
	defer cleanup()

	target := filepath.Join(tmpDir, "remote_pkg")
	initGitRepo(t, target)

	pkgs := map[string]config.Package{
		"remote_pkg": {
			Source: "remote",
			Repo:   "https://github.com/example/repo.git",
			Target: target,
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost")
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if !s.Cloned {
		t.Error("expected Cloned=true")
	}
	if !s.NeedsBuild {
		t.Error("expected NeedsBuild=true for never-built remote package")
	}
	if s.NeedReason != "never built" {
		t.Errorf("expected reason 'never built', got '%s'", s.NeedReason)
	}
}

func TestCheckPackageStatuses_RemoteUpToDate(t *testing.T) {
	tmpDir, cleanup := testStateDir(t)
	defer cleanup()

	target := filepath.Join(tmpDir, "remote_pkg")
	hash := initGitRepo(t, target)

	saveBuildState(t, tmpDir, &hooks.BuildState{
		Builds: map[string]hooks.BuildRecord{
			"pkg:remote_pkg": {
				CompletedAt: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
				GitHash:     hash,
			},
		},
	})

	pkgs := map[string]config.Package{
		"remote_pkg": {
			Source: "remote",
			Repo:   "https://github.com/example/repo.git",
			Target: target,
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost")
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if s.NeedsBuild {
		t.Errorf("expected NeedsBuild=false for up-to-date remote package, got reason: %s", s.NeedReason)
	}
	if s.LastBuiltAt == nil {
		t.Error("expected LastBuiltAt to be set")
	}
}

func TestCheckPackageStatuses_SortedAlphabetically(t *testing.T) {
	_, cleanup := testStateDir(t)
	defer cleanup()

	pkgs := map[string]config.Package{
		"zebra": {Source: "local", WorkingDir: "/tmp/z"},
		"alpha": {Source: "local", WorkingDir: "/tmp/a"},
		"mango": {Source: "local", WorkingDir: "/tmp/m"},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost")
	if len(statuses) != 3 {
		t.Fatalf("expected 3 statuses, got %d", len(statuses))
	}
	if statuses[0].Name != "alpha" {
		t.Errorf("expected first item 'alpha', got '%s'", statuses[0].Name)
	}
	if statuses[1].Name != "mango" {
		t.Errorf("expected second item 'mango', got '%s'", statuses[1].Name)
	}
	if statuses[2].Name != "zebra" {
		t.Errorf("expected third item 'zebra', got '%s'", statuses[2].Name)
	}
}
