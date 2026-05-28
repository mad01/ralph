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
