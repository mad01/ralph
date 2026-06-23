package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureGitignored(t *testing.T) {
	t.Run("creates .gitignore when missing", func(t *testing.T) {
		dir := t.TempDir()
		added, err := ensureGitignored(dir, "config.local.toml")
		if err != nil {
			t.Fatalf("ensureGitignored() error = %v", err)
		}
		if !added {
			t.Fatal("expected added=true")
		}
		got := readFile(t, filepath.Join(dir, ".gitignore"))
		if got != "config.local.toml\n" {
			t.Errorf(".gitignore = %q, want %q", got, "config.local.toml\n")
		}
	})

	t.Run("idempotent when entry already present", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".gitignore"), "node_modules\nconfig.local.toml\n")

		added, err := ensureGitignored(dir, "config.local.toml")
		if err != nil {
			t.Fatalf("ensureGitignored() error = %v", err)
		}
		if added {
			t.Error("expected added=false when entry already present")
		}
		got := readFile(t, filepath.Join(dir, ".gitignore"))
		if got != "node_modules\nconfig.local.toml\n" {
			t.Errorf(".gitignore changed: %q", got)
		}
	})

	t.Run("appends a newline when file lacks trailing newline", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".gitignore"), "node_modules")

		if _, err := ensureGitignored(dir, "config.local.toml"); err != nil {
			t.Fatalf("ensureGitignored() error = %v", err)
		}
		got := readFile(t, filepath.Join(dir, ".gitignore"))
		if got != "node_modules\nconfig.local.toml\n" {
			t.Errorf(".gitignore = %q, want %q", got, "node_modules\nconfig.local.toml\n")
		}
	})

	t.Run("no-op when repo dir does not exist", func(t *testing.T) {
		added, err := ensureGitignored(filepath.Join(t.TempDir(), "nope"), "config.local.toml")
		if err != nil {
			t.Fatalf("ensureGitignored() error = %v", err)
		}
		if added {
			t.Error("expected added=false for missing repo dir")
		}
	})
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
