package packages

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/buildstate"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/gitutil"
	"github.com/mad01/ralph/internal/testutil"
)

func TestCheckPackageStatuses_EmptyPackages(t *testing.T) {
	result := CheckPackageStatuses(nil, "", "testhost", nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}

	result = CheckPackageStatuses(map[string]config.Package{}, "", "testhost", nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestCheckPackageStatuses_DisabledPackage(t *testing.T) {
	_ = testutil.WithHome(t)

	enabled := false
	pkgs := map[string]config.Package{
		"disabled_pkg": {
			Source:     "local",
			WorkingDir: "/some/path",
			Enable:     &enabled,
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost", nil)
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
	_ = testutil.WithHome(t)

	pkgs := map[string]config.Package{
		"filtered_pkg": {
			Source:     "local",
			WorkingDir: "/some/path",
			Hosts:      []string{"otherhost"},
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "myhost", nil)
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

func TestCheckPackageStatuses_ProfileFiltered(t *testing.T) {
	_ = testutil.WithHome(t)

	pkgs := map[string]config.Package{
		"filtered_pkg": {
			Source:     "local",
			WorkingDir: "/some/path",
			Profiles:   []string{"work"},
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "myhost", []string{"personal"})
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if !s.Enabled {
		t.Error("expected Enabled=true")
	}
	if s.ProfileMatch {
		t.Error("expected ProfileMatch=false")
	}
	if s.NeedsBuild {
		t.Error("expected NeedsBuild=false for profile-filtered package")
	}
}

func TestCheckPackageStatuses_LocalNeverBuilt(t *testing.T) {
	tmpDir := testutil.WithHome(t)

	workDir := filepath.Join(tmpDir, "local_pkg")
	testutil.InitGitRepo(t, workDir)

	pkgs := map[string]config.Package{
		"local_pkg": {
			Source:     "local",
			WorkingDir: workDir,
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost", nil)
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
	tmpDir := testutil.WithHome(t)

	workDir := filepath.Join(tmpDir, "local_pkg")
	testutil.InitGitRepo(t, workDir)
	treeHash := gitutil.GetTreeHash(workDir)

	testutil.SaveBuildStateJSON(t, tmpDir, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{
			"pkg:local_pkg": {
				CompletedAt: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
				GitHash:     treeHash,
			},
		},
	})

	pkgs := map[string]config.Package{
		"local_pkg": {
			Source:     "local",
			WorkingDir: workDir,
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost", nil)
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
	if s.CurrentHash != treeHash {
		t.Errorf("expected CurrentHash=%s, got %s", treeHash, s.CurrentHash)
	}
}

func TestCheckPackageStatuses_LocalHashChanged(t *testing.T) {
	tmpDir := testutil.WithHome(t)

	workDir := filepath.Join(tmpDir, "local_pkg")
	testutil.InitGitRepo(t, workDir)

	testutil.SaveBuildStateJSON(t, tmpDir, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{
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

	statuses := CheckPackageStatuses(pkgs, "", "testhost", nil)
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
	tmpDir := testutil.WithHome(t)

	workDir := filepath.Join(tmpDir, "local_pkg")
	testutil.InitGitRepo(t, workDir)
	// Record the committed tree hash; the uncommitted write below does not
	// change HEAD's tree, so the "uncommitted changes" rail is what must fire.
	treeHash := gitutil.GetTreeHash(workDir)

	// Add uncommitted changes
	_ = os.WriteFile(filepath.Join(workDir, "test.txt"), []byte("modified"), 0o644)

	testutil.SaveBuildStateJSON(t, tmpDir, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{
			"pkg:local_pkg": {
				CompletedAt: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
				GitHash:     treeHash,
			},
		},
	})

	pkgs := map[string]config.Package{
		"local_pkg": {
			Source:     "local",
			WorkingDir: workDir,
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost", nil)
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
	_ = testutil.WithHome(t)

	pkgs := map[string]config.Package{
		"missing_pkg": {
			Source:     "local",
			WorkingDir: "/nonexistent/path",
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost", nil)
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
	_ = testutil.WithHome(t)

	pkgs := map[string]config.Package{
		"remote_pkg": {
			Source: "remote",
			Repo:   "https://github.com/example/repo.git",
			Target: "/nonexistent/target",
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost", nil)
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
	tmpDir := testutil.WithHome(t)

	target := filepath.Join(tmpDir, "remote_pkg")
	testutil.InitGitRepo(t, target)

	pkgs := map[string]config.Package{
		"remote_pkg": {
			Source: "remote",
			Repo:   "https://github.com/example/repo.git",
			Target: target,
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost", nil)
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
	tmpDir := testutil.WithHome(t)

	target := filepath.Join(tmpDir, "remote_pkg")
	testutil.InitGitRepo(t, target)
	treeHash := gitutil.GetTreeHash(target)

	testutil.SaveBuildStateJSON(t, tmpDir, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{
			"pkg:remote_pkg": {
				CompletedAt: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
				GitHash:     treeHash,
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

	statuses := CheckPackageStatuses(pkgs, "", "testhost", nil)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if s.NeedsBuild {
		t.Errorf(
			"expected NeedsBuild=false for up-to-date remote package, got reason: %s",
			s.NeedReason,
		)
	}
	if s.LastBuiltAt == nil {
		t.Error("expected LastBuiltAt to be set")
	}
}

func TestCheckPackageStatuses_MakeSourceNotCloned(t *testing.T) {
	_ = testutil.WithHome(t)

	pkgs := map[string]config.Package{
		"make_pkg": {
			Source: "make",
			Repo:   "https://github.com/example/repo.git",
			Target: "/nonexistent/target",
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost", nil)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if s.Cloned {
		t.Error("expected Cloned=false for not-cloned make package")
	}
	if !s.NeedsBuild {
		t.Error("expected NeedsBuild=true for not-cloned make package")
	}
	if s.NeedReason != "not cloned" {
		t.Errorf("expected reason 'not cloned', got '%s'", s.NeedReason)
	}
}

func TestCheckPackageStatuses_MakeSourceClonedNeverBuilt(t *testing.T) {
	tmpDir := testutil.WithHome(t)

	target := filepath.Join(tmpDir, "make_pkg")
	testutil.InitGitRepo(t, target)

	pkgs := map[string]config.Package{
		"make_pkg": {
			Source: "make",
			Repo:   "https://github.com/example/repo.git",
			Target: target,
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost", nil)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if !s.Cloned {
		t.Error("expected Cloned=true")
	}
	if !s.NeedsBuild {
		t.Error("expected NeedsBuild=true for never-built make package")
	}
	if s.NeedReason != "never built" {
		t.Errorf("expected reason 'never built', got '%s'", s.NeedReason)
	}
}

func TestCheckPackageStatuses_GoInstallNeverBuilt(t *testing.T) {
	_ = testutil.WithHome(t)

	pkgs := map[string]config.Package{
		"go_tool": {
			Source:       "go-install",
			Module:       "github.com/example/tool",
			Version:      "v1.0.0",
			InstallPaths: []string{"~/code/bin/tool"},
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost", nil)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if s.Source != "go-install" {
		t.Errorf("expected source=go-install, got %s", s.Source)
	}
	if !s.NeedsBuild {
		t.Error("expected NeedsBuild=true for never-built go-install package")
	}
	if s.NeedReason != "never installed" {
		t.Errorf("expected reason 'never installed', got '%s'", s.NeedReason)
	}
}

func TestCheckPackageStatuses_GoInstallUpToDate(t *testing.T) {
	tmpDir := testutil.WithHome(t)
	binPath := filepath.Join(tmpDir, "code", "bin", "tool")
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binPath, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	installHash, err := buildstate.ComputeInstallHash([]string{binPath})
	if err != nil {
		t.Fatal(err)
	}

	testutil.SaveBuildStateJSON(t, tmpDir, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{
			"pkg:go_tool": {
				CompletedAt: time.Now(),
				Version:     "v1.0.0",
				InstallHash: installHash,
			},
		},
	})

	pkgs := map[string]config.Package{
		"go_tool": {
			Source:       "go-install",
			Module:       "github.com/example/tool",
			Version:      "v1.0.0",
			InstallPaths: []string{binPath},
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost", nil)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if s.NeedsBuild {
		t.Errorf(
			"expected NeedsBuild=false for up-to-date go-install, got reason: %s",
			s.NeedReason,
		)
	}
}

func TestCheckPackageStatuses_GoInstallVersionChanged(t *testing.T) {
	tmpDir := testutil.WithHome(t)

	testutil.SaveBuildStateJSON(t, tmpDir, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{
			"pkg:go_tool": {
				CompletedAt: time.Now(),
				Version:     "v1.0.0",
			},
		},
	})

	pkgs := map[string]config.Package{
		"go_tool": {
			Source:       "go-install",
			Module:       "github.com/example/tool",
			Version:      "v2.0.0", // Different version
			InstallPaths: []string{"~/code/bin/tool"},
		},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost", nil)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if !s.NeedsBuild {
		t.Error("expected NeedsBuild=true for version-changed go-install package")
	}
	if s.NeedReason != "version changed" {
		t.Errorf("expected reason 'version changed', got '%s'", s.NeedReason)
	}
}

func TestCheckPackageStatuses_SortedAlphabetically(t *testing.T) {
	_ = testutil.WithHome(t)

	pkgs := map[string]config.Package{
		"zebra": {Source: "local", WorkingDir: "/tmp/z"},
		"alpha": {Source: "local", WorkingDir: "/tmp/a"},
		"mango": {Source: "local", WorkingDir: "/tmp/m"},
	}

	statuses := CheckPackageStatuses(pkgs, "", "testhost", nil)
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
