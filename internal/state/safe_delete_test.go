package state

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withSandbox creates a temp dir to act as $HOME, returns it plus a cleanup
// function. AllowedPrefixes is set to the sandbox so SafeRemove only
// permits removal under it.
func withSandbox(t *testing.T) (string, SafeRemoveOptions, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "ralph-safe-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	opts := SafeRemoveOptions{
		AllowedPrefixes: []string{dir},
		Logger:          &bytes.Buffer{},
	}
	return dir, opts, func() { os.RemoveAll(dir) }
}

// --- Path validation rails ---

func TestSafeRemove_RejectsEmptyPath(t *testing.T) {
	err := SafeRemove("", KindSymlink, SafeRemoveOptions{})
	if !errors.Is(err, ErrEmptyPath) {
		t.Errorf("expected ErrEmptyPath, got %v", err)
	}
}

func TestSafeRemove_RejectsRelativePath(t *testing.T) {
	err := SafeRemove("relative/path", KindSymlink, SafeRemoveOptions{
		AllowedPrefixes: []string{"/"},
	})
	if !errors.Is(err, ErrNotAbsolute) {
		t.Errorf("expected ErrNotAbsolute, got %v", err)
	}
}

func TestSafeRemove_RejectsGlobAsterisk(t *testing.T) {
	err := SafeRemove("/home/u/code/bin/*", KindInstallPath, SafeRemoveOptions{})
	if !errors.Is(err, ErrGlobInPath) {
		t.Errorf("expected ErrGlobInPath, got %v", err)
	}
}

func TestSafeRemove_RejectsGlobBracket(t *testing.T) {
	err := SafeRemove("/home/u/[abc]", KindSymlink, SafeRemoveOptions{})
	if !errors.Is(err, ErrGlobInPath) {
		t.Errorf("expected ErrGlobInPath, got %v", err)
	}
}

func TestSafeRemove_RejectsGlobBrace(t *testing.T) {
	err := SafeRemove("/home/u/{a,b}", KindSymlink, SafeRemoveOptions{})
	if !errors.Is(err, ErrGlobInPath) {
		t.Errorf("expected ErrGlobInPath, got %v", err)
	}
}

func TestSafeRemove_RejectsGlobQuestion(t *testing.T) {
	err := SafeRemove("/home/u/?file", KindSymlink, SafeRemoveOptions{})
	if !errors.Is(err, ErrGlobInPath) {
		t.Errorf("expected ErrGlobInPath, got %v", err)
	}
}

func TestSafeRemove_RejectsOutsideAllowedPrefix(t *testing.T) {
	_, opts, cleanup := withSandbox(t)
	defer cleanup()
	err := SafeRemove("/etc/passwd", KindInstallPath, opts)
	if !errors.Is(err, ErrOutsideHome) {
		t.Errorf("expected ErrOutsideHome, got %v", err)
	}
}

func TestSafeRemove_RejectsPrefixTraversal(t *testing.T) {
	// Sandbox=/tmp/foo. /tmp/foobar must be rejected (string-prefix would
	// match without the separator check).
	dir, err := os.MkdirTemp("", "ralph-safe-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(dir)
	sibling := dir + "bar"
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatalf("mkdir sibling: %v", err)
	}
	defer os.RemoveAll(sibling)

	target := filepath.Join(sibling, "file")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	opts := SafeRemoveOptions{AllowedPrefixes: []string{dir}}
	if err := SafeRemove(target, KindInstallPath, opts); !errors.Is(err, ErrOutsideHome) {
		t.Errorf("expected ErrOutsideHome for sibling-path attack, got %v", err)
	}
}

// --- Kind verification: symlink ---

func TestSafeRemove_Symlink_Removes(t *testing.T) {
	dir, opts, cleanup := withSandbox(t)
	defer cleanup()

	link := filepath.Join(dir, "link")
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("t"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := SafeRemove(link, KindSymlink, opts); err != nil {
		t.Fatalf("SafeRemove: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("expected link to be gone")
	}
	if _, err := os.Lstat(target); err != nil {
		t.Errorf("target should be untouched")
	}
}

func TestSafeRemove_Symlink_RefusesRegularFile(t *testing.T) {
	dir, opts, cleanup := withSandbox(t)
	defer cleanup()

	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := SafeRemove(regular, KindSymlink, opts)
	if !errors.Is(err, ErrWrongKind) {
		t.Errorf("expected ErrWrongKind, got %v", err)
	}
	if _, statErr := os.Stat(regular); statErr != nil {
		t.Errorf("file should still exist")
	}
}

func TestSafeRemove_Symlink_GoneIsNoOp(t *testing.T) {
	dir, opts, cleanup := withSandbox(t)
	defer cleanup()

	if err := SafeRemove(filepath.Join(dir, "missing"), KindSymlink, opts); err != nil {
		t.Errorf("missing path should be a no-op, got %v", err)
	}
}

// --- Kind verification: install_path / regular file ---

func TestSafeRemove_InstallPath_Removes(t *testing.T) {
	dir, opts, cleanup := withSandbox(t)
	defer cleanup()
	bin := filepath.Join(dir, "code", "bin", "foo")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SafeRemove(bin, KindInstallPath, opts); err != nil {
		t.Fatalf("SafeRemove: %v", err)
	}
	if _, err := os.Stat(bin); !os.IsNotExist(err) {
		t.Error("expected binary to be removed")
	}
}

func TestSafeRemove_InstallPath_RefusesDirectory(t *testing.T) {
	dir, opts, cleanup := withSandbox(t)
	defer cleanup()
	target := filepath.Join(dir, "actually_a_dir")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	err := SafeRemove(target, KindInstallPath, opts)
	if !errors.Is(err, ErrWrongKind) {
		t.Errorf("expected ErrWrongKind, got %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Error("dir should be untouched")
	}
}

// --- Kind verification: directory ---

func TestSafeRemove_Directory_RemovesEmpty(t *testing.T) {
	dir, opts, cleanup := withSandbox(t)
	defer cleanup()
	target := filepath.Join(dir, "empty")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SafeRemove(target, KindDirectory, opts); err != nil {
		t.Fatalf("SafeRemove: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected dir to be removed")
	}
}

func TestSafeRemove_Directory_RefusesNonEmpty(t *testing.T) {
	dir, opts, cleanup := withSandbox(t)
	defer cleanup()
	target := filepath.Join(dir, "not_empty")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "child"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := SafeRemove(target, KindDirectory, opts)
	if !errors.Is(err, ErrDirNotEmpty) {
		t.Errorf("expected ErrDirNotEmpty, got %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Error("dir should still exist")
	}
}

// --- Unsupported kinds ---

func TestSafeRemove_Repo_RefusedAlwaysAbandoned(t *testing.T) {
	dir, opts, cleanup := withSandbox(t)
	defer cleanup()
	target := filepath.Join(dir, "repo")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	err := SafeRemove(target, KindRepo, opts)
	if !errors.Is(err, ErrUnsupportedKind) {
		t.Errorf("expected ErrUnsupportedKind, got %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Error("repo should be untouched")
	}
}

func TestSafeRemove_ShellAlias_NotAFilesystemKind(t *testing.T) {
	dir, opts, cleanup := withSandbox(t)
	defer cleanup()
	err := SafeRemove(filepath.Join(dir, "anything"), KindShellAlias, opts)
	if !errors.Is(err, ErrUnsupportedKind) {
		t.Errorf("expected ErrUnsupportedKind, got %v", err)
	}
}

func TestSafeRemove_UnknownKind_Errors(t *testing.T) {
	dir, opts, cleanup := withSandbox(t)
	defer cleanup()
	err := SafeRemove(filepath.Join(dir, "x"), ArtifactKind("bogus"), opts)
	if !errors.Is(err, ErrUnknownKind) {
		t.Errorf("expected ErrUnknownKind, got %v", err)
	}
}

// --- Dry-run ---

func TestSafeRemove_DryRun_DoesNotModifyDisk(t *testing.T) {
	dir, opts, cleanup := withSandbox(t)
	defer cleanup()
	opts.DryRun = true

	target := filepath.Join(dir, "file")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SafeRemove(target, KindInstallPath, opts); err != nil {
		t.Fatalf("SafeRemove: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("expected file to remain in dry-run mode, got: %v", err)
	}
	buf := opts.Logger.(*bytes.Buffer)
	if !strings.Contains(buf.String(), "would remove") {
		t.Errorf("expected log line containing 'would remove', got: %s", buf.String())
	}
}

func TestSafeRemove_LoggerNilSafe(t *testing.T) {
	dir, _, cleanup := withSandbox(t)
	defer cleanup()
	target := filepath.Join(dir, "file")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := SafeRemoveOptions{
		AllowedPrefixes: []string{dir},
		Logger:          nil,
	}
	if err := SafeRemove(target, KindInstallPath, opts); err != nil {
		t.Errorf("nil logger should be safe, got %v", err)
	}
}

// --- DefaultAllowedPrefixes ---

func TestDefaultAllowedPrefixes_IncludesHome(t *testing.T) {
	got := DefaultAllowedPrefixes("/home/alice")
	if len(got) == 0 {
		t.Fatal("expected at least one prefix")
	}
	if got[0] != "/home/alice" {
		t.Errorf("expected /home/alice as first prefix, got %v", got)
	}
}
