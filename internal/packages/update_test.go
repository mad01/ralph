package packages

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/buildstate"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/testutil"
)

func TestSyncPackages_MakeSourceTreatedAsRemote(t *testing.T) {
	tmpDir := testutil.WithHome(t)

	// Create the packages dir
	pkgDir := filepath.Join(tmpDir, "pkg")
	os.MkdirAll(pkgDir, 0755)

	pkgs := map[string]config.Package{
		"make_pkg": {
			Source: "make",
			Repo:   "https://github.com/example/repo.git",
		},
	}

	var buf bytes.Buffer
	results := SyncPackages(context.Background(), &buf, pkgs, pkgDir, "testhost", SyncOptions{DryRun: true, Verbose: true})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// source=make should NOT be skipped as "local package"
	if results[0].Action == "skipped" && strings.Contains(results[0].Message, "local package") {
		t.Errorf("source=make should not be skipped as local; got action=%s message=%s", results[0].Action, results[0].Message)
	}

	// It should attempt a clone (dry-run), so action should be "cloned"
	if results[0].Action != "cloned" {
		t.Errorf("expected action=cloned for dry-run make package, got %s", results[0].Action)
	}
}

func TestResolvePackagePaths_MakeSource(t *testing.T) {
	tmpDir := testutil.WithHome(t)

	pkgDir := filepath.Join(tmpDir, "pkg")

	pkg := config.Package{
		Source: "make",
		Repo:   "https://github.com/example/repo.git",
	}

	resolved := ResolvePackagePaths("test_pkg", pkg, pkgDir)

	expectedTarget := filepath.Join(pkgDir, "test_pkg")
	if resolved.Target != expectedTarget {
		t.Errorf("expected target=%s, got %s", expectedTarget, resolved.Target)
	}

	if resolved.WorkingDir != expectedTarget {
		t.Errorf("expected working_dir=%s, got %s", expectedTarget, resolved.WorkingDir)
	}
}

func TestBuildPackage_MakeSourceDefaultBuild(t *testing.T) {
	tmpDir := testutil.WithHome(t)

	workDir := filepath.Join(tmpDir, "make_pkg")
	testutil.InitGitRepo(t, workDir)

	// Create a Makefile so commands succeed
	makefile := filepath.Join(workDir, "Makefile")
	os.WriteFile(makefile, []byte("build:\n\t@echo built\ninstall:\n\t@echo installed\n"), 0644)

	pkg := config.Package{
		Source:     "make",
		WorkingDir: workDir,
		// Build and Install left empty — should default to ["make build"] and ["make install"]
	}

	var buf bytes.Buffer
	result := BuildPackage(context.Background(), &buf, "make_pkg", pkg, BuildOptions{Force: true})

	if result.Action != "built" {
		t.Errorf("expected action=built, got %s (message: %s, err: %v)", result.Action, result.Message, result.Err)
	}

	output := buf.String()
	if !strings.Contains(output, "make build") {
		t.Errorf("expected output to contain 'make build', got: %s", output)
	}
	if !strings.Contains(output, "make install") {
		t.Errorf("expected output to contain 'make install', got: %s", output)
	}
}

func TestBuildPackage_MakeSourceDefaultInstall(t *testing.T) {
	tmpDir := testutil.WithHome(t)

	workDir := filepath.Join(tmpDir, "make_pkg2")
	testutil.InitGitRepo(t, workDir)

	makefile := filepath.Join(workDir, "Makefile")
	os.WriteFile(makefile, []byte("build:\n\t@echo built\ninstall:\n\t@echo installed\n"), 0644)

	pkg := config.Package{
		Source:     "make",
		WorkingDir: workDir,
		Build:      []string{"echo custom-build"},
		// Install left empty — should default to ["make install"]
	}

	var buf bytes.Buffer
	result := BuildPackage(context.Background(), &buf, "make_pkg2", pkg, BuildOptions{Force: true})

	if result.Action != "built" {
		t.Errorf("expected action=built, got %s (message: %s, err: %v)", result.Action, result.Message, result.Err)
	}

	output := buf.String()
	if !strings.Contains(output, "echo custom-build") {
		t.Errorf("expected output to contain 'echo custom-build', got: %s", output)
	}
	if !strings.Contains(output, "make install") {
		t.Errorf("expected output to contain 'make install', got: %s", output)
	}
}

func TestBuildPackage_MakeSourceExplicitBuildOverridesDefault(t *testing.T) {
	tmpDir := testutil.WithHome(t)

	workDir := filepath.Join(tmpDir, "make_pkg3")
	testutil.InitGitRepo(t, workDir)

	pkg := config.Package{
		Source:     "make",
		WorkingDir: workDir,
		Build:      []string{"echo custom-build"},
		Install:    []string{"echo custom-install"},
	}

	var buf bytes.Buffer
	result := BuildPackage(context.Background(), &buf, "make_pkg3", pkg, BuildOptions{Force: true})

	if result.Action != "built" {
		t.Errorf("expected action=built, got %s (message: %s, err: %v)", result.Action, result.Message, result.Err)
	}

	output := buf.String()
	if !strings.Contains(output, "echo custom-build") {
		t.Errorf("expected output to contain 'echo custom-build', got: %s", output)
	}
	if !strings.Contains(output, "echo custom-install") {
		t.Errorf("expected output to contain 'echo custom-install', got: %s", output)
	}
	if strings.Contains(output, "make build") {
		t.Errorf("should NOT contain default 'make build' when explicit build is provided, got: %s", output)
	}
	if strings.Contains(output, "make install") {
		t.Errorf("should NOT contain default 'make install' when explicit install is provided, got: %s", output)
	}
}

// --- Tests for source=go-install packages ---

func TestMaybeRestartService(t *testing.T) {
	svc := func(cmd string) *config.Service { return &config.Service{Restart: cmd} }

	t.Run("no service block → no restart", func(t *testing.T) {
		var buf bytes.Buffer
		if maybeRestartService(context.Background(), &buf, "p", config.Package{}, "old", "new", BuildOptions{}) {
			t.Error("expected no restart when Service is nil")
		}
	})

	t.Run("unhashable binary (empty newHash) → no restart", func(t *testing.T) {
		var buf bytes.Buffer
		pkg := config.Package{Service: svc("true")}
		if maybeRestartService(context.Background(), &buf, "p", pkg, "old", "", BuildOptions{}) {
			t.Error("expected no restart when newHash is empty")
		}
	})

	t.Run("unchanged binary → no restart", func(t *testing.T) {
		var buf bytes.Buffer
		pkg := config.Package{Service: svc("true")}
		if maybeRestartService(context.Background(), &buf, "p", pkg, "same", "same", BuildOptions{}) {
			t.Error("expected no restart when hash is unchanged")
		}
	})

	t.Run("changed binary → restart runs", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "restarted")
		var buf bytes.Buffer
		pkg := config.Package{Service: svc("touch " + marker)}
		if !maybeRestartService(context.Background(), &buf, "p", pkg, "old", "new", BuildOptions{}) {
			t.Fatal("expected restart to run when hash changed")
		}
		if _, err := os.Stat(marker); err != nil {
			t.Errorf("restart command did not run: %v", err)
		}
	})

	t.Run("first build (empty prevHash) → restart runs", func(t *testing.T) {
		var buf bytes.Buffer
		pkg := config.Package{Service: svc("true")}
		if !maybeRestartService(context.Background(), &buf, "p", pkg, "", "new", BuildOptions{}) {
			t.Error("expected restart on first build (no prior hash)")
		}
	})

	t.Run("failing restart command → best-effort false, no panic", func(t *testing.T) {
		var buf bytes.Buffer
		pkg := config.Package{Service: svc("exit 7")}
		if maybeRestartService(context.Background(), &buf, "p", pkg, "old", "new", BuildOptions{}) {
			t.Error("expected false when restart command fails")
		}
	})

	t.Run("dry-run → previews, does not run", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "should-not-exist")
		var buf bytes.Buffer
		pkg := config.Package{Service: svc("touch " + marker)}
		if maybeRestartService(context.Background(), &buf, "p", pkg, "old", "new", BuildOptions{DryRun: true}) {
			t.Error("expected no restart in dry-run")
		}
		if _, err := os.Stat(marker); err == nil {
			t.Error("dry-run must not execute the restart command")
		}
		if !strings.Contains(buf.String(), "would restart service") {
			t.Errorf("expected dry-run preview, got: %s", buf.String())
		}
	})
}

func TestBuildPackage_ServiceRestartOnBinaryChange(t *testing.T) {
	tmpDir := testutil.WithHome(t)
	workDir := filepath.Join(tmpDir, "svc_pkg")
	testutil.InitGitRepo(t, workDir)

	binDir := filepath.Join(tmpDir, "code", "bin")
	marker := filepath.Join(tmpDir, "restart.count")

	// Build writes the "binary"; the content is read from a file we control so
	// we can produce identical vs. changed installs across runs.
	contentFile := filepath.Join(workDir, "VERSION")
	os.WriteFile(contentFile, []byte("v1"), 0644)
	mk := "build:\n\t@mkdir -p " + binDir + " && cp VERSION " + filepath.Join(binDir, "svc_tool") +
		"\ninstall:\n\t@true\n"
	os.WriteFile(filepath.Join(workDir, "Makefile"), []byte(mk), 0644)

	pkg := config.Package{
		Source:       "make",
		WorkingDir:   workDir,
		InstallPaths: []string{filepath.Join(binDir, "svc_tool")},
		Service:      &config.Service{Restart: "printf x >> " + marker},
	}

	// First build: binary is new → restart fires.
	var buf bytes.Buffer
	r := BuildPackage(context.Background(), &buf, "svc_pkg", pkg, BuildOptions{Force: true})
	if r.Action != "built" || !r.ServiceRestarted {
		t.Fatalf("first build: action=%s restarted=%v, want built/true (msg=%s err=%v)", r.Action, r.ServiceRestarted, r.Message, r.Err)
	}
	if got := markerLen(t, marker); got != 1 {
		t.Fatalf("expected 1 restart after first build, got %d", got)
	}

	// Second build, identical content → byte-identical install → NO restart.
	r = BuildPackage(context.Background(), &buf, "svc_pkg", pkg, BuildOptions{Force: true})
	if r.ServiceRestarted {
		t.Error("byte-identical rebuild must not restart the service")
	}
	if got := markerLen(t, marker); got != 1 {
		t.Errorf("expected still 1 restart after identical rebuild, got %d", got)
	}

	// Third build, changed content → restart fires again.
	os.WriteFile(contentFile, []byte("v2"), 0644)
	r = BuildPackage(context.Background(), &buf, "svc_pkg", pkg, BuildOptions{Force: true})
	if !r.ServiceRestarted {
		t.Error("changed binary must restart the service")
	}
	if got := markerLen(t, marker); got != 2 {
		t.Errorf("expected 2 restarts after changed rebuild, got %d", got)
	}
}

func markerLen(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(data)
}

func TestSyncPackages_GoInstallSkipped(t *testing.T) {
	_ = testutil.WithHome(t)

	pkgs := map[string]config.Package{
		"go_tool": {
			Source:       "go-install",
			Module:       "github.com/example/tool",
			Version:      "v1.0.0",
			InstallPaths: []string{"~/code/bin/tool"},
		},
	}

	var buf bytes.Buffer
	results := SyncPackages(context.Background(), &buf, pkgs, "", "testhost", SyncOptions{Verbose: true})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Action != "skipped" {
		t.Errorf("expected action=skipped for go-install sync, got %s", r.Action)
	}
	if !strings.Contains(r.Message, "go-install") {
		t.Errorf("expected message to mention 'go-install', got: %s", r.Message)
	}
}

func TestResolvePackagePaths_GoInstallSource(t *testing.T) {
	pkg := config.Package{
		Source:       "go-install",
		Module:       "github.com/example/tool",
		Version:      "v1.0.0",
		InstallPaths: []string{"~/code/bin/tool"},
	}

	resolved := ResolvePackagePaths("go_tool", pkg, "")

	// go-install should not set Target or WorkingDir
	if resolved.Target != "" {
		t.Errorf("expected empty target for go-install, got %s", resolved.Target)
	}
	if resolved.WorkingDir != "" {
		t.Errorf("expected empty working_dir for go-install, got %s", resolved.WorkingDir)
	}
}

func TestBuildPackage_GoInstallUpToDate(t *testing.T) {
	tmpDir := testutil.WithHome(t)

	// Pre-populate state with same version
	testutil.SaveBuildStateJSON(t, tmpDir, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{
			"pkg:go_tool": {
				CompletedAt: time.Now(),
				Version:     "v1.0.0",
			},
		},
	})

	pkg := config.Package{
		Source:       "go-install",
		Module:       "github.com/example/tool",
		Version:      "v1.0.0",
		InstallPaths: []string{"~/code/bin/tool"},
	}

	var buf bytes.Buffer
	result := BuildPackage(context.Background(), &buf, "go_tool", pkg, BuildOptions{})

	if result.Action != "up-to-date" {
		t.Errorf("expected action=up-to-date when version matches, got %s (message: %s)", result.Action, result.Message)
	}
}

func TestBuildPackage_GoInstallVersionChanged(t *testing.T) {
	tmpDir := testutil.WithHome(t)

	// Create the install dir so the command can target it
	binDir := filepath.Join(tmpDir, "code", "bin")
	os.MkdirAll(binDir, 0755)

	// Pre-populate state with old version
	testutil.SaveBuildStateJSON(t, tmpDir, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{
			"pkg:go_tool": {
				CompletedAt: time.Now(),
				Version:     "v1.0.0",
			},
		},
	})

	pkg := config.Package{
		Source:       "go-install",
		Module:       "github.com/example/tool",
		Version:      "v2.0.0",
		InstallPaths: []string{filepath.Join(binDir, "tool")},
	}

	var buf bytes.Buffer
	// This will fail because the module doesn't exist, but it should attempt the install
	// (not skip as up-to-date). We check it tries by verifying action != "up-to-date".
	result := BuildPackage(context.Background(), &buf, "go_tool", pkg, BuildOptions{})

	if result.Action == "up-to-date" {
		t.Error("expected go-install to attempt rebuild when version changed, got up-to-date")
	}
}

func TestBuildPackage_GoInstallNeverBuilt(t *testing.T) {
	tmpDir := testutil.WithHome(t)

	binDir := filepath.Join(tmpDir, "code", "bin")
	os.MkdirAll(binDir, 0755)

	pkg := config.Package{
		Source:       "go-install",
		Module:       "github.com/example/tool",
		Version:      "v1.0.0",
		InstallPaths: []string{filepath.Join(binDir, "tool")},
	}

	var buf bytes.Buffer
	result := BuildPackage(context.Background(), &buf, "go_tool", pkg, BuildOptions{})

	// Should attempt to build (not skip), though it will fail because module is fake
	if result.Action == "up-to-date" {
		t.Error("expected go-install to attempt build when never built, got up-to-date")
	}
}

func TestBuildPackage_GoInstallDryRun(t *testing.T) {
	_ = testutil.WithHome(t)

	pkg := config.Package{
		Source:       "go-install",
		Module:       "github.com/example/tool",
		Version:      "v1.0.0",
		InstallPaths: []string{"~/code/bin/tool"},
	}

	var buf bytes.Buffer
	result := BuildPackage(context.Background(), &buf, "go_tool", pkg, BuildOptions{DryRun: true})

	if result.Action != "built" {
		t.Errorf("expected action=built for dry-run, got %s", result.Action)
	}

	output := buf.String()
	if !strings.Contains(output, "DRY RUN") {
		t.Errorf("expected DRY RUN in output, got: %s", output)
	}
	if !strings.Contains(output, "go install") {
		t.Errorf("expected 'go install' in dry-run output, got: %s", output)
	}
}
