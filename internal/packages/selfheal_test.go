package packages

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/buildstate"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/testutil"
)

// Fix #3 — a normal `ralph up` must rebuild a package whose source is unchanged
// but whose installed binary is missing on disk, so a deleted install_path
// self-heals without needing `--reset-builds`.

func TestFirstMissingInstallPath(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present")
	if err := os.WriteFile(present, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing")

	if p, ok := firstMissingInstallPath(config.Package{InstallPaths: []string{present}}); ok {
		t.Errorf("all present should report none missing, got %q", p)
	}
	if p, ok := firstMissingInstallPath(config.Package{InstallPaths: []string{present, missing}}); !ok ||
		p != missing {
		t.Errorf("expected %q reported missing, got %q ok=%v", missing, p, ok)
	}
	if _, ok := firstMissingInstallPath(config.Package{}); ok {
		t.Error("no install_paths should report none missing")
	}
}

func TestBuildPackage_RebuildsWhenInstallPathMissing(t *testing.T) {
	tmpDir := testutil.WithHome(t)
	workDir := filepath.Join(tmpDir, "heal_pkg")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Saved state with no source change => change-detection alone says up-to-date.
	testutil.SaveBuildStateJSON(t, tmpDir, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{
			"pkg:heal_pkg": {CompletedAt: time.Now()},
		},
	})

	binPath := filepath.Join(tmpDir, "code", "bin", "heal_tool") // does NOT exist
	pkg := config.Package{
		Source:     "local",
		WorkingDir: workDir,
		Build:      []string{"true"},
		Install: []string{
			fmt.Sprintf("mkdir -p %q && touch %q", filepath.Dir(binPath), binPath),
		},
		InstallPaths: []string{binPath},
	}

	var buf bytes.Buffer
	result := BuildPackage(context.Background(), &buf, "heal_pkg", pkg, BuildOptions{})

	if result.Action != "built" {
		t.Fatalf("expected rebuild when install_path missing, got action=%s (msg=%s err=%v)\n%s",
			result.Action, result.Message, result.Err, buf.String())
	}
	if _, err := os.Stat(binPath); err != nil {
		t.Errorf("expected install_path recreated by the rebuild, stat err=%v", err)
	}
}

// Regression guard — when the install_path is present and the source is
// unchanged, the package stays up-to-date (no needless rebuild).
func TestBuildPackage_UpToDateWhenInstallPathPresent(t *testing.T) {
	tmpDir := testutil.WithHome(t)
	workDir := filepath.Join(tmpDir, "present_pkg")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.SaveBuildStateJSON(t, tmpDir, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{
			"pkg:present_pkg": {CompletedAt: time.Now()},
		},
	})

	binPath := filepath.Join(tmpDir, "code", "bin", "present_tool")
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binPath, []byte("bin"), 0o755); err != nil { // EXISTS
		t.Fatal(err)
	}

	pkg := config.Package{
		Source:       "local",
		WorkingDir:   workDir,
		Build:        []string{"echo should-not-run"},
		Install:      []string{"echo should-not-run"},
		InstallPaths: []string{binPath},
	}

	var buf bytes.Buffer
	result := BuildPackage(context.Background(), &buf, "present_pkg", pkg, BuildOptions{})
	if result.Action != "up-to-date" {
		t.Errorf(
			"expected up-to-date when install_path present and source unchanged, got %s\n%s",
			result.Action,
			buf.String(),
		)
	}
}

// go-install path: a matching recorded version must NOT short-circuit to
// up-to-date when the installed binary is missing.
func TestBuildPackage_GoInstallReinstallsWhenInstallPathMissing(t *testing.T) {
	tmpDir := testutil.WithHome(t)
	t.Setenv("GOPROXY", "off") // fail fast offline instead of reaching the network

	testutil.SaveBuildStateJSON(t, tmpDir, &buildstate.BuildState{
		Builds: map[string]buildstate.BuildRecord{
			"pkg:go_tool": {CompletedAt: time.Now(), Version: "v1.0.0"},
		},
	})

	pkg := config.Package{
		Source:       "go-install",
		Module:       "github.com/example/doesnotexist",
		Version:      "v1.0.0",
		InstallPaths: []string{filepath.Join(tmpDir, "code", "bin", "go_tool")}, // missing
	}

	var buf bytes.Buffer
	result := BuildPackage(context.Background(), &buf, "go_tool", pkg, BuildOptions{})
	if result.Action == "up-to-date" {
		t.Fatalf(
			"expected NOT up-to-date when install_path missing, got up-to-date\n%s",
			buf.String(),
		)
	}
	if !strings.Contains(buf.String(), "install_path missing") {
		t.Errorf("expected an 'install_path missing' log line, got: %s", buf.String())
	}
}
